package ipv6

import (
	"fmt"
	"math/rand"
	"net"

	"github.com/lavalamp-/ipv666/internal/data"
	"github.com/lavalamp-/ipv666/internal/modeling"
)

func IPv6AddrGen(fromNetwork string, genCount int) ([]*net.IP, error) {

	var clusterModel *modeling.ClusterModel
	var err error

	// logging.Info("No model path specified. Using default model packaged with IPv666.")
	clusterModel, err = data.GetProbabilisticClusterModel()
	if err != nil {
		return nil, fmt.Errorf("loading randomness error %s", err)
	}

	var generatedAddrs []*net.IP

	if fromNetwork == "" {
		return nil, fmt.Errorf("No network specified. no addresses returned")
	} else {
		_, ipnet, _ := net.ParseCIDR(fromNetwork)
		// logging.Infof("Generating addresses in specified network range of '%s'.", ipnet)
		generatedAddrs, err = clusterModel.GenerateAddressesFromNetwork(genCount, rand.Float64(), ipnet)
		if err != nil {
			return nil, fmt.Errorf("generating error %s", err)
		}
	}

	// logging.Infof("Successfully generated %d IP addresses. Writing results to file at path '%s'.", genCount, outputPath)

	// logging.Infof("Successfully wrote addresses to file '%s'.", outputPath)

	return generatedAddrs, nil
}
