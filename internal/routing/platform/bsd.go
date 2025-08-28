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
	"github.com/wesleywu/smart-route/internal/routing/batch"
	"github.com/wesleywu/smart-route/internal/routing/entities"
	"github.com/wesleywu/smart-route/internal/routing/metrics"
	"github.com/wesleywu/smart-route/internal/utils"
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
	return batch.Process(routes, rm.AddRoute, rm.concurrencyLimit, log)
}

// BatchDeleteRoutes deletes multiple routes from the system
func (rm *BSDRouteManager) BatchDeleteRoutes(routes []*entities.Route, log *logger.Logger) error {
	return batch.Process(routes, rm.DeleteRoute, rm.concurrencyLimit, log)
}

// GetPhysicalGateway gets the physical gateway from the system (for route management)
func (rm *BSDRouteManager) GetPhysicalGateway() (net.IP, string, error) {
	// ALWAYS look for physical interface gateway, never rely on default route
	// In VPN scenarios, default route will point to VPN, but we need the physical gateway
	return utils.GetPhysicalGatewayBSD()
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
		physGW, physIface, physErr := utils.GetPhysicalGatewayBSD()
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

	return parseNetstatOutputBSD(string(output))
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

// parseNetstatOutputBSD parses the output of netstat -rn for BSD systems
func parseNetstatOutputBSD(output string) ([]*entities.Route, error) {
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
		network, err := utils.ParseDestination(destination)
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
