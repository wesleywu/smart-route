//go:build darwin || freebsd
// Package routing provides a route manager for BSD-based systems
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

// BSDRouteManager is a route manager for BSD-based systems
type BSDRouteManager struct {
	socket           int
	mutex            sync.Mutex
	concurrencyLimit int
	maxRetries       int
	metrics          *Metrics
	seqNum           int32  // Add sequence number counter
}

func newPlatformRouteManager(concurrencyLimit, maxRetries int) (RouteManager, error) {
	return NewBSDRouteManager(concurrencyLimit, maxRetries)
}

// NewBSDRouteManager creates a new BSD route manager
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
		seqNum:           1,  // Initialize sequence number
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
func (rm *BSDRouteManager) BatchAddRoutes(routes []Route, log *logger.Logger) error {
	return rm.batchOperation(routes, ActionAdd, log)
}

// BatchDeleteRoutes deletes multiple routes from the system
func (rm *BSDRouteManager) BatchDeleteRoutes(routes []Route, log *logger.Logger) error {
	return rm.batchOperation(routes, ActionDelete, log)
}

// GetDefaultGateway gets the default gateway from the system
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

// ListRoutes lists all routes from the system
func (rm *BSDRouteManager) ListRoutes() ([]Route, error) {
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

// deleteRouteWithRetry deletes a route from the system with retry logic
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
	// Try native system call first
	err := rm.deleteRouteNative(network, gateway, log)
	
	// If native method fails with "no such process", try command line as fallback
	if err != nil && strings.Contains(err.Error(), "no such process") {
		log.Debug("Native delete failed with 'no such process', trying command line fallback", 
			"network", network.String(), "gateway", gateway.String())
		return rm.deleteRouteCommand(network, gateway, log)
	}
	
	return err
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
	var routes []Route
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
		iface := ""
		
		// Parse fields based on standard netstat format:
		// Destination Gateway Flags Netif [Expire]
		if len(fields) >= 4 {
			iface = fields[3] // Netif is always the 4th field (index 3)
		}
		
		// Skip if no interface or gateway
		if gateway == "" || gateway == "-" || iface == "" {
			continue
		}
		
		// Parse destination network
		network, err := parseDestination(destination)
		if err != nil {
			continue // Skip unparseable destinations
		}
		
		// Parse gateway IP
		gwIP := net.ParseIP(gateway)
		if gwIP == nil {
			continue // Skip unparseable gateways (like link# formats)
		}
		
		route := Route{
			Network:   *network,
			Gateway:   gwIP,
			Interface: iface,
		}
		
		routes = append(routes, route)
	}
	
	return routes, nil
}

// parseDestination parses various destination formats from netstat
func parseDestination(dest string) (*net.IPNet, error) {
	// Handle special destinations
	if dest == "default" {
		_, network, _ := net.ParseCIDR("0.0.0.0/0")
		return network, nil
	}
	
	// Handle CIDR notation (e.g., "192.168.1.0/24" or "114.114.114.114/32")
	if strings.Contains(dest, "/") {
		// Handle netstat's simplified format like "1.0.1/24" -> "1.0.1.0/24"
		parts := strings.Split(dest, "/")
		if len(parts) == 2 {
			ip := parts[0]
			mask := parts[1]
			
			// Count dots in IP part
			dotCount := strings.Count(ip, ".")
			
			// Add missing octets to make it a valid IP
			switch dotCount {
			case 0: // "1/24" -> "1.0.0.0/24"
				ip = ip + ".0.0.0"
			case 1: // "1.0/24" -> "1.0.0.0/24" 
				ip = ip + ".0.0"
			case 2: // "1.0.1/24" -> "1.0.1.0/24"
				ip = ip + ".0"
			case 3: // "1.0.1.0/24" - already complete
				// no change needed
			}
			
			dest = ip + "/" + mask
		}
		
		_, network, err := net.ParseCIDR(dest)
		return network, err
	}
	
	// Handle single IP addresses (assume /32 for IPv4, /128 for IPv6)
	ip := net.ParseIP(dest)
	if ip != nil {
		if ip.To4() != nil {
			// IPv4
			return &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(32, 32),
			}, nil
		}
		if ip.To16() != nil {
			// IPv6
			return &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(128, 128),
			}, nil
		}
	}
	
	// Handle incomplete network addresses without explicit mask (e.g., "203.26.55" -> "203.26.55.0/24")
	if strings.Contains(dest, ".") {
		dotCount := strings.Count(dest, ".")
		if dotCount < 3 {
			// Add missing octets and assume appropriate network mask
			switch dotCount {
			case 1: // "203.26" -> "203.26.0.0/16"
				dest = dest + ".0.0"
				return &net.IPNet{
					IP:   net.ParseIP(dest),
					Mask: net.CIDRMask(16, 32),
				}, nil
			case 2: // "203.26.55" -> "203.26.55.0/24"
				dest = dest + ".0"
				return &net.IPNet{
					IP:   net.ParseIP(dest),
					Mask: net.CIDRMask(24, 32),
				}, nil
			}
		}
	}
	
	// Assume /32 for what looks like a complete IP
	testIP := net.ParseIP(dest)
	if testIP != nil {
		if testIP.To4() != nil {
			return &net.IPNet{
				IP:   testIP,
				Mask: net.CIDRMask(32, 32),
			}, nil
		}
	}
	
	return nil, fmt.Errorf("unsupported destination format: %s", dest)
}

// deleteRouteCommand deletes a route using the command line route tool as fallback
func (rm *BSDRouteManager) deleteRouteCommand(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	// For single IP addresses (/32), use just the IP without /32
	var target string
	if network.Mask != nil {
		ones, bits := network.Mask.Size()
		if bits == 32 && ones == 32 {
			// This is a /32 route, use just the IP
			target = network.IP.String()
		} else {
			// This is a network route, use CIDR notation
			target = network.String()
		}
	} else {
		target = network.IP.String()
	}
	
	log.Debug("Using command line route delete", "target", target, "gateway", gateway.String())
	
	cmd := exec.Command("route", "-n", "delete", target, gateway.String())
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		// Check if this is an acceptable "not found" error
		outputStr := string(output)
		if strings.Contains(outputStr, "not in table") || strings.Contains(outputStr, "No such process") {
			log.Debug("Route not found in table (acceptable)", "target", target, "output", outputStr)
			return nil // Route already doesn't exist
		}
		
		log.Error("Command line route delete failed", "target", target, "gateway", gateway.String(), 
			"error", err, "output", outputStr)
		return &RouteError{
			Type:    ErrSystemCall,
			Network: *network,
			Gateway: gateway,
			Cause:   fmt.Errorf("command line delete failed: %w, output: %s", err, outputStr),
		}
	}
	
	log.Debug("Command line route delete succeeded", "target", target, "gateway", gateway.String())
	return nil
}