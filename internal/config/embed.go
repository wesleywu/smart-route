package config

import (
	_ "embed"
	"net"
	"strings"
)

//go:embed chndns.txt
var embeddedDNSData string

//go:embed chnroute.txt  
var embeddedRouteData string

// GetEmbeddedDNSServers returns DNS servers from embedded data
func GetEmbeddedDNSServers() ([]*net.IPNet, error) {
	lines := strings.Split(strings.TrimSpace(embeddedDNSData), "\n")
	return parseDNSLines(lines)
}

// GetEmbeddedRoutes returns IP routes from embedded data
func GetEmbeddedRoutes() (*IPSet, error) {
	lines := strings.Split(strings.TrimSpace(embeddedRouteData), "\n")
	ipSet := NewIPSet()
	if err := ipSet.parseIPLines(lines); err != nil {
		return nil, err
	}
	return ipSet, nil
}

// LoadManagedIPSetWithFallback loads IP routes and DNS servers from file, falls back to embedded data
func LoadManagedIPSetWithFallback(routesFile string, dnsFile string) (*IPSet, error) {
	var ipSet *IPSet
	var dnsServers []*net.IPNet
	var err error
	if routesFile != "" {
		// Try to load from external file first
		ipSet, err = LoadChnRoutes(routesFile)
		if err != nil {
			return nil, err
		}
	} else {
		ipSet, err = GetEmbeddedRoutes()
		if err != nil {
			return nil, err
		}
	}

	if dnsFile != "" {
		dnsServers, err = LoadChnDNS(dnsFile)
		if err != nil {
			return nil, err
		}
	} else {
		dnsServers, err = GetEmbeddedDNSServers()
		if err != nil {
			return nil, err
		}
	}

	for _, dnsIPNet := range dnsServers {
		ipSet.Add(dnsIPNet)
	}

	return ipSet, nil
}
