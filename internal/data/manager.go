package data

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/ekaley/ipv666/internal/addressing"
	"github.com/ekaley/ipv666/internal/blacklist"
	"github.com/ekaley/ipv666/internal/config"
	"github.com/ekaley/ipv666/internal/filtering"
	"github.com/ekaley/ipv666/internal/fs"
	"github.com/ekaley/ipv666/internal/logging"
	"github.com/ekaley/ipv666/internal/modeling"
	"github.com/gobuffalo/packr/v2"
	"github.com/spf13/viper"
	"github.com/willf/bloom"
)

var curCandidatePingResults []*net.IP
var curCandidatePingResultsPath string
var curScanResultsNetworkRanges []*net.IPNet
var curScanResultsNetworkRangesPath string
var curBlacklist *blacklist.NetworkBlacklist
var curBlacklistPath string
var curCleanPingResults []*net.IP
var curCleanPingResultsPath string
var curBloomFilter *bloom.BloomFilter
var curBloomFilterPath string
var curAliasedNetworks []*net.IPNet
var curAliasedNetworksPath string
var curClusterModel *modeling.ClusterModel
var packedBox = packr.New("box", "../../assets")

//TODO add unit tests for making sure that the boxed assets are returned

func GetMostRecentTargetNetworkString() (string, error) {
	if !fs.CheckIfFileExists(config.GetTargetNetworkFilePath()) {
		return "", nil
	}
	content, err := ioutil.ReadFile(config.GetTargetNetworkFilePath())
	if err != nil {
		return "", nil
	}
	network, err := addressing.GetIPv6NetworkFromBytesIncLength(content)
	if err != nil {
		return "", err
	}
	return network.String(), nil
}

func WriteMostRecentTargetNetwork(toWrite *net.IPNet) error {
	return addressing.WriteIPv6NetworksToFile(config.GetTargetNetworkFilePath(), []*net.IPNet{toWrite})
}

func UpdateAliasedNetworks(nets []*net.IPNet, filePath string) {
	curAliasedNetworks = nets
	curAliasedNetworksPath = filePath
}

func GetAliasedNetworks() ([]*net.IPNet, error) {
	aliasedDir := config.GetAliasedNetworkDirPath()
	logging.Debugf("Attempting to retrieve most recent aliased networks from directory '%s'.", aliasedDir)
	fileName, err := fs.GetMostRecentFileFromDirectory(aliasedDir)
	if err != nil {
		logging.Warnf("Error thrown when retrieving aliased networks from directory '%s': %s", aliasedDir, err)
		return nil, err
	} else if fileName == "" {
		logging.Warnf("The directory at '%s' was empty.", aliasedDir)
		return nil, errors.New(fmt.Sprintf("No aliased networks were found in directory %s.", aliasedDir))
	}
	filePath := filepath.Join(aliasedDir, fileName)
	logging.Debugf("Most recent aliased networks file is at path '%s'.", filePath)
	if filePath == curAliasedNetworksPath {
		logging.Debugf("Already have aliased networks from path '%s' loaded in memory. Returning.", filePath)
		return curAliasedNetworks, nil
	} else {
		logging.Debugf("Loading aliased networks from path '%s'.", filePath)
		toReturn, err := addressing.ReadIPv6NetworksFromFile(filePath)
		if err == nil {
			UpdateAliasedNetworks(toReturn, filePath)
		}
		return toReturn, err
	}
}

func UpdateBloomFilter(filter *bloom.BloomFilter, filePath string) {
	curBloomFilter = filter
	curBloomFilterPath = filePath
}

func LoadBloomFilterFromOutput() (*bloom.BloomFilter, error) {
	logging.Debugf("Creating Bloom filter from output file '%s'.", config.GetOutputFilePath())
	ips, err := fs.ReadIPsFromHexFile(config.GetOutputFilePath())
	ips = addressing.GetUniqueIPs(ips, viper.GetInt("LogLoopEmitFreq"))
	if err != nil {
		return nil, err
	}
	logging.Debugf("%d IP addresses loaded from file '%s'.", len(ips), config.GetOutputFilePath())
	newBloom := bloom.New(uint(viper.GetInt("AddressFilterSize")), uint(viper.GetInt("AddressFilterHashCount")))
	for _, ip := range ips {
		ipBytes := ([]byte)(*ip)
		newBloom.Add(ipBytes)
	}
	logging.Debugf("Created Bloom filter with %d addresses from '%s'.", len(ips), config.GetOutputFilePath())
	return newBloom, nil
}

func GetBloomFilter() (*bloom.BloomFilter, error) {
	filterDir := config.GetBloomDirPath()
	logging.Debugf("Attempting to retrieve most recent Bloom filter from directory '%s'.", filterDir)
	fileName, err := fs.GetMostRecentFileFromDirectory(filterDir)
	if err != nil {
		logging.Warnf("Error thrown when retrieving Bloom filter from directory '%s': %s", filterDir, err)
		return nil, err
	} else if fileName == "" {
		logging.Debugf("The directory at '%s' was empty. Checking for pre-existing output file at '%s'.", filterDir, config.GetOutputFilePath())
		if _, err := os.Stat(config.GetOutputFilePath()); !os.IsNotExist(err) {
			logging.Debugf("File at path '%s' exists. Using for new Bloom filter.", config.GetOutputFilePath())
			return LoadBloomFilterFromOutput()
		} else {
			logging.Debugf("No existing output file at '%s'. Returning a new, empty Bloom filter.", config.GetOutputFilePath())
			return bloom.New(uint(viper.GetInt("AddressFilterSize")), uint(viper.GetInt("AddressFilterHashCount"))), nil
		}
	}
	filePath := filepath.Join(filterDir, fileName)
	logging.Debugf("Most recent Bloom filter is at path '%s'.", filePath)
	if filePath == curBloomFilterPath {
		logging.Debugf("Already have Bloom filter at path '%s' loaded in memory. Returning.", filePath)
		return curBloomFilter, nil
	} else {
		logging.Debugf("Loading Bloom filter from path '%s'.", filePath)
		toReturn, err := filtering.GetBloomFilterFromFile(filePath, uint(viper.GetInt("AddressFilterSize")), uint(viper.GetInt("AddressFilterHashCount")))
		if err == nil {
			UpdateBloomFilter(toReturn, filePath)
		}
		return toReturn, err
	}
}

func UpdateCleanPingResults(addrs []*net.IP, filePath string) {
	curCleanPingResults = addrs
	curCleanPingResultsPath = filePath
}

func GetCleanPingResults() ([]*net.IP, error) {
	resultsDir := config.GetCleanPingDirPath()
	logging.Debugf("Attempting to retrieve most recent cleaned ping results from directory '%s'.", resultsDir)
	fileName, err := fs.GetMostRecentFileFromDirectory(resultsDir)
	if err != nil {
		logging.Warnf("Error thrown when retrieving cleaned ping results from directory '%s': %e", resultsDir, err)
		return nil, err
	} else if fileName == "" {
		logging.Debugf("The directory at '%s' was empty.", resultsDir)
		return nil, errors.New(fmt.Sprintf("No cleaned ping results files were found in directory %s.", resultsDir))
	}
	filePath := filepath.Join(resultsDir, fileName)
	logging.Debugf("Most recent cleaned ping results file is at path '%s'.", filePath)
	if filePath == curCleanPingResultsPath {
		logging.Debugf("Already have cleaned ping results at path '%s' loaded in memory. Returning.", filePath)
		return curCleanPingResults, nil
	} else {
		logging.Debugf("Loading cleaned ping results from path '%s'.", filePath)
		toReturn, err := addressing.ReadIPsFromBinaryFile(filePath)
		if err == nil {
			UpdateCleanPingResults(toReturn, filePath)
		}
		return toReturn, err
	}
}

func UpdateBlacklist(blacklist *blacklist.NetworkBlacklist, filePath string) {
	curBlacklist = blacklist
	curBlacklistPath = filePath
}

func GetBlacklist() (*blacklist.NetworkBlacklist, error) {
	blacklistDir := config.GetNetworkBlacklistDirPath()
	logging.Debugf("Attempting to retrieve most recent blacklist from directory '%s'.", blacklistDir)
	fileName, err := fs.GetMostRecentFileFromDirectory(blacklistDir)
	if err != nil {
		logging.Warnf("Error thrown when retrieving blacklist from directory '%s': %s", blacklistDir, err)
		return nil, err
	} else if fileName == "" {
		logging.Debugf("The directory at '%s' was empty.", blacklistDir)
		if curBlacklist != nil {
			logging.Debugf("Already have a blacklist loaded from box. Using it.")
			return curBlacklist, nil
		}
		logging.Debugf("Loading blacklist from box.")
		toReturn, err := getBlacklistFromBox()
		if err != nil {
			return nil, err
		}
		UpdateBlacklist(toReturn, "")
		return toReturn, nil
	}
	filePath := filepath.Join(blacklistDir, fileName)
	logging.Debugf("Most recent blacklist file is at path '%s'.", filePath)
	if filePath == curBlacklistPath {
		logging.Debugf("Already have blacklist at path '%s' loaded in memory. Returning.", filePath)
		return curBlacklist, nil
	} else {
		toReturn, err := blacklist.ReadNetworkBlacklistFromFile(filePath)
		if err == nil {
			UpdateBlacklist(toReturn, filePath)
		}
		return toReturn, err
	}
}

func getBlacklistFromBox() (*blacklist.NetworkBlacklist, error) {
	content, err := packedBox.Find("blacklist.zlib")
	if err != nil {
		return nil, err
	}
	b := bytes.NewReader(content)
	z, err := zlib.NewReader(b)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	decompressed, err := ioutil.ReadAll(z)
	if err != nil {
		return nil, err
	}
	nets, err := addressing.BytesToIPv6Networks(decompressed)
	if err != nil {
		return nil, err
	}
	return blacklist.NewNetworkBlacklist(nets), nil
}

func UpdateScanResultsNetworkRanges(networks []*net.IPNet, filePath string) {
	curScanResultsNetworkRanges = networks
	curScanResultsNetworkRangesPath = filePath
}

func GetScanResultsNetworkRanges() ([]*net.IPNet, error) {
	scanResultsDir := config.GetNetworkGroupDirPath()
	logging.Debugf("Attempting to retrieve most recent candidate ping networks from directory '%s'.", scanResultsDir)
	fileName, err := fs.GetMostRecentFileFromDirectory(scanResultsDir)
	if err != nil {
		logging.Warnf("Error thrown when retrieving candidate ping networks from directory '%s': %s", scanResultsDir, err)
		return nil, err
	} else if fileName == "" {
		logging.Debugf("The directory at '%s' was empty.", scanResultsDir)
		return nil, errors.New(fmt.Sprintf("No candidate ping networks files were found in directory %s.", scanResultsDir))
	}
	filePath := filepath.Join(scanResultsDir, fileName)
	logging.Debugf("Most recent candidate ping networks file is at path '%s'.", filePath)
	if filePath == curScanResultsNetworkRangesPath {
		logging.Debugf("Already have candidate ping networks at path '%s' loaded in memory. Returning.", filePath)
		return curScanResultsNetworkRanges, nil
	} else {
		logging.Debugf("Loading candidate ping networks from path '%s'.", filePath)
		toReturn, err := addressing.ReadIPv6NetworksFromFile(filePath)
		if err == nil {
			UpdateScanResultsNetworkRanges(toReturn, filePath)
		}
		return toReturn, err
	}
}

func UpdateCandidatePingResults(ips []*net.IP, filePath string) {
	curCandidatePingResultsPath = filePath
	curCandidatePingResults = ips
}

func GetCandidatePingResults() ([]*net.IP, error) {
	pingResultsDir := config.GetPingResultDirPath()
	logging.Debugf("Attempting to retrieve most recent candidate ping results from directory '%s'.", pingResultsDir)
	fileName, err := fs.GetMostRecentFileFromDirectory(pingResultsDir)
	if err != nil {
		logging.Warnf("Error thrown when retrieving candidate ping results from directory '%s': %s", pingResultsDir, err)
		return nil, err
	} else if fileName == "" {
		logging.Debugf("The directory at '%s' was empty.", pingResultsDir)
		return nil, errors.New(fmt.Sprintf("No candidate ping files were found in directory %s.", pingResultsDir))
	}
	filePath := filepath.Join(pingResultsDir, fileName)
	logging.Debugf("Most recent ping results file is at path '%s'.", filePath)
	if filePath == curCandidatePingResultsPath {
		logging.Debugf("Already have candidate ping results at path '%s' loaded in memory. Returning.", filePath)
		return curCandidatePingResults, nil
	} else {
		logging.Debugf("Loading candidate ping results from path '%s'.", filePath)
		toReturn, err := fs.ReadIPsFromHexFile(filePath)
		if err == nil {
			UpdateCandidatePingResults(toReturn, filePath)
		}
		return toReturn, err
	}
}

func GetProbabilisticClusterModel() (*modeling.ClusterModel, error) {
	if curClusterModel != nil {
		logging.Debugf("Already have a cluster model loaded from box. Returning.")
		return curClusterModel, nil
	}
	logging.Debugf("Loading cluster model from box...")
	toReturn, err := getClusterModelFromBox()
	if err != nil {
		return &modeling.ClusterModel{}, err
	}
	curClusterModel = toReturn
	return toReturn, nil
}

func getClusterModelFromBox() (*modeling.ClusterModel, error) { //TODO generalize fetching from box and decompressing zlib
	content, err := packedBox.Find("clustermodel.zlib")
	if err != nil {
		return &modeling.ClusterModel{}, err
	}
	b := bytes.NewReader(content)
	z, err := zlib.NewReader(b)
	if err != nil {
		return &modeling.ClusterModel{}, err
	}
	defer z.Close()
	modelBytes, err := ioutil.ReadAll(z)
	if err != nil {
		return &modeling.ClusterModel{}, err
	}
	return modeling.LoadModelFromBytes(modelBytes)
}

func GetMostRecentFilePathFromDir(candidateDir string) (string, error) {
	logging.Debugf("Attempting to find most recent file path in directory '%s'.", candidateDir)
	fileName, err := fs.GetMostRecentFileFromDirectory(candidateDir)
	if err != nil {
		logging.Warnf("Error thrown when finding most recent candidate file path in directory '%s': %s", candidateDir, err)
		return "", err
	} else if fileName == "" {
		return "", errors.New(fmt.Sprintf("No file was found in directory '%s'.", candidateDir))
	} else {
		logging.Debugf("Most recent file path in directory '%s' is '%s'.", candidateDir, fileName)
		filePath := filepath.Join(candidateDir, fileName)
		return filePath, nil
	}
}
