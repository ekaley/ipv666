package addressing

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ekaley/ipv666/internal"
	"github.com/ekaley/ipv666/internal/logging"
	"github.com/ekaley/ipv666/internal/zrandom"
	"github.com/spf13/viper"
	"io"
	"net"
	"os"
	"strings"
)

func GetAdjacentNetworkAddressesFromIPs(toParse []*net.IP, fromNybble int, toNybble int) ([]*net.IP, error) {
	var toReturn []*net.IP
	for _, curIP := range toParse {
		newIPs, err := GetAdjacentNetworkAddressesFromIP(curIP, fromNybble, toNybble)
		if err != nil {
			return nil, err
		} else {
			toReturn = append(toReturn, newIPs...)
		}
	}
	return GetUniqueIPs(toReturn, viper.GetInt("LogLoopEmitFreq")), nil
}

func GetAdjacentNetworkAddressesFromIP(toParse *net.IP, fromNybble int, toNybble int) ([]*net.IP, error) {
	if fromNybble < 0 {
		return nil, fmt.Errorf("fromNybble must be >= 0 (got %d)", fromNybble)
	} else if toNybble > 32 {
		return nil, fmt.Errorf("toNybble must be <= 32 (got %d)", toNybble)
	} else if fromNybble == toNybble {
		return nil, fmt.Errorf("fromNybble and toNybble must be at least one apart (got %d, %d)", fromNybble, toNybble)
	}
	toReturn := []*net.IP{toParse}
	ipNybbles := GetNybblesFromIP(toParse, 32)
	var j uint8
	curNybbles := make([]uint8, len(ipNybbles))
	for i := fromNybble; i < toNybble; i++ {
		copy(curNybbles, ipNybbles)
		for j = 0; j < 16; j++ {
			if ipNybbles[i] == j {
				continue
			} else {
				curNybbles[i] = j
				toReturn = append(toReturn, NybblesToIP(curNybbles))
			}
		}
	}
	return toReturn, nil
}

func IsAddressIPv4(toCheck *net.IP) bool {
	return toCheck.To4() != nil
}

func GetIPsFromStrings(toParse []string) []*net.IP {
	var toReturn []*net.IP
	for _, curParse := range toParse {
		newIP := net.ParseIP(curParse)
		if newIP == nil {
			logging.Warnf("Could not parse IP from string '%s'.", curParse)
		} else {
			toReturn = append(toReturn, &newIP)
		}
	}
	return toReturn
}

func GetIPSet(ips []*net.IP) map[string]*internal.Empty {
	toReturn := make(map[string]*internal.Empty)
	blacklistEntry := &internal.Empty{}
	for _, ip := range ips {
		toReturn[ip.String()] = blacklistEntry
	}
	return toReturn
}

func GetFirst64BitsOfIP(ip *net.IP) uint64 {
	ipBytes := ([]byte)(*ip)
	return binary.LittleEndian.Uint64(ipBytes[:8])
}

func GetUniqueIPs(ips []*net.IP, updateFreq int) []*net.IP { // TODO refactor this to use addr tree
	checkMap := make(map[string]bool)
	var toReturn []*net.IP
	for i, ip := range ips {
		if i%updateFreq == 0 {
			logging.Debugf("Processing %d out of %d for unique IPs.", i, len(ips))
		}
		if _, ok := checkMap[ip.String()]; !ok {
			checkMap[ip.String()] = true
			toReturn = append(toReturn, ip)
		}
	}
	return toReturn
}

func WriteIPsToHexFile(filePath string, addrs []*net.IP) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0644)
	writer := bufio.NewWriter(file)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, addr := range addrs {
		writer.WriteString(fmt.Sprintf("%s\n", addr.String()))
	}
	writer.Flush()
	return nil
}

func GetTextLinesFromIPs(addrs []*net.IP) string {
	var toReturn []string
	for _, addr := range addrs {
		toReturn = append(toReturn, fmt.Sprintf("%s\n", addr.String()))
	}
	return strings.Join(toReturn, "")
}

func ReadIPsFromBinaryFile(filePath string) ([]*net.IP, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fileInfo.Size()
	if fileSize%16 != 0 {
		return nil, errors.New(fmt.Sprintf("Expected file size to be a multiple of 16 (got %d).", fileSize))
	}
	buffer := make([]byte, 16)
	var toReturn []*net.IP
	for {
		_, err := file.Read(buffer)
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}
		ipBytes := make([]byte, 16)
		copy(ipBytes, buffer)
		newIP := net.IP(ipBytes)
		toReturn = append(toReturn, &newIP)
	}
	return toReturn, nil
}

func WriteIPsToBinaryFile(filePath string, addrs []*net.IP) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)

	for _, addr := range addrs {
		writer.Write(*addr)
	}
	writer.Flush()
	return nil
}

func WriteIPsToFatHexFile(filePath string, addrs []*net.IP) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0644)
	writer := bufio.NewWriter(file)
	if err != nil {
		return err
	}
	defer file.Close()
	buffer := make([]byte, 32)
	for _, addr := range addrs {
		hex.Encode(buffer, *addr)
		writer.Write(buffer)
		writer.Write([]byte("\n"))
	}
	writer.Flush()
	return nil
}

func GetNybbleFromIP(ip *net.IP, index int) uint8 {
	// TODO fatal error if index > 31
	byteIndex := index / 2
	addrBytes := ([]byte)(*ip)
	addrByte := addrBytes[byteIndex]
	if index%2 == 0 {
		return addrByte >> 4
	} else {
		return addrByte & 0xf
	}
}

func GetNybblesFromIP(ip *net.IP, nybbleCount int) []uint8 {
	var toReturn []uint8
	for i := 0; i < nybbleCount; i++ {
		toReturn = append(toReturn, GetNybbleFromIP(ip, i))
	}
	return toReturn
}

func NybblesToIP(nybbles []uint8) *net.IP {
	var bytes []byte
	var curByte byte = 0x0
	for i, curNybble := range nybbles {
		if i%2 == 0 {
			curByte ^= curNybble << 4
		} else {
			curByte ^= curNybble
			bytes = append(bytes, curByte)
			curByte = 0x0
		}
	}
	newIP := net.IP(bytes)
	return &newIP
}

func GenerateRandomAddress() *net.IP {
	bytes := zrandom.GenerateHostBits(128)
	toReturn := net.IP(bytes)
	return &toReturn
}

func FlipBitsInAddress(toFlip *net.IP, startIndex uint8, endIndex uint8) *net.IP {
	toFlipBytes := *toFlip
	endIndex++
	startByte := startIndex / 8
	startOffset := startIndex % 8
	endByte := endIndex / 8
	endOffset := endIndex % 8
	var maskBytes []byte
	var flipBytes []byte
	var i uint8

	if startByte == endByte {
		for i = 0; i < 16; i++ {
			if i == startByte {
				firstHalf := byte(^(0xff >> startOffset))
				secondHalf := byte(0xff >> endOffset)
				maskBytes = append(maskBytes, firstHalf|secondHalf)
			} else {
				maskBytes = append(maskBytes, 0xff)
			}
		}
	} else {
		for i = 0; i < 16; i++ {
			if i < startByte {
				maskBytes = append(maskBytes, 0xff)
			} else if i == startByte {
				maskBytes = append(maskBytes, byte(^(0xff >> startOffset)))
			} else if i < endByte {
				maskBytes = append(maskBytes, 0x00)
			} else if i == endByte {
				maskBytes = append(maskBytes, byte(0xff>>endOffset))
			} else {
				maskBytes = append(maskBytes, 0xff)
			}
		}
	}

	for i = 0; i < 16; i++ {
		flippedBits := ^toFlipBytes[i] & ^maskBytes[i]
		flipBytes = append(flipBytes, toFlipBytes[i]&maskBytes[i]|flippedBits)
	}

	toReturn := net.IP(flipBytes)
	return &toReturn

}

func AddressToUints(toProcess net.IP) (uint64, uint64) {
	var first uint64 = 0
	var second uint64 = 0
	for i := range toProcess {
		if i < 8 {
			first ^= uint64(toProcess[i]) << ((7 - uint(i)) * 8)
		} else {
			second ^= uint64(toProcess[i]) << ((15 - uint(i)) * 8)
		}
	}
	return first, second
}

func UintsToAddress(first uint64, second uint64) *net.IP {
	var addrBytes []byte
	processBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(processBytes, first)
	addrBytes = append(addrBytes, processBytes...)
	binary.BigEndian.PutUint64(processBytes, second)
	addrBytes = append(addrBytes, processBytes...)
	newIP := net.IP(addrBytes)
	return &newIP
}
