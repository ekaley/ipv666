package app

import (
	"fmt"
	"github.com/ekaley/ipv666/internal/addressing"
	"github.com/ekaley/ipv666/internal/blacklist"
	"github.com/ekaley/ipv666/internal/config"
	"github.com/ekaley/ipv666/internal/fs"
	"github.com/ekaley/ipv666/internal/logging"
	"github.com/ekaley/ipv666/internal/pingscan"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"net"
)

func RunAlias(targetNetworkString string) {

	_, targetNetwork, err := net.ParseCIDR(targetNetworkString)
	if err != nil {
		logging.ErrorF(err)
	}

	ip, aliased, err := checkNetworkForAliased(targetNetwork)

	if err != nil {
		logging.ErrorF(err)
	} else if !aliased {
		logging.ErrorStringFf("Your input range of %s does not appear to be aliased based on your current configured settings. Exiting.", targetNetwork.String())
	}

	logging.Info("As the initial network appears to be aliased, we will now seek out the network length.")

	aliasedNet, err := seekAliasedNetwork(targetNetwork, ip)

	if err != nil {
		logging.ErrorF(err)
	}

	logging.Success("Aliased network found!")
	logging.Success("")
	logging.Successf("%s", aliasedNet)

}

func seekAliasedNetwork(inputNet *net.IPNet, inputIP *net.IP) (*net.IPNet, error) {

	logging.Infof("Now seeking aliased network length starting from input range of %s. Addresses that responded will be %s.", inputNet, inputIP)

	ones, _ := inputNet.Mask.Size()
	acs, err := blacklist.NewAliasCheckStates([]*net.IP{inputIP}, uint8(viper.GetInt("AliasLeftIndexStart")), uint8(ones))
	var i int
	var toReturn *net.IPNet

	if err != nil {
		logging.Warnf("Error thrown when creating new alias check states: %e", err)
		return nil, err
	}

	loopCount := 0
	for {
		logging.Debugf("Now starting loop %d.", loopCount)
		logging.Debug("Generating test addresses...")
		testAddrs := acs.GetTestAddresses()
		if len(testAddrs) == 0 {
			logging.Warnf("Somehow did not generate any test addresses in loop %d. This shouldn't happen.", loopCount)
			return nil, errors.New(fmt.Sprintf("did not generate any test addresses in loop %d", loopCount))
		}
		logging.Debugf("%d addresses generated for loop %d.", len(testAddrs), loopCount)
		var scanAddrs []*net.IP
		for _, testAddr := range testAddrs {
			for i = 0; i < viper.GetInt("AliasDuplicateScanCount"); i++ {
				scanAddrs = append(scanAddrs, testAddr)
			}
		}
		targetsPath := fs.GetTimedFilePath(config.GetNetworkScanTargetsDirPath())
		logging.Debugf("Writing %d blacklist scan addresses to file '%s'.", len(scanAddrs), targetsPath)
		err := addressing.WriteIPsToHexFile(targetsPath, scanAddrs)
		if err != nil {
			logging.Warnf("Error thrown when writing %d addresses to file '%s': %e", len(scanAddrs), targetsPath, err)
			return nil, err
		}
		logging.Debugf("Successfully wrote %d blacklist scan addresses to file '%s'.", len(scanAddrs), targetsPath)
		outputPath := fs.GetTimedFilePath(config.GetNetworkScanResultsDirPath())
		logging.Debugf("Kicking off ping scan from file path '%s' to output path '%s'.", targetsPath, outputPath)
		_, err = pingscan.ScanFromConfig(targetsPath, outputPath)
		if err != nil {
			logging.Warnf("An error was thrown when trying to run ping scan: %s", err)
			return nil, err
		}
		foundAddrs, err := fs.ReadIPsFromHexFile(outputPath)
		if err != nil {
			logging.Warnf("Error thrown when reading IP addresses from file '%s': %e", outputPath, err)
			return nil, err
		}
		logging.Debugf("%d addresses responded to ICMP pings.", len(foundAddrs))
		foundAddrSet := addressing.GetIPSet(foundAddrs)
		logging.Debugf("Updating check list with results from ping scan.")
		acs.Update(foundAddrSet)
		acs.PrintStates()
		if acs.GetAllFound() {
			nets, err := acs.GetAliasedNetworks()
			if err != nil {
				logging.Warnf("Error thrown when retrieving aliased networks from AliasCheckStates: %e", err)
				return nil, err
			} else if len(nets) == 0 {
				return nil, errors.New("no aliased network returned in call to GetAliasedNetworks (length 0)")
			}
			toReturn = nets[0]
			logging.Infof("It looks like we've found the aliased network border. Aliased network is %s.", toReturn)
			break
		} else {
			logging.Infof("Did not find aliased network on loop %d. Let's do this again!", loopCount)
			loopCount++
		}
	}

	logging.Successf("It took a total of %d loops to identify the aliased network %s.", loopCount, toReturn)

	return toReturn, nil

}

func checkNetworkForAliased(inputNet *net.IPNet) (*net.IP, bool, error) {

	logging.Infof("Now checking network range %s for aliased status.", inputNet)

	addrs := addressing.GenerateRandomAddressesInNetwork(inputNet, viper.GetInt("NetworkPingCount"))
	addrsPath := fs.GetTimedFilePath(config.GetNetworkScanTargetsDirPath())

	logging.Debugf("Writing %d test addresses to file at path '%s'.", len(addrs), addrsPath)

	err := addressing.WriteIPsToHexFile(addrsPath, addrs)
	if err != nil {
		logging.Warnf("Error thrown when writing %d addresses to file '%s': %e", len(addrs), addrsPath, err)
		return nil, false, err
	}

	logging.Debugf("Wrote test addresses to file at path '%s'.", addrsPath)
	outputPath := fs.GetTimedFilePath(config.GetNetworkScanResultsDirPath())

	_, err = pingscan.ScanFromConfig(addrsPath, outputPath)
	if err != nil {
		logging.Warnf("An error was thrown when trying to run ping scan: %s", err)
		return nil, false, err
	}

	foundAddrs, err := fs.ReadIPsFromHexFile(outputPath)
	if err != nil {
		logging.Warnf("Error thrown when reading IP addresses from file '%s': %e", outputPath, err)
		return nil, false, err
	}

	threshold := (int)(float64(viper.GetInt("NetworkPingCount")) * viper.GetFloat64("NetworkBlacklistPercent"))
	logging.Infof("Threshold for aliased network detection is %d (%d ping count, %f percent). %d addresses responded.", threshold, viper.GetInt("NetworkPingCount"), viper.GetFloat64("NetworkBlacklistPercent"), len(foundAddrs))

	if len(foundAddrs) >= threshold {
		logging.Infof("Initial network of %s appears to be aliased.", inputNet)
		return addrs[0], true, nil
	} else {
		logging.Infof("Initial network of %s does not appear to be aliased.", inputNet)
		return nil, false, nil
	}

}
