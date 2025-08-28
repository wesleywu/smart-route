package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/wesleywu/smart-route/internal/utils"
)


// parseDNSLines parses DNS server lines from a slice of strings
func parseDNSLines(lines []string) ([]*net.IPNet, error) {
	ips := make([]*net.IPNet, 0, len(lines))
	
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		ip := net.ParseIP(line)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address at line %d: %s", lineNum+1, line)
		}

		ips = append(ips, utils.ToIPNet(ip))
	}

	return ips, nil
}

// LoadChnDNS loads DNS servers from a file
func LoadChnDNS(file string) ([]*net.IPNet, error) {
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

	return parseDNSLines(lines)
}
