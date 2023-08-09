package statemachine

import (
	"errors"
	"fmt"
	"github.com/ekaley/ipv666/internal/config"
	"github.com/ekaley/ipv666/internal/logging"
	"github.com/rcrowley/go-metrics"
	"github.com/spf13/viper"
	"io/ioutil"
	"time"
)

// noinspection GoSnakeCaseUsage
const (
	GEN_ADDRESSES State = iota
	PING_SCAN_ADDR
	PING_SCAN_ALIAS_REMOVAL
	FAN_OUT_NYBBLE_ADJACENT
	FAN_OUT_NYBBLE_ADJACENT_ALIAS_REMOVAL
	FAN_OUT_64
	FAN_OUT_64_ALIAS_REMOVAL
	CLEAN_UP
	EMIT_METRICS
)

var FIRST_STATE = GEN_ADDRESSES
var LAST_STATE = EMIT_METRICS

type State int8

var stateLoopTimers = make(map[string]metrics.Timer)

func init() {
	for i := FIRST_STATE; i <= LAST_STATE; i++ {
		key := getTimerKeyForLoop((int)(i))
		timer := metrics.NewTimer()
		metrics.Register(key, timer)
		stateLoopTimers[key] = timer
	}
}

func getTimerKeyForLoop(loop int) string {
	return fmt.Sprintf("loop.state_%d.time", loop)
}

func getStateLoopTimer(state State) (metrics.Timer, bool) {
	key := getTimerKeyForLoop((int)(state))
	timer, found := stateLoopTimers[key]
	return timer, found
}

func fetchStateFromFile(filePath string) (State, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return -1, err
	}
	if len(content) == 0 || len(content) > 1 {
		return -1, errors.New(fmt.Sprintf("Content of file at '%s' was of unexpected length (%d).", filePath, len(content)))
	}
	state := int(content[0])
	if state < int(FIRST_STATE) || state > int(LAST_STATE) {
		return -1, errors.New(fmt.Sprintf("State with value %d was unexpected (expected between %d and %d, inclusive).", state, FIRST_STATE, LAST_STATE))
	}
	return State(state), nil
}

func postScanCleanup() error {

	// Process results of ping scan into a set of network ranges
	err := generateScanResultsNetworkRanges()
	if err != nil {
		return err
	}

	// Seek out aliased networks
	err = seekAliasedNetworks()
	if err != nil {
		return err
	}

	// Process the results of aliased network seeking (add to blacklist and de-dupe)
	err = processAliasedNetworks()
	if err != nil {
		return err
	}

	// Remove all the addressing from the ping scan results that are in ranges that failed
	// the test in the previous step
	err = cleanBlacklistedAddresses()
	if err != nil {
		return err
	}

	// Update the cumulative addresses file
	err = updateAddressFile()
	if err != nil {
		return err
	}

	return nil
}

func SetStateFile(filePath string, curState State) error {
	logging.Debugf("Now updating state file at path '%s' with current state of %d.", filePath, curState)
	var b []byte
	b = append(b, byte(curState))
	return ioutil.WriteFile(filePath, b, 0644)
}

func ResetStateFile(filePath string) error {
	return SetStateFile(filePath, FIRST_STATE)
}

func InitStateFile(filePath string) error {
	return SetStateFile(filePath, FIRST_STATE)
}

func RunStateMachine() error {

	logging.Infof("Now starting to run the state machine.")

	state, err := fetchStateFromFile(config.GetStateFilePath())

	if err != nil {
		return err
	}

	logging.Debugf("Starting at state %d.", state)

	for {

		logging.Debugf("Now entering state %d.", state)
		start := time.Now()

		switch state {
		case GEN_ADDRESSES:
			// Generate the candidate addressing to scan from the most recent model
			err := generateCandidateAddresses()
			if err != nil {
				return err
			}
		case PING_SCAN_ADDR:
			// Perform a ping scan of the candidate addressing that were generated
			err := pingScanCandidateAddresses()
			if err != nil {
				return err
			}
		case PING_SCAN_ALIAS_REMOVAL:
			// Perform alias network detection and cleanup
			err := postScanCleanup()
			if err != nil {
				return err
			}
		case FAN_OUT_NYBBLE_ADJACENT:
			// Fan out to find neighboring nybble-adjacent addresses
			err := fanOutNybbleAdjacent()
			if err != nil {
				return err
			}
		case FAN_OUT_NYBBLE_ADJACENT_ALIAS_REMOVAL:
			// Perform alias network detection and cleanup
			err := postScanCleanup()
			if err != nil {
				return err
			}
		case FAN_OUT_64:
			// Fan out to find neighboring /64 networks from the discovered address set, and
			// monotonically-increasing addresses from each /64
			err := fanOutSlash64s()
			if err != nil {
				return err
			}
		case FAN_OUT_64_ALIAS_REMOVAL:
			// Perform alias network detection and cleanup
			err := postScanCleanup()
			if err != nil {
				return err
			}
		case CLEAN_UP:
			// Remove all but the most recent files in each of the directories
			if !viper.GetBool("CleanUpEnabled") {
				logging.Infof("Clean up disabled. Skipping clean up step.")
			} else {
				err := cleanUpNonRecentFiles()
				if err != nil {
					return err
				}
			}
		case EMIT_METRICS:
			// Emit metrics
		}

		elapsed := time.Since(start)
		logging.Debugf("Completed state %d (took %s).", state, elapsed)

		timer, found := getStateLoopTimer(state)
		if !found {
			logging.Warnf("Unable to find state loop timer for state %d.", state)
			if viper.GetBool("ExitOnFailedMetrics") {
				return errors.New(fmt.Sprintf("Unable to find state loop timer for state %d.", state))
			}
		}
		timer.Update(elapsed)

		state = (state + 1) % (LAST_STATE + 1)
		err = SetStateFile(config.GetStateFilePath(), state)
		if err != nil {
			return err
		}
	}
}
