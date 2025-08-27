package config

import (
	_ "embed"
	"strings"
)

//go:embed chndns.txt
var embeddedDNSData string

//go:embed chnroute.txt  
var embeddedRouteData string

// GetEmbeddedDNSServers returns DNS servers from embedded data
func GetEmbeddedDNSServers() (*DNSServers, error) {
	lines := strings.Split(strings.TrimSpace(embeddedDNSData), "\n")
	return parseDNSLines(lines)
}

// GetEmbeddedRoutes returns IP routes from embedded data
func GetEmbeddedRoutes() (*IPSet, error) {
	lines := strings.Split(strings.TrimSpace(embeddedRouteData), "\n")
	return parseIPLines(lines)
}

// LoadChnDNSWithFallback loads DNS servers from file, falls back to embedded data
func LoadChnDNSWithFallback(filename string) (*DNSServers, error) {
	if filename != "" {
		// Try to load from external file first
		if dns, err := LoadChnDNS(filename); err == nil {
			return dns, nil
		}
	}
	
	// Fall back to embedded data
	return GetEmbeddedDNSServers()
}

// LoadChnRoutesWithFallback loads routes from file, falls back to embedded data  
func LoadChnRoutesWithFallback(filename string) (*IPSet, error) {
	if filename != "" {
		// Try to load from external file first
		if routes, err := LoadChnRoutes(filename); err == nil {
			return routes, nil
		}
	}
	
	// Fall back to embedded data
	return GetEmbeddedRoutes()
}