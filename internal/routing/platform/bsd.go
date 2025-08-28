//go:build darwin || freebsd

// Package platform provides platform-specific route manager implementations
package platform

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wesleywu/smart-route/internal/logger"
	"github.com/wesleywu/smart-route/internal/routing/entities"
	"github.com/wesleywu/smart-route/internal/routing/metrics"
	"golang.org/x/sys/unix"
)

// BSDRouteManager is a route manager for BSD-based systems
type BSDRouteManager struct {
	socket           int
	mutex            sync.Mutex
	concurrencyLimit int
	maxRetries       int
	metrics          *metrics.Metrics
	seqNum           int32 // Add sequence number counter
}

// NewPlatformRouteManager creates a platform-specific route manager (BSD implementation)
func NewPlatformRouteManager(concurrencyLimit, maxRetries int) (entities.RouteManager, error) {
	sock, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return nil, fmt.Errorf("failed to create route socket: %w", err)
	}

	return &BSDRouteManager{
		socket:           sock,
		concurrencyLimit: concurrencyLimit,
		maxRetries:       maxRetries,
		metrics:          metrics.NewMetrics(),
		seqNum:           1, // Initialize sequence number
	}, nil
}

// AddRoute adds a route to the system
func (rm *BSDRouteManager) AddRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.addRouteWithRetry(network, gateway, log)
}

// DeleteRoute deletes a route from the system
func (rm *BSDRouteManager) DeleteRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.deleteRouteWithRetry(network, gateway, log)
}

// BatchAddRoutes adds multiple routes to the system
func (rm *BSDRouteManager) BatchAddRoutes(routes []*entities.Route, log *logger.Logger) error {
	return rm.batchOperation(routes, entities.RouteActionAdd, log)
}

// BatchDeleteRoutes deletes multiple routes from the system
func (rm *BSDRouteManager) BatchDeleteRoutes(routes []*entities.Route, log *logger.Logger) error {
	return rm.batchOperation(routes, entities.RouteActionDelete, log)
}

// GetPhysicalGateway gets the physical gateway from the system (for route management)
func (rm *BSDRouteManager) GetPhysicalGateway() (net.IP, string, error) {
	// ALWAYS look for physical interface gateway, never rely on default route
	// In VPN scenarios, default route will point to VPN, but we need the physical gateway
	return rm.getPhysicalGateway()
}

// GetSystemDefaultRoute gets the current default route (including VPN) from the system
func (rm *BSDRouteManager) GetSystemDefaultRoute() (net.IP, string, error) {
	// Use 'route get default' to get the actual current default route
	cmd := exec.Command("route", "-n", "get", "default")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get current default route: %w", err)
	}

	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")
	var gateway net.IP
	var iface string

	// Remove debug output - not needed in production

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				gateway = net.ParseIP(parts[1])
			}
		} else if strings.HasPrefix(line, "interface:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				iface = parts[1]
			}
		}
	}

	// Enhanced error handling: if interface is missing, try to get it from physical gateway
	if iface == "" {
		// During network transitions, route output might be incomplete
		// Try to fall back to physical gateway information
		physGW, physIface, physErr := rm.getPhysicalGateway()
		if physErr == nil && physIface != "" {
			// Use physical interface as fallback, but keep the current gateway if found
			if gateway == nil {
				gateway = physGW
			}
			iface = physIface
			// Using physical gateway as fallback during route transition
		} else {
			return nil, "", fmt.Errorf("failed to parse interface from current default route - route table may be in transition")
		}
	}

	// For VPN interfaces, there might not be a gateway (direct connection)
	// In this case, we can use a placeholder IP or the interface's IP
	if gateway == nil {
		// For VPN/TUN interfaces, use a placeholder gateway IP
		gateway = net.ParseIP("0.0.0.0") // Indicates direct connection
	}

	return gateway, iface, nil
}

// ListSystemRoutes gets all routes from the system
func (rm *BSDRouteManager) ListSystemRoutes() ([]*entities.Route, error) {
	cmd := exec.Command("netstat", "-rn")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}

	return parseNetstatOutput(string(output))
}

// Close closes the route manager
func (rm *BSDRouteManager) Close() error {
	return unix.Close(rm.socket)
}

// addRouteWithRetry adds a route to the system with retry logic
func (rm *BSDRouteManager) addRouteWithRetry(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	var lastErr error
	start := time.Now()

	for attempt := 0; attempt < rm.maxRetries; attempt++ {
		err := rm.addRouteNative(network, gateway, log)
		if err == nil {
			rm.metrics.RecordOperation(time.Since(start), true)
			return nil
		}

		if routeErr, ok := err.(*entities.RouteOperationError); ok && !routeErr.IsRetryable() {
			rm.metrics.RecordOperation(time.Since(start), false)
			return err
		}

		lastErr = err
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}

	rm.metrics.RecordOperation(time.Since(start), false)
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// deleteRouteWithRetry deletes a route from the system with retry logic
func (rm *BSDRouteManager) deleteRouteWithRetry(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	var lastErr error
	start := time.Now()

	for attempt := 0; attempt < rm.maxRetries; attempt++ {
		err := rm.deleteRouteNative(network, gateway, log)

		if err == nil {
			rm.metrics.RecordOperation(time.Since(start), true)
			return nil
		}

		if routeErr, ok := err.(*entities.RouteOperationError); ok && !routeErr.IsRetryable() {
			rm.metrics.RecordOperation(time.Since(start), false)
			return err
		}

		lastErr = err
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}

	rm.metrics.RecordOperation(time.Since(start), false)
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (rm *BSDRouteManager) batchOperation(routes []*entities.Route, action entities.RouteAction, log *logger.Logger) error {
	semaphore := make(chan struct{}, rm.concurrencyLimit)
	var wg sync.WaitGroup
	errChan := make(chan error, len(routes))

	for _, route := range routes {
		wg.Add(1)
		go func(r *entities.Route) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			var err error
			switch action {
			case entities.RouteActionAdd:
				err = rm.AddRoute(&r.Destination, r.Gateway, log)
			case entities.RouteActionDelete:
				err = rm.DeleteRoute(&r.Destination, r.Gateway, log)
			}

			if err != nil {
				errChan <- err
			}
		}(route)
	}

	wg.Wait()
	close(errChan)

	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("batch operation failed: %d errors", len(errors))
	}

	return nil
}

// getPhysicalGateway gets the physical gateway for macOS/BSD systems
func (rm *BSDRouteManager) getPhysicalGateway() (net.IP, string, error) {
	// Strategy 1: First try to get gateway from active network interface (most reliable for detecting changes)
	gateway, iface, err := rm.getGatewayFromInterfaces()
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
			if !rm.isPhysicalInterface(iface) {
				continue
			}

			// Skip link-local gateways
			if strings.HasPrefix(gatewayStr, "link#") {
				continue
			}

			// Check if this is a valid IP gateway
			gateway := net.ParseIP(gatewayStr)
			if gateway != nil && rm.isPrivateIP(gateway) {
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

// isPhysicalInterface checks if the interface is a physical interface
func (rm *BSDRouteManager) isPhysicalInterface(iface string) bool {
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

// isPrivateIP checks if the IP is a private IP
func (rm *BSDRouteManager) isPrivateIP(ip net.IP) bool {
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

// getGatewayFromInterfaces gets the gateway from interfaces
func (rm *BSDRouteManager) getGatewayFromInterfaces() (net.IP, string, error) {
	// Get active physical interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	// Find the primary active physical interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && rm.isPhysicalInterface(iface.Name) {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}

			// Check if this interface has a valid IP
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ip4 := ipnet.IP.To4(); ip4 != nil && rm.isPrivateIP(ip4) {
						// Calculate gateway from subnet
						// Most networks use .1 as gateway
						gateway := rm.calculateGatewayFromSubnet(ipnet)
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
func (rm *BSDRouteManager) calculateGatewayFromSubnet(ipnet *net.IPNet) net.IP {
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
