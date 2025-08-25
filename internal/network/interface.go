package network

import (
	"fmt"
	"net"
	"strings"
)

type InterfaceInfo struct {
	Name         string
	HardwareAddr string
	IPs          []net.IP
	MTU          int
	Flags        net.Flags
	IsUp         bool
	IsLoopback   bool
}

func GetNetworkInterfaces() ([]InterfaceInfo, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	var result []InterfaceInfo
	for _, iface := range interfaces {
		info := InterfaceInfo{
			Name:         iface.Name,
			HardwareAddr: iface.HardwareAddr.String(),
			MTU:          iface.MTU,
			Flags:        iface.Flags,
			IsUp:         iface.Flags&net.FlagUp != 0,
			IsLoopback:   iface.Flags&net.FlagLoopback != 0,
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				info.IPs = append(info.IPs, ipNet.IP)
			}
		}

		result = append(result, info)
	}

	return result, nil
}

func GetInterfaceByName(name string) (*InterfaceInfo, error) {
	interfaces, err := GetNetworkInterfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		if iface.Name == name {
			return &iface, nil
		}
	}

	return nil, fmt.Errorf("interface %s not found", name)
}

func GetActiveInterface() (*InterfaceInfo, error) {
	_, ifaceName, err := GetDefaultGateway()
	if err != nil {
		return nil, fmt.Errorf("failed to get default gateway: %w", err)
	}

	return GetInterfaceByName(ifaceName)
}

func GetWiFiInterface() (*InterfaceInfo, error) {
	interfaces, err := GetNetworkInterfaces()
	if err != nil {
		return nil, err
	}

	wifiPrefixes := []string{"wlan", "wlp", "en1", "en0"}
	
	for _, iface := range interfaces {
		if !iface.IsUp || iface.IsLoopback {
			continue
		}

		for _, prefix := range wifiPrefixes {
			if strings.HasPrefix(iface.Name, prefix) {
				return &iface, nil
			}
		}
	}

	return nil, fmt.Errorf("no WiFi interface found")
}

func (info *InterfaceInfo) HasIPv4() bool {
	for _, ip := range info.IPs {
		if ip.To4() != nil {
			return true
		}
	}
	return false
}

func (info *InterfaceInfo) HasIPv6() bool {
	for _, ip := range info.IPs {
		if ip.To4() == nil && !ip.IsLoopback() {
			return true
		}
	}
	return false
}

func (info *InterfaceInfo) GetIPv4Addresses() []net.IP {
	var ipv4s []net.IP
	for _, ip := range info.IPs {
		if ip.To4() != nil {
			ipv4s = append(ipv4s, ip)
		}
	}
	return ipv4s
}

func (info *InterfaceInfo) GetIPv6Addresses() []net.IP {
	var ipv6s []net.IP
	for _, ip := range info.IPs {
		if ip.To4() == nil && !ip.IsLoopback() {
			ipv6s = append(ipv6s, ip)
		}
	}
	return ipv6s
}