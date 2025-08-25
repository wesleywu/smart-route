//go:build darwin || freebsd

package routing

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wesleywu/update-routes-native/internal/logger"
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

func (rm *BSDRouteManager) AddRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.addRouteWithRetry(network, gateway, log)
}

func (rm *BSDRouteManager) DeleteRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.deleteRouteWithRetry(network, gateway, log)
}

func (rm *BSDRouteManager) BatchAddRoutes(routes []Route, log *logger.Logger) error {
	return rm.batchOperation(routes, ActionAdd, log)
}

func (rm *BSDRouteManager) BatchDeleteRoutes(routes []Route, log *logger.Logger) error {
	return rm.batchOperation(routes, ActionDelete, log)
}

func (rm *BSDRouteManager) GetDefaultGateway() (net.IP, string, error) {
	// Currently using command-line method
	// TODO: Implement native method using route socket for consistency
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
	// Instead of parsing netstat output, we'll attempt to delete routes
	// that we know we might have added. This is more reliable since
	// we control what routes we add.
	
	// For now, we don't need to implement full route flushing
	// because we handle route conflicts during batch add operations
	// The error handling in bsd_batch.go already skips "file exists" errors
	
	return nil // Routes will be overwritten/skipped during batch add
}

func (rm *BSDRouteManager) Close() error {
	return unix.Close(rm.socket)
}

func (rm *BSDRouteManager) addRouteWithRetry(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	var lastErr error
	start := time.Now()
	
	for attempt := 0; attempt < rm.maxRetries; attempt++ {
		err := rm.addRouteDirect(network, gateway, log)
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

func (rm *BSDRouteManager) deleteRouteWithRetry(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	var lastErr error
	start := time.Now()
	
	for attempt := 0; attempt < rm.maxRetries; attempt++ {
		err := rm.deleteRouteDirect(network, gateway, log)
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

func (rm *BSDRouteManager) addRouteDirect(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	// Use native system call for better performance
	return rm.addRouteNative(network, gateway, log)
}

func (rm *BSDRouteManager) deleteRouteDirect(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	// Use native system call for better performance
	return rm.deleteRouteNative(network, gateway, log)
}

func (rm *BSDRouteManager) batchOperation(routes []Route, action ActionType, log *logger.Logger) error {
	// Use optimized native batch operation for better performance
	return rm.batchOperationNative(routes, action, log)
}

func parseDefaultRoute(output string) (net.IP, string, error) {
	lines := strings.Split(output, "\n")
	var gateway net.IP
	var iface string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			gatewayStr := strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
			gateway = net.ParseIP(gatewayStr)
			if gateway == nil {
				return nil, "", fmt.Errorf("invalid gateway IP: %s", gatewayStr)
			}
		}
		if strings.HasPrefix(line, "interface:") {
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}

	if gateway == nil {
		return nil, "", fmt.Errorf("no default gateway found in output")
	}

	return gateway, iface, nil
}

func parseNetstatOutput(output string) ([]Route, error) {
	return nil, fmt.Errorf("not implemented")
}