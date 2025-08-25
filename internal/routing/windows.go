//go:build windows

package routing

import (
	"fmt"
	"net"
	"os/exec"
	"sync"
	"time"
)

type WindowsRouteManager struct {
	mutex            sync.Mutex
	concurrencyLimit int
	maxRetries       int
	metrics          *Metrics
}

func newPlatformRouteManager(concurrencyLimit, maxRetries int) (RouteManager, error) {
	return NewWindowsRouteManager(concurrencyLimit, maxRetries)
}

func NewWindowsRouteManager(concurrencyLimit, maxRetries int) (RouteManager, error) {
	return &WindowsRouteManager{
		concurrencyLimit: concurrencyLimit,
		maxRetries:       maxRetries,
		metrics:          NewMetrics(),
	}, nil
}

func (rm *WindowsRouteManager) AddRoute(network *net.IPNet, gateway net.IP) error {
	return rm.addRouteWithRetry(network, gateway)
}

func (rm *WindowsRouteManager) DeleteRoute(network *net.IPNet, gateway net.IP) error {
	return rm.deleteRouteWithRetry(network, gateway)
}

func (rm *WindowsRouteManager) BatchAddRoutes(routes []Route) error {
	return rm.batchOperation(routes, ActionAdd)
}

func (rm *WindowsRouteManager) BatchDeleteRoutes(routes []Route) error {
	return rm.batchOperation(routes, ActionDelete)
}

func (rm *WindowsRouteManager) GetDefaultGateway() (net.IP, string, error) {
	cmd := exec.Command("route", "print", "0.0.0.0")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get default route: %w", err)
	}

	return parseDefaultRouteWindows(string(output))
}

func (rm *WindowsRouteManager) ListRoutes() ([]Route, error) {
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

	var routesToDelete []Route
	for _, route := range routes {
		if route.Gateway.Equal(gateway) {
			routesToDelete = append(routesToDelete, route)
		}
	}

	return rm.BatchDeleteRoutes(routesToDelete)
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
		
		if routeErr, ok := err.(*RouteError); ok && !routeErr.IsRetryable() {
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
		
		if routeErr, ok := err.(*RouteError); ok && !routeErr.IsRetryable() {
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
				return &RouteError{Type: ErrPermission, Network: *network, Gateway: gateway, Cause: err}
			case 87:
				return &RouteError{Type: ErrInvalidRoute, Network: *network, Gateway: gateway, Cause: err}
			default:
				return &RouteError{Type: ErrSystemCall, Network: *network, Gateway: gateway, Cause: err}
			}
		}
		return &RouteError{Type: ErrSystemCall, Network: *network, Gateway: gateway, Cause: err}
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
		return &RouteError{Type: ErrSystemCall, Network: *network, Gateway: gateway, Cause: err}
	}
	
	return nil
}

func (rm *WindowsRouteManager) batchOperation(routes []Route, action ActionType) error {
	semaphore := make(chan struct{}, rm.concurrencyLimit)
	var wg sync.WaitGroup
	errChan := make(chan error, len(routes))

	for _, route := range routes {
		wg.Add(1)
		go func(r Route) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			var err error
			switch action {
			case ActionAdd:
				err = rm.AddRoute(&r.Network, r.Gateway)
			case ActionDelete:
				err = rm.DeleteRoute(&r.Network, r.Gateway)
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

func parseDefaultRouteWindows(output string) (net.IP, string, error) {
	return nil, "", fmt.Errorf("not implemented")
}

func parseRouteOutput(output string) ([]Route, error) {
	return nil, fmt.Errorf("not implemented")
}