//go:build windows

package platform

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wesleywu/smart-route/internal/logger"
	"github.com/wesleywu/smart-route/internal/routing/batch"
	"github.com/wesleywu/smart-route/internal/routing/entities"
	"github.com/wesleywu/smart-route/internal/routing/metrics"
)

type WindowsRouteManager struct {
	mutex            sync.Mutex
	concurrencyLimit int
	maxRetries       int
	metrics          *metrics.Metrics
}

// NewPlatformRouteManager creates a platform-specific route manager (Windows implementation)
func NewPlatformRouteManager(concurrencyLimit, maxRetries int) (entities.RouteManager, error) {
	return &WindowsRouteManager{
		concurrencyLimit: concurrencyLimit,
		maxRetries:       maxRetries,
		metrics:          metrics.NewMetrics(),
	}, nil
}

func (rm *WindowsRouteManager) AddRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.addRouteWithRetry(network, gateway)
}

func (rm *WindowsRouteManager) DeleteRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.deleteRouteWithRetry(network, gateway)
}

func (rm *WindowsRouteManager) BatchAddRoutes(routes []*entities.Route, log *logger.Logger) error {
	return batch.ProcessUsingAnts(routes, rm.AddRoute, rm.concurrencyLimit, log)
}

func (rm *WindowsRouteManager) BatchDeleteRoutes(routes []*entities.Route, log *logger.Logger) error {
	return batch.ProcessUsingAnts(routes, rm.DeleteRoute, rm.concurrencyLimit, log)
}

// GetPhysicalGateway gets the underlying physical network gateway (for route management)
func (rm *WindowsRouteManager) GetPhysicalGateway() (net.IP, string, error) {
	// For Windows, we use the same implementation as system default route
	// In a real scenario, this would need logic to detect VPN interfaces
	return rm.GetSystemDefaultRoute()
}

// GetSystemDefaultRoute gets the current default route (including VPN) from the system
func (rm *WindowsRouteManager) GetSystemDefaultRoute() (net.IP, string, error) {
	cmd := exec.Command("route", "print", "0.0.0.0")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get default route: %w", err)
	}

	return rm.parseDefaultRouteWindows(string(output))
}

// ListSystemRoutes gets all routes from the system routing table by `netstat -rn`
// 
// IPv4 Route Table
// ===========================================================================
// Active Routes:
// Network Destination        Netmask          Gateway       Interface  Metric
//           0.0.0.0          0.0.0.0      10.211.55.1      10.211.55.9     15
//       10.211.55.0    255.255.255.0         On-link       10.211.55.9    271
//       10.211.55.9  255.255.255.255         On-link       10.211.55.9    271
//     10.211.55.255  255.255.255.255         On-link       10.211.55.9    271
//         127.0.0.0        255.0.0.0         On-link         127.0.0.1    331
//         127.0.0.1  255.255.255.255         On-link         127.0.0.1    331
//   127.255.255.255  255.255.255.255         On-link         127.0.0.1    331
//         224.0.0.0        240.0.0.0         On-link         127.0.0.1    331
//         224.0.0.0        240.0.0.0         On-link       10.211.55.9    271
//   255.255.255.255  255.255.255.255         On-link         127.0.0.1    331
//   255.255.255.255  255.255.255.255         On-link       10.211.55.9    271
// ===========================================================================
func (rm *WindowsRouteManager) ListSystemRoutes() ([]*entities.Route, error) {
	cmd := exec.Command("netstat", "-rn")
	_, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}

	// TODO: Implement parsing of netstat output

	return nil, nil
}

func (rm *WindowsRouteManager) FlushRoutes(gateway net.IP) error {
	routes, err := rm.ListSystemRoutes()
	if err != nil {
		return fmt.Errorf("failed to list routes: %w", err)
	}

	var routesToDelete []*entities.Route
	for _, route := range routes {
		if route.Gateway.Equal(gateway) {
			routesToDelete = append(routesToDelete, route)
		}
	}

	return rm.BatchDeleteRoutes(routesToDelete, nil)
}

func (rm *WindowsRouteManager) Close() error {
	return nil
}

func (rm *WindowsRouteManager) addRouteWithRetry(network *net.IPNet, gateway net.IP) error {
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

func (rm *WindowsRouteManager) deleteRouteWithRetry(network *net.IPNet, gateway net.IP) error {
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

func (rm *WindowsRouteManager) addRouteDirect(network *net.IPNet, gateway net.IP) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	ones, _ := network.Mask.Size()
	cmd := exec.Command("route", "add", network.IP.String(), "mask", net.IP(network.Mask).String(), gateway.String(), "metric", "1")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			switch exitErr.ExitCode() {
			case 1:
				return &entities.RouteOperationError{ErrorType: entities.RouteErrPermission, Destination: *network, Gateway: gateway, Cause: err}
			case 87:
				return &entities.RouteOperationError{ErrorType: entities.RouteErrInvalidRoute, Destination: *network, Gateway: gateway, Cause: err}
			default:
				return &entities.RouteOperationError{ErrorType: entities.RouteErrSystemCall, Destination: *network, Gateway: gateway, Cause: err}
			}
		}
		return &entities.RouteOperationError{ErrorType: entities.RouteErrSystemCall, Destination: *network, Gateway: gateway, Cause: err}
	}

	_ = ones
	return nil
}

func (rm *WindowsRouteManager) deleteRouteDirect(network *net.IPNet, gateway net.IP) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	cmd := exec.Command("route", "delete", network.IP.String(), "mask", net.IP(network.Mask).String())
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return nil
			}
		}
		return &entities.RouteOperationError{ErrorType: entities.RouteErrSystemCall, Destination: *network, Gateway: gateway, Cause: err}
	}

	return nil
}

func (rm *WindowsRouteManager) parseDefaultRouteWindows(output string) (net.IP, string, error) {
	// Parse Windows route output format
	// Looking for lines like: "0.0.0.0 0.0.0.0 192.168.1.1 interface_index metric"
	lines := strings.Split(output, "\n")
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

