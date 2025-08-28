package utils

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// GetPhysicalGatewayBSD gets the physical gateway for macOS/BSD/Linux systems
func GetPhysicalGatewayBSD() (net.IP, string, error) {
	// Strategy 1: First try to get gateway from active network interface (most reliable for detecting changes)
	gateway, iface, err := GetGatewayFromInterfaces()
	if err == nil {
		return gateway, iface, nil
	}

	// Strategy 2: If interface method fails, fall back to route table analysis
	cmd := exec.Command("netstat", "-rn")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get routing table: %w", err)
	}

	lines := strings.Split(string(output), "\n")

	// Look for any existing routes through physical interfaces
	gatewayCount := make(map[string]int)
	gatewayToIface := make(map[string]string)

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			gatewayStr := fields[1]
			iface := fields[3]

			// Only consider physical interfaces (typically en0, en1, eth0, etc.)
			if !IsPhysicalInterface(iface) {
				continue
			}

			// Skip link-local gateways
			if strings.HasPrefix(gatewayStr, "link#") {
				continue
			}

			// Check if this is a valid IP gateway
			gateway := net.ParseIP(gatewayStr)
			if gateway != nil && IsPrivateIP(gateway) {
				gatewayCount[gatewayStr]++
				gatewayToIface[gatewayStr] = iface
			}
		}
	}

	// Find the most commonly used physical gateway
	var bestGateway string
	maxCount := 0
	for gw, count := range gatewayCount {
		if count > maxCount {
			maxCount = count
			bestGateway = gw
		}
	}

	if bestGateway != "" {
		return net.ParseIP(bestGateway), gatewayToIface[bestGateway], nil
	}

	return nil, "", fmt.Errorf("no physical gateway found")
}

// GetGatewayFromInterfaces gets the gateway from interfaces
func GetGatewayFromInterfaces() (net.IP, string, error) {
	// Get active physical interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	// Find the primary active physical interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && IsPhysicalInterface(iface.Name) {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}

			// Check if this interface has a valid IP
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ip4 := ipnet.IP.To4(); ip4 != nil && IsPrivateIP(ip4) {
						// Calculate gateway from subnet
						// Most networks use .1 as gateway
						gateway := calculateGatewayFromSubnet(ipnet)
						if gateway != nil {
							return gateway, iface.Name, nil
						}
					}
				}
			}
		}
	}

	return nil, "", fmt.Errorf("no physical gateway found")
}

// calculateGatewayFromSubnet calculates the gateway from the subnet
func calculateGatewayFromSubnet(ipnet *net.IPNet) net.IP {
	ip := ipnet.IP.To4()
	if ip == nil {
		return nil
	}

	// For immediate detection, calculate likely gateway (.1 in the network)
	// This is fast and works for most networks
	network := ip.Mask(ipnet.Mask)
	gateway := make(net.IP, 4)
	copy(gateway, network)
	gateway[3] = 1

	return gateway
}
