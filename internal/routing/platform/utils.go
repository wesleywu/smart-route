package platform

import (
	"fmt"
	"net"
	"strings"

	"github.com/wesleywu/smart-route/internal/routing/entities"
)

// routesMatch checks if two networks are equivalent
func routesMatch(net1, net2 net.IPNet) bool {
	return net1.IP.Equal(net2.IP) &&
		len(net1.Mask) == len(net2.Mask) &&
		net1.Mask.String() == net2.Mask.String()
}

func parseNetstatOutput(output string) ([]*entities.Route, error) {
	var routes []*entities.Route
	lines := strings.Split(output, "\n")

	// Skip header lines and find the start of routing table
	start := -1
	for i, line := range lines {
		if strings.Contains(line, "Destination") && strings.Contains(line, "Gateway") {
			start = i + 1
			break
		}
	}

	if start == -1 {
		// Try alternative header format
		for i, line := range lines {
			if strings.Contains(line, "Internet:") {
				start = i + 2 // Skip "Internet:" and the header line
				break
			}
		}
	}

	if start == -1 {
		return routes, nil // No routing table found
	}

	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Skip non-route lines (like "Internet6:" section)
		if strings.Contains(line, ":") && !strings.Contains(line, ".") {
			break
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		destination := fields[0]
		gateway := fields[1]

		// Parse fields based on standard netstat format:
		// Destination Gateway Flags Netif [Expire]
		// Skip if no gateway
		if gateway == "" || gateway == "-" {
			continue
		}

		// Parse destination network
		network, err := parseDestination(destination)
		if err != nil {
			continue // Skip unparseable destinations
		}

		// Parse gateway IP
		gwIP := net.ParseIP(gateway)
		if gwIP == nil {
			continue // Skip unparseable gateways (like link# formats)
		}

		route := &entities.Route{
			Destination: *network,
			Gateway:     gwIP,
		}

		routes = append(routes, route)
	}

	return routes, nil
}

// parseDestination parses various destination formats from netstat
func parseDestination(dest string) (*net.IPNet, error) {
	// Handle special destinations
	if dest == "default" {
		_, network, _ := net.ParseCIDR("0.0.0.0/0")
		return network, nil
	}

	// Handle CIDR notation (e.g., "192.168.1.0/24" or "114.114.114.114/32")
	if strings.Contains(dest, "/") {
		// Handle netstat's simplified format like "1.0.1/24" -> "1.0.1.0/24"
		parts := strings.Split(dest, "/")
		if len(parts) == 2 {
			ip := parts[0]
			mask := parts[1]

			// Count dots in IP part
			dotCount := strings.Count(ip, ".")

			// Add missing octets to make it a valid IP
			switch dotCount {
			case 0: // "1/24" -> "1.0.0.0/24"
				ip = ip + ".0.0.0"
			case 1: // "1.0/24" -> "1.0.0.0/24"
				ip = ip + ".0.0"
			case 2: // "1.0.1/24" -> "1.0.1.0/24"
				ip = ip + ".0"
			case 3: // "1.0.1.0/24" - already complete
				// no change needed
			}

			dest = ip + "/" + mask
		}

		_, network, err := net.ParseCIDR(dest)
		return network, err
	}

	// Handle single IP addresses (assume /32 for IPv4, /128 for IPv6)
	ip := net.ParseIP(dest)
	if ip != nil {
		if ip.To4() != nil {
			// IPv4
			return &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(32, 32),
			}, nil
		}
		if ip.To16() != nil {
			// IPv6
			return &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(128, 128),
			}, nil
		}
	}

	// Handle incomplete network addresses without explicit mask (e.g., "203.26.55" -> "203.26.55.0/24")
	if strings.Contains(dest, ".") {
		dotCount := strings.Count(dest, ".")
		if dotCount < 3 {
			// Add missing octets and assume appropriate network mask
			switch dotCount {
			case 1: // "203.26" -> "203.26.0.0/16"
				dest = dest + ".0.0"
				return &net.IPNet{
					IP:   net.ParseIP(dest),
					Mask: net.CIDRMask(16, 32),
				}, nil
			case 2: // "203.26.55" -> "203.26.55.0/24"
				dest = dest + ".0"
				return &net.IPNet{
					IP:   net.ParseIP(dest),
					Mask: net.CIDRMask(24, 32),
				}, nil
			}
		}
	}

	// Assume /32 for what looks like a complete IP
	testIP := net.ParseIP(dest)
	if testIP != nil {
		if testIP.To4() != nil {
			return &net.IPNet{
				IP:   testIP,
				Mask: net.CIDRMask(32, 32),
			}, nil
		}
	}

	return nil, fmt.Errorf("unsupported destination format: %s", dest)
}
