package app

import (
	"github.com/ekaley/ipv666/internal/addressing"
	"github.com/ekaley/ipv666/internal/blacklist"
	"github.com/ekaley/ipv666/internal/fs"
	"github.com/ekaley/ipv666/internal/logging"
	"github.com/spf13/viper"
)

func RunClean(inputPath string, outputPath string, blist *blacklist.NetworkBlacklist) {

	addrs, err := fs.ReadIPsFromHexFile(inputPath)

	if err != nil {
		logging.ErrorStringFf("Error thrown when reading input list of IP addresses at path '%s': %e", inputPath, err)
	}
	logging.Infof("Successfully loaded IP addresses from '%s'.", inputPath)

	uniqAddrs := addressing.GetUniqueIPs(addrs, viper.GetInt("LogLoopEmitFreq"))

	logging.Infof("Whittled %d input addresses down to %d unique addresses.", len(addrs), len(uniqAddrs))

	outAddrs := blist.CleanIPList(uniqAddrs, viper.GetInt("LogLoopEmitFreq"))

	logging.Infof("%d addresses remain after cleaning from blacklist (started with %d).", len(outAddrs), len(uniqAddrs))

	// Write results to disk

	logging.Infof("Writing cleaned address list to file at path '%s'.", outputPath)

	err = addressing.WriteIPsToHexFile(outputPath, outAddrs)

	if err != nil {
		logging.Warnf("Error thrown when writing %d addresses to '%s': %e", len(outAddrs), outputPath, err)
	}

	logging.Successf("Successfully wrote results to file '%s'.", outputPath)

}
