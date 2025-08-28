//go:build linux

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
)

type LinuxRouteManager struct {
	mutex            sync.Mutex
	concurrencyLimit int
	maxRetries       int
	metrics          *metrics.Metrics
}

// NewPlatformRouteManager creates a platform-specific route manager (Linux implementation)
func NewPlatformRouteManager(concurrencyLimit, maxRetries int) (entities.RouteManager, error) {
	return &LinuxRouteManager{
		concurrencyLimit: concurrencyLimit,
		maxRetries:       maxRetries,
		metrics:          metrics.NewMetrics(),
	}, nil
}

func (rm *LinuxRouteManager) AddRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.addRouteWithRetry(network, gateway)
}

func (rm *LinuxRouteManager) DeleteRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.deleteRouteWithRetry(network, gateway)
}

func (rm *LinuxRouteManager) BatchAddRoutes(routes []*entities.Route, log *logger.Logger) error {
	return batch.Process(routes, rm.AddRoute, rm.concurrencyLimit, log)
}

func (rm *LinuxRouteManager) BatchDeleteRoutes(routes []*entities.Route, log *logger.Logger) error {
	return batch.Process(routes, rm.DeleteRoute, rm.concurrencyLimit, log)
}

// GetPhysicalGateway gets the underlying physical network gateway (for route management)
func (rm *LinuxRouteManager) GetPhysicalGateway() (net.IP, string, error) {
	// ALWAYS look for physical interface gateway, never rely on default route
	// In VPN scenarios, default route will point to VPN, but we need the physical gateway
	// TODO: Implement Linux specific physical gateway detection
	return utils.GetPhysicalGatewayBSD()
}

// GetSystemDefaultRoute gets the current default route (including VPN) from the system
func (rm *LinuxRouteManager) GetSystemDefaultRoute() (net.IP, string, error) {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get default route: %w", err)
	}

	return rm.parseDefaultRouteLinux(string(output))
}

// ListSystemRoutes gets all routes from the system routing table
func (rm *LinuxRouteManager) ListSystemRoutes() ([]*entities.Route, error) {
	cmd := exec.Command("netstat", "-rn")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}

	return parseNetstatOutputLinux(string(output))
}

func (rm *LinuxRouteManager) Close() error {
	return nil
}

func (rm *LinuxRouteManager) addRouteWithRetry(network *net.IPNet, gateway net.IP) error {
	var lastErr error
	start := time.Now()

	for attempt := 0; attempt < rm.maxRetries; attempt++ {
		err := rm.addRouteDirect(network, gateway)
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

func (rm *LinuxRouteManager) deleteRouteWithRetry(network *net.IPNet, gateway net.IP) error {
	var lastErr error
	start := time.Now()

	for attempt := 0; attempt < rm.maxRetries; attempt++ {
		err := rm.deleteRouteDirect(network, gateway)
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

func (rm *LinuxRouteManager) addRouteDirect(network *net.IPNet, gateway net.IP) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	cmd := exec.Command("ip", "route", "add", network.String(), "via", gateway.String())
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			switch exitErr.ExitCode() {
			case 1:
				return &entities.RouteOperationError{ErrorType: entities.RouteErrPermission, Destination: *network, Gateway: gateway, Cause: err}
			case 2:
				return &entities.RouteOperationError{ErrorType: entities.RouteErrInvalidRoute, Destination: *network, Gateway: gateway, Cause: err}
			default:
				return &entities.RouteOperationError{ErrorType: entities.RouteErrSystemCall, Destination: *network, Gateway: gateway, Cause: err}
			}
		}
		return &entities.RouteOperationError{ErrorType: entities.RouteErrSystemCall, Destination: *network, Gateway: gateway, Cause: err}
	}

	return nil
}

func (rm *LinuxRouteManager) deleteRouteDirect(network *net.IPNet, gateway net.IP) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	cmd := exec.Command("ip", "route", "del", network.String(), "via", gateway.String())
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 2 {
				return nil
			}
		}
		return &entities.RouteOperationError{ErrorType: entities.RouteErrSystemCall, Destination: *network, Gateway: gateway, Cause: err}
	}

	return nil
}

func (rm *LinuxRouteManager) parseDefaultRouteLinux(output string) (net.IP, string, error) {
	// Parse "default via 192.168.1.1 dev eth0" format
	lines := strings.Split(output, "\n")
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

// parseNetstatOutputLinux parses the output of netstat -rn for Linux systems
func parseNetstatOutputLinux(output string) ([]*entities.Route, error) {
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
