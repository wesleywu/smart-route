package network

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func GetDefaultGateway() (net.IP, string, error) {
	switch runtime.GOOS {
	case "darwin":
		return getDefaultGatewayDarwin()
	case "linux":
		return getDefaultGatewayLinux()
	case "windows":
		return getDefaultGatewayWindows()
	default:
		return nil, "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func getDefaultGatewayDarwin() (net.IP, string, error) {
	// ALWAYS look for physical interface gateway, never rely on default route
	// In VPN scenarios, default route will point to VPN, but we need the physical gateway
	return getPhysicalGatewayDarwin()
}

func getPhysicalGatewayDarwin() (net.IP, string, error) {
	// Strategy 1: First try to get gateway from active network interface (most reliable for detecting changes)
	gateway, iface, err := getGatewayFromInterfaces()
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
			if !isPhysicalInterface(iface) {
				continue
			}
			
			// Skip link-local gateways
			if strings.HasPrefix(gatewayStr, "link#") {
				continue
			}
			
			// Check if this is a valid IP gateway
			gateway := net.ParseIP(gatewayStr)
			if gateway != nil && isPrivateIP(gateway) {
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

func isPhysicalInterface(iface string) bool {
	// Physical interfaces: en0, en1, eth0, eth1, etc.
	// Skip VPN: utun, tun, tap, ppp, ipsec, etc.
	// Skip system: lo, awdl, bridge, etc.
	
	if strings.HasPrefix(iface, "en") || strings.HasPrefix(iface, "eth") {
		return true
	}
	
	// Skip VPN interfaces
	vpnPrefixes := []string{"utun", "tun", "tap", "ppp", "ipsec", "wg"}
	for _, prefix := range vpnPrefixes {
		if strings.HasPrefix(iface, prefix) {
			return false
		}
	}
	
	// Skip system interfaces
	systemPrefixes := []string{"lo", "awdl", "bridge", "gif", "stf"}
	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(iface, prefix) {
			return false
		}
	}
	
	return false
}

func isPrivateIP(ip net.IP) bool {
	// Check if IP is in private ranges: 10.x.x.x, 172.16-31.x.x, 192.168.x.x
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
	}
	return false
}

func getGatewayFromInterfaces() (net.IP, string, error) {
	// Get active physical interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get network interfaces: %w", err)
	}
	
	// Find the primary active physical interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && isPhysicalInterface(iface.Name) {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			
			// Check if this interface has a valid IP
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ip4 := ipnet.IP.To4(); ip4 != nil && isPrivateIP(ip4) {
						// Calculate gateway from subnet
						// Most networks use .1 as gateway, but let's also check ARP table
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

func getDefaultGatewayLinux() (net.IP, string, error) {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get default route: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "default" && fields[1] == "via" {
			gateway := net.ParseIP(fields[2])
			if gateway == nil {
				continue
			}
			
			var iface string
			for i, field := range fields {
				if field == "dev" && i+1 < len(fields) {
					iface = fields[i+1]
					break
				}
			}
			
			return gateway, iface, nil
		}
	}

	return nil, "", fmt.Errorf("no default gateway found")
}

func getDefaultGatewayWindows() (net.IP, string, error) {
	cmd := exec.Command("route", "print", "0.0.0.0")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get default route: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "0.0.0.0" && fields[1] == "0.0.0.0" {
			gateway := net.ParseIP(fields[2])
			if gateway == nil {
				continue
			}
			
			ifaceIndex, err := strconv.Atoi(fields[4])
			if err != nil {
				continue
			}
			
			iface := fmt.Sprintf("Interface%d", ifaceIndex)
			return gateway, iface, nil
		}
	}

	return nil, "", fmt.Errorf("no default gateway found")
}

func IsInterfaceUp(ifaceName string) (bool, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range ifaces {
		if iface.Name == ifaceName {
			return iface.Flags&net.FlagUp != 0, nil
		}
	}

	return false, fmt.Errorf("interface %s not found", ifaceName)
}

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
	
	fmt.Printf("DEBUG: IP %s in subnet %s -> calculated gateway %s\n", ip.String(), ipnet.String(), gateway.String())
	
	return gateway
}

func findGatewayFromARP(ipnet *net.IPNet) net.IP {
	// Use ARP table to find the gateway
	// This is more reliable than guessing .1
	cmd := exec.Command("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Look for lines like: ? (192.168.32.1) at xx:xx:xx:xx:xx:xx on en0 [ethernet]
		if strings.Contains(line, "en0") && strings.Contains(line, "ethernet") {
			// Extract IP from parentheses
			start := strings.Index(line, "(")
			end := strings.Index(line, ")")
			if start >= 0 && end > start {
				ipStr := line[start+1 : end]
				ip := net.ParseIP(ipStr)
				if ip != nil && ipnet.Contains(ip) {
					// Additional check: this should be the gateway (usually .1, .254, etc.)
					if isLikelyGateway(ip, ipnet) {
						return ip
					}
				}
			}
		}
	}
	
	return nil
}

func isLikelyGateway(ip net.IP, ipnet *net.IPNet) bool {
	// Check if this IP is likely to be a gateway
	// Gateways are usually .1, .254, or similar "special" addresses
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	
	lastOctet := ip4[3]
	// Common gateway addresses: .1, .254, .2
	return lastOctet == 1 || lastOctet == 254 || lastOctet == 2
}