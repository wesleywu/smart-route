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
	// First try to get the physical interface gateway (non-VPN)
	gateway, iface, err := getPhysicalGatewayDarwin()
	if err == nil {
		return gateway, iface, nil
	}
	
	// Fallback to default route parsing
	cmd := exec.Command("route", "-n", "get", "default")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get default route: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	var detectedGateway net.IP
	var detectedIface string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			gatewayStr := strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
			detectedGateway = net.ParseIP(gatewayStr)
			if detectedGateway == nil {
				return nil, "", fmt.Errorf("invalid gateway IP: %s", gatewayStr)
			}
		}
		if strings.HasPrefix(line, "interface:") {
			detectedIface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}

	if detectedGateway == nil {
		return nil, "", fmt.Errorf("no default gateway found")
	}

	return detectedGateway, detectedIface, nil
}

func getPhysicalGatewayDarwin() (net.IP, string, error) {
	cmd := exec.Command("netstat", "-rn")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get routing table: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "default" {
			gatewayStr := fields[1]
			iface := fields[3]
			
			// Skip VPN interfaces (utun, tun, tap, etc.)
			if strings.HasPrefix(iface, "utun") || strings.HasPrefix(iface, "tun") || 
			   strings.HasPrefix(iface, "tap") || strings.HasPrefix(iface, "ppp") {
				continue
			}
			
			// Skip link-local gateways
			if strings.HasPrefix(gatewayStr, "link#") {
				continue
			}
			
			gateway := net.ParseIP(gatewayStr)
			if gateway != nil {
				return gateway, iface, nil
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