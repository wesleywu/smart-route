package utils

import (
	"fmt"
	"net"
	"strings"
)

// IsPrivateIP checks if the IP is a private IP
func IsPrivateIP(ip net.IP) bool {
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

// ParseDestination parses various destination formats from netstat
func ParseDestination(dest string) (*net.IPNet, error) {
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

// ToIPNet converts an IP address to a network address
func ToIPNet(ip net.IP) *net.IPNet {
	var ipNet *net.IPNet
	if ip.To4() != nil {
		ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
	} else {
		ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
	}
	return ipNet
}