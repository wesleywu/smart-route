package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
)

type DNSServers struct {
	IPs   []net.IP
	mutex sync.RWMutex
}

func NewDNSServers() *DNSServers {
	return &DNSServers{
		IPs: make([]net.IP, 0),
	}
}

// parseDNSLines parses DNS server lines from a slice of strings
func parseDNSLines(lines []string) (*DNSServers, error) {
	ips := make([]net.IP, 0, len(lines))
	
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		ip := net.ParseIP(line)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address at line %d: %s", lineNum+1, line)
		}

		ips = append(ips, ip)
	}

	return &DNSServers{IPs: ips}, nil
}

func LoadChnDNS(file string) (*DNSServers, error) {
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

func (dns *DNSServers) Contains(ip net.IP) bool {
	dns.mutex.RLock()
	defer dns.mutex.RUnlock()

	for _, dnsIP := range dns.IPs {
		if dnsIP.Equal(ip) {
			return true
		}
	}
	return false
}

func (dns *DNSServers) Size() int {
	dns.mutex.RLock()
	defer dns.mutex.RUnlock()
	return len(dns.IPs)
}

func (dns *DNSServers) Add(ip net.IP) {
	dns.mutex.Lock()
	defer dns.mutex.Unlock()
	dns.IPs = append(dns.IPs, ip)
}

func (dns *DNSServers) Clear() {
	dns.mutex.Lock()
	defer dns.mutex.Unlock()
	dns.IPs = dns.IPs[:0]
}

func (dns *DNSServers) GetIPs() []net.IP {
	dns.mutex.RLock()
	defer dns.mutex.RUnlock()

	ips := make([]net.IP, len(dns.IPs))
	copy(ips, dns.IPs)
	return ips
}