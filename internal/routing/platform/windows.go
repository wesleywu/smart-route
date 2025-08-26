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

	"github.com/wesleywu/update-routes-native/internal/logger"
	"github.com/wesleywu/update-routes-native/internal/routing/entities"
	"github.com/wesleywu/update-routes-native/internal/routing/metrics"
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

func (rm *WindowsRouteManager) BatchAddRoutes(routes []entities.Route, log *logger.Logger) error {
	return rm.batchOperation(routes, entities.ActionAdd, log)
}

func (rm *WindowsRouteManager) BatchDeleteRoutes(routes []entities.Route, log *logger.Logger) error {
	return rm.batchOperation(routes, entities.ActionDelete, log)
}

// GetDefaultGateway gets the physical gateway from the system (for route management)
func (rm *WindowsRouteManager) GetDefaultGateway() (net.IP, string, error) {
	return rm.GetCurrentDefaultRoute()
}

// GetCurrentDefaultRoute gets the current default route (including VPN) from the system
func (rm *WindowsRouteManager) GetCurrentDefaultRoute() (net.IP, string, error) {
	cmd := exec.Command("route", "print", "0.0.0.0")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get default route: %w", err)
	}

	return rm.parseDefaultRouteWindows(string(output))
}

func (rm *WindowsRouteManager) ListRoutes() ([]entities.Route, error) {
	cmd := exec.Command("route", "print")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}

	return parseRouteOutput(string(output))
}

func (rm *WindowsRouteManager) FlushRoutes(gateway net.IP) error {
	routes, err := rm.ListRoutes()
	if err != nil {
		return fmt.Errorf("failed to list routes: %w", err)
	}

	var routesToDelete []entities.Route
	for _, route := range routes {
		if route.Gateway.Equal(gateway) {
			routesToDelete = append(routesToDelete, route)
		}
	}

	return rm.BatchDeleteRoutes(routesToDelete, nil)
}

// CleanupRoutesForNetworks removes all existing routes for the specified networks/IPs
func (rm *WindowsRouteManager) CleanupRoutesForNetworks(networks []net.IPNet, log *logger.Logger) error {
	if len(networks) == 0 {
		return nil
	}

	// Get all current routes
	allRoutes, err := rm.ListRoutes()
	if err != nil {
		log.Debug("failed to list routes for cleanup", "error", err)
		// Don't fail - we'll try to delete anyway
	}

	var routesToDelete []entities.Route
	
	// Find existing routes that match our target networks
	for _, network := range networks {
		// Check all current routes to see if any match this network
		for _, route := range allRoutes {
			if routesMatch(network, route.Network) {
				routesToDelete = append(routesToDelete, route)
				log.Debug("found existing route to cleanup", 
					"network", route.Network.String(), 
					"gateway", route.Gateway.String())
			}
		}
	}

	// Delete found routes
	if len(routesToDelete) > 0 {
		log.Info("cleaning up existing routes", "count", len(routesToDelete))
		if err := rm.BatchDeleteRoutes(routesToDelete, log); err != nil {
			log.Warn("failed to cleanup some routes", "error", err)
			// Don't return error - some routes might not exist anymore
		}
	}

	return nil
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
		
		if routeErr, ok := err.(*entities.RouteError); ok && !routeErr.IsRetryable() {
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
		
		if routeErr, ok := err.(*entities.RouteError); ok && !routeErr.IsRetryable() {
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
				return &entities.RouteError{Type: entities.ErrPermission, Network: *network, Gateway: gateway, Cause: err}
			case 87:
				return &entities.RouteError{Type: entities.ErrInvalidRoute, Network: *network, Gateway: gateway, Cause: err}
			default:
				return &entities.RouteError{Type: entities.ErrSystemCall, Network: *network, Gateway: gateway, Cause: err}
			}
		}
		return &entities.RouteError{Type: entities.ErrSystemCall, Network: *network, Gateway: gateway, Cause: err}
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
		return &entities.RouteError{Type: entities.ErrSystemCall, Network: *network, Gateway: gateway, Cause: err}
	}
	
	return nil
}

func (rm *WindowsRouteManager) batchOperation(routes []entities.Route, action entities.ActionType, log *logger.Logger) error {
	semaphore := make(chan struct{}, rm.concurrencyLimit)
	var wg sync.WaitGroup
	errChan := make(chan error, len(routes))

	for _, route := range routes {
		wg.Add(1)
		go func(r entities.Route) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			var err error
			switch action {
			case entities.ActionAdd:
				err = rm.AddRoute(&r.Network, r.Gateway, log)
			case entities.ActionDelete:
				err = rm.DeleteRoute(&r.Network, r.Gateway, log)
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

func parseRouteOutput(output string) ([]entities.Route, error) {
	return nil, fmt.Errorf("not implemented")
}