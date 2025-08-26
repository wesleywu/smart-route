//go:build linux

package routing

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wesleywu/update-routes-native/internal/logger"
)

type LinuxRouteManager struct {
	mutex            sync.Mutex
	concurrencyLimit int
	maxRetries       int
	metrics          *Metrics
}

func newPlatformRouteManager(concurrencyLimit, maxRetries int) (RouteManager, error) {
	return NewLinuxRouteManager(concurrencyLimit, maxRetries)
}

func NewLinuxRouteManager(concurrencyLimit, maxRetries int) (RouteManager, error) {
	return &LinuxRouteManager{
		concurrencyLimit: concurrencyLimit,
		maxRetries:       maxRetries,
		metrics:          NewMetrics(),
	}, nil
}

func (rm *LinuxRouteManager) AddRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.addRouteWithRetry(network, gateway)
}

func (rm *LinuxRouteManager) DeleteRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.deleteRouteWithRetry(network, gateway)
}

func (rm *LinuxRouteManager) BatchAddRoutes(routes []Route, log *logger.Logger) error {
	return rm.batchOperation(routes, ActionAdd, log)
}

func (rm *LinuxRouteManager) BatchDeleteRoutes(routes []Route, log *logger.Logger) error {
	return rm.batchOperation(routes, ActionDelete, log)
}

func (rm *LinuxRouteManager) GetDefaultGateway() (net.IP, string, error) {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get default route: %w", err)
	}

	return rm.parseDefaultRouteLinux(string(output))
}

func (rm *LinuxRouteManager) ListRoutes() ([]Route, error) {
	cmd := exec.Command("ip", "route", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}

	return parseIPRouteOutput(string(output))
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

func (rm *LinuxRouteManager) deleteRouteWithRetry(network *net.IPNet, gateway net.IP) error {
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

func (rm *LinuxRouteManager) addRouteDirect(network *net.IPNet, gateway net.IP) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	cmd := exec.Command("ip", "route", "add", network.String(), "via", gateway.String())
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			switch exitErr.ExitCode() {
			case 1:
				return &RouteError{Type: ErrPermission, Network: *network, Gateway: gateway, Cause: err}
			case 2:
				return &RouteError{Type: ErrInvalidRoute, Network: *network, Gateway: gateway, Cause: err}
			default:
				return &RouteError{Type: ErrSystemCall, Network: *network, Gateway: gateway, Cause: err}
			}
		}
		return &RouteError{Type: ErrSystemCall, Network: *network, Gateway: gateway, Cause: err}
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
		return &RouteError{Type: ErrSystemCall, Network: *network, Gateway: gateway, Cause: err}
	}
	
	return nil
}

func (rm *LinuxRouteManager) batchOperation(routes []Route, action ActionType, log *logger.Logger) error {
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
				err = rm.AddRoute(&r.Network, r.Gateway, log)
			case ActionDelete:
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

func parseIPRouteOutput(output string) ([]Route, error) {
	return nil, fmt.Errorf("not implemented")
}