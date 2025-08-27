package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
)

type IPSet struct {
	Networks []net.IPNet
	mutex    sync.RWMutex
}

func NewIPSet() *IPSet {
	return &IPSet{
		Networks: make([]net.IPNet, 0),
	}
}

// parseIPLines parses IP network lines from a slice of strings
func parseIPLines(lines []string) (*IPSet, error) {
	networks := make([]net.IPNet, 0, len(lines))

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		_, network, err := net.ParseCIDR(line)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR at line %d: %s: %w", lineNum+1, line, err)
		}

		networks = append(networks, *network)
	}

	return &IPSet{Networks: networks}, nil
}

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

	return parseIPLines(lines)
}

func (ip *IPSet) Contains(addr net.IP) bool {
	ip.mutex.RLock()
	defer ip.mutex.RUnlock()

	for _, network := range ip.Networks {
		if network.Contains(addr) {
			return true
		}
	}
	return false
}

func (ip *IPSet) Size() int {
	ip.mutex.RLock()
	defer ip.mutex.RUnlock()
	return len(ip.Networks)
}

func (ip *IPSet) Add(network net.IPNet) {
	ip.mutex.Lock()
	defer ip.mutex.Unlock()
	ip.Networks = append(ip.Networks, network)
}

func (ip *IPSet) Clear() {
	ip.mutex.Lock()
	defer ip.mutex.Unlock()
	ip.Networks = ip.Networks[:0]
}

func (ip *IPSet) GetNetworks() []net.IPNet {
	ip.mutex.RLock()
	defer ip.mutex.RUnlock()

	networks := make([]net.IPNet, len(ip.Networks))
	copy(networks, ip.Networks)
	return networks
}