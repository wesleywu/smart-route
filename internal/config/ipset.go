package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// IPSet is a set of IP networks
type IPSet struct {
	ipNets map[uint64]*net.IPNet // maps network hash to network pointer
}

// NewIPSet creates a new IPSet
func NewIPSet() *IPSet {
	return &IPSet{
		ipNets: make(map[uint64]*net.IPNet),
	}
}

// parseIPLines parses IP network lines from a slice of strings
func (is *IPSet) parseIPLines(lines []string) error {
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		_, network, err := net.ParseCIDR(line)
		if err != nil {
			return fmt.Errorf("invalid CIDR at line %d: %s: %w", lineNum+1, line, err)
		}

		hash := hashIPNet(*network)
		is.ipNets[hash] = network
	}

	return nil
}

// LoadChnRoutes loads all China ip net
func LoadChnRoutes(file string) (*IPSet, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", file, err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", file, err)
	}

	ipSet := NewIPSet()
	if err := ipSet.parseIPLines(lines); err != nil {
		return nil, err
	}
	return ipSet, nil
}

// Size returns the number of networks in the IPSet
func (is *IPSet) Size() int {
	return len(is.ipNets)
}

// IPNets returns a copy of the networks in the IPSet
func (is *IPSet) IPNets() map[uint64]*net.IPNet {
	return is.ipNets
}

// Add adds a network to the set
func (is *IPSet) Add(ipNet *net.IPNet) bool {
	if ipNet == nil {
		return false
	}
	hash := hashIPNet(*ipNet)

	// Check if network already exists
	if _, exists := is.ipNets[hash]; exists {
		return false
	}

	is.ipNets[hash] = ipNet
	return true
}

// ContainsIPNet checks if the set contains a network
func (is *IPSet) ContainsIPNet(ipNet net.IPNet) bool {
	hash := hashIPNet(ipNet)
	_, exists := is.ipNets[hash]
	return exists
}

// Hash returns the hash code for this Route, only based on Destination network
func hashIPNet(ip net.IPNet) uint64 {
	// Create hasher instance
	h := xxhash.New()

	// Hash the destination IP (convert to 4-byte representation for IPv4)
	ip4 := ip.IP.To4()
	if ip4 != nil {
		// IPv4: use the 4-byte representation
		_, _ = h.Write(ip4)
	} else {
		// IPv6: use the full 16-byte representation
		_, _ = h.Write(ip.IP.To16())
	}

	// Hash the network mask information
	maskBytes, maskBits := ip.Mask.Size()
	_, _ = h.Write([]byte{byte(maskBytes), byte(maskBits)})

	return h.Sum64()
}
