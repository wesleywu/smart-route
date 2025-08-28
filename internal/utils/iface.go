package utils

import "strings"

// IsVPNInterface checks if the given interface name is a VPN interface
func IsVPNInterface(interfaceName string) bool {
	// Common VPN interface patterns
	if len(interfaceName) >= 4 {
		prefix := interfaceName[:4]
		switch prefix {
		case "utun", "tun0", "tap0":
			return true
		}
	}

	// Check for other common VPN interface patterns
	if len(interfaceName) >= 3 {
		prefix := interfaceName[:3]
		switch prefix {
		case "tun", "tap", "ppp":
			return true
		}
	}

	return false
}

// IsPhysicalInterface checks if the interface is a physical interface
func IsPhysicalInterface(iface string) bool {
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
