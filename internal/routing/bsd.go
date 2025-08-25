//go:build darwin || freebsd

package routing

import (
	"fmt"
	"net"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type BSDRouteManager struct {
	socket           int
	mutex            sync.Mutex
	concurrencyLimit int
	maxRetries       int
	metrics          *Metrics
}

func newPlatformRouteManager(concurrencyLimit, maxRetries int) (RouteManager, error) {
	return NewBSDRouteManager(concurrencyLimit, maxRetries)
}

func NewBSDRouteManager(concurrencyLimit, maxRetries int) (RouteManager, error) {
	sock, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return nil, fmt.Errorf("failed to create route socket: %w", err)
	}

	return &BSDRouteManager{
		socket:           sock,
		concurrencyLimit: concurrencyLimit,
		maxRetries:       maxRetries,
		metrics:          NewMetrics(),
	}, nil
}

func (rm *BSDRouteManager) AddRoute(network *net.IPNet, gateway net.IP) error {
	return rm.addRouteWithRetry(network, gateway)
}

func (rm *BSDRouteManager) DeleteRoute(network *net.IPNet, gateway net.IP) error {
	return rm.deleteRouteWithRetry(network, gateway)
}

func (rm *BSDRouteManager) BatchAddRoutes(routes []Route) error {
	return rm.batchOperation(routes, ActionAdd)
}

func (rm *BSDRouteManager) BatchDeleteRoutes(routes []Route) error {
	return rm.batchOperation(routes, ActionDelete)
}

func (rm *BSDRouteManager) GetDefaultGateway() (net.IP, string, error) {
	cmd := exec.Command("route", "-n", "get", "default")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get default route: %w", err)
	}

	return parseDefaultRoute(string(output))
}

func (rm *BSDRouteManager) ListRoutes() ([]Route, error) {
	cmd := exec.Command("netstat", "-rn")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}

	return parseNetstatOutput(string(output))
}

func (rm *BSDRouteManager) FlushRoutes(gateway net.IP) error {
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

func (rm *BSDRouteManager) Close() error {
	return unix.Close(rm.socket)
}

func (rm *BSDRouteManager) addRouteWithRetry(network *net.IPNet, gateway net.IP) error {
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

func (rm *BSDRouteManager) deleteRouteWithRetry(network *net.IPNet, gateway net.IP) error {
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

func (rm *BSDRouteManager) addRouteDirect(network *net.IPNet, gateway net.IP) error {
	// Use native system call for better performance
	return rm.addRouteNative(network, gateway)
}

func (rm *BSDRouteManager) deleteRouteDirect(network *net.IPNet, gateway net.IP) error {
	// Use native system call for better performance
	return rm.deleteRouteNative(network, gateway)
}

func (rm *BSDRouteManager) batchOperation(routes []Route, action ActionType) error {
	// Use optimized native batch operation for better performance
	return rm.batchOperationNative(routes, action)
}

func parseDefaultRoute(output string) (net.IP, string, error) {
	return nil, "", fmt.Errorf("not implemented")
}

func parseNetstatOutput(output string) ([]Route, error) {
	return nil, fmt.Errorf("not implemented")
}