package routing

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/wesleywu/update-routes-native/internal/routing/entities"
)

// NetworkMonitor is a monitor for the network
type NetworkMonitor struct {
	gateway        net.IP
	defaultIface   string
	routeSocket    int
	eventChan      chan NetworkEvent
	stopChan       chan struct{}
	mutex          sync.RWMutex
	isRunning      bool
	pollInterval   time.Duration
	lastRouteCheck time.Time

	// Smart polling control
	pollEnabled         bool
	pollTicker          *time.Ticker
	pollStopChan        chan struct{}
	lastEventTime       time.Time
	routeSocketErrors   int
	maxSocketErrors     int
	healthCheckInterval time.Duration

	// RouteManager for getting gateway information
	routeManager entities.RouteManager

	// VPN state tracking
	lastVPNInterface string
	lastVPNState     bool // true if VPN was connected in last check
}

// NetworkEvent is an event from the network monitor
type NetworkEvent struct {
	Type            EventType
	Interface       string
	PhysicalGateway net.IP // physical gateway
	DefaultGateway  net.IP // default gateway
	Timestamp       time.Time
	// VPN related information
	IsVPNConnected bool
	VPNState       string // "Connected", "Disconnected", or ""
}

// EventType is the type of event
type EventType int

const (
	// GatewayChanged is a gateway change event
	GatewayChanged EventType = iota
	// InterfaceUp is an interface up event
	InterfaceUp
	// InterfaceDown is an interface down event
	InterfaceDown
	// AddressChanged is an address change event
	AddressChanged
	// VPNConnected is a VPN connection event
	VPNConnected
	// VPNDisconnected is a VPN disconnection event
	VPNDisconnected
)

// String returns the string representation of the event type
func (e EventType) String() string {
	switch e {
	case GatewayChanged:
		return "GatewayChanged"
	case InterfaceUp:
		return "InterfaceUp"
	case InterfaceDown:
		return "InterfaceDown"
	case AddressChanged:
		return "AddressChanged"
	case VPNConnected:
		return "VPNConnected"
	case VPNDisconnected:
		return "VPNDisconnected"
	default:
		return "Unknown"
	}
}

// NewNetworkMonitor creates a new NetworkMonitor
func NewNetworkMonitor(pollInterval time.Duration, routeManager entities.RouteManager) (*NetworkMonitor, error) {
	if routeManager == nil {
		return nil, fmt.Errorf("routeManager cannot be nil")
	}

	// For initialization, use physical gateway for route management
	gateway, iface, err := routeManager.GetDefaultGateway()
	if err != nil {
		return nil, fmt.Errorf("failed to get initial gateway: %w", err)
	}

	// Also check current default route to initialize VPN state
	_, currentIface, _ := routeManager.GetCurrentDefaultRoute()
	isVPNInitial := false
	vpnIface := ""
	if currentIface != "" {
		// Temporary instance to use isVPNInterface method
		temp := &NetworkMonitor{}
		isVPNInitial = temp.isVPNInterface(currentIface)
		if isVPNInitial {
			vpnIface = currentIface
		}
	}

	return &NetworkMonitor{
		gateway:             gateway,
		defaultIface:        iface,
		eventChan:           make(chan NetworkEvent, 100),
		stopChan:            make(chan struct{}),
		pollInterval:        pollInterval,
		pollStopChan:        make(chan struct{}),
		pollEnabled:         false,            // Default to disable polling
		maxSocketErrors:     3,                // Enable polling after 3 consecutive socket errors
		healthCheckInterval: 30 * time.Second, // 30 second health check interval
		lastEventTime:       time.Now(),
		routeManager:        routeManager,
		// Initialize VPN state
		lastVPNState:     isVPNInitial,
		lastVPNInterface: vpnIface,
	}, nil
}

// Start starts the network monitor
func (nm *NetworkMonitor) Start() error {
	nm.mutex.Lock()
	defer nm.mutex.Unlock()

	if nm.isRunning {
		return fmt.Errorf("network monitor is already running")
	}

	// Try to create route socket for real-time monitoring
	nm.startPlatformMonitoring()

	// Start health check goroutine
	go nm.healthCheck()

	// If polling is enabled, start polling
	if nm.pollEnabled {
		nm.startPolling()
	}

	nm.isRunning = true
	return nil
}

// Stop stops the network monitor
func (nm *NetworkMonitor) Stop() error {
	nm.mutex.Lock()
	defer nm.mutex.Unlock()

	if !nm.isRunning {
		return nil
	}

	close(nm.stopChan)

	// 停止轮询
	nm.stopPolling()

	nm.closeRouteSocket()

	nm.isRunning = false
	return nil
}

// Events returns the channel for network events
func (nm *NetworkMonitor) Events() <-chan NetworkEvent {
	return nm.eventChan
}

// GetCurrentGateway returns the current gateway and interface
func (nm *NetworkMonitor) GetCurrentGateway() (net.IP, string) {
	nm.mutex.RLock()
	defer nm.mutex.RUnlock()

	gateway := make(net.IP, len(nm.gateway))
	copy(gateway, nm.gateway)

	return gateway, nm.defaultIface
}

// monitorRouteSocket monitors the route socket
func (nm *NetworkMonitor) monitorRouteSocket() {

	buffer := make([]byte, 4096)
	for {
		select {
		case <-nm.stopChan:
			return
		default:
			// Use blocking read to avoid busy waiting
			n, err := nm.readRouteSocket(buffer)
			if err != nil {
				// Check if the error is due to monitor stop
				select {
				case <-nm.stopChan:
					return
				default:
				}

				// Only count actual read errors
				if nm.isSocketError(err) {
					nm.mutex.Lock()
					nm.routeSocketErrors++
					fmt.Printf("Route socket read error (%d/%d): %v\n", nm.routeSocketErrors, nm.maxSocketErrors, err)
					nm.mutex.Unlock()
				}

				// Short delay before retrying to avoid busy waiting
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// 成功读取数据，重置错误计数
			nm.mutex.Lock()
			nm.routeSocketErrors = 0
			nm.lastEventTime = time.Now()
			nm.mutex.Unlock()

			if event := nm.parseRouteMessage(buffer[:n]); event != nil {
				select {
				case nm.eventChan <- *event:
				case <-nm.stopChan:
					return
				}
			}
		}
	}
}

// healthCheck is a health check for the network monitor
// It only enables polling when the route socket has consecutive errors
// Note: No network events are normal and should not trigger polling
func (nm *NetworkMonitor) healthCheck() {
	ticker := time.NewTicker(nm.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-nm.stopChan:
			return
		case <-ticker.C:
			nm.mutex.RLock()
			routeSocketErrors := nm.routeSocketErrors
			pollEnabled := nm.pollEnabled
			nm.mutex.RUnlock()

			// 只有在route socket连续出错时才启用轮询
			// 没有网络变化时不收到事件是正常的，不应该视为错误
			if !pollEnabled && routeSocketErrors >= nm.maxSocketErrors {
				fmt.Printf("Route socket has %d errors, enabling polling as backup\n", routeSocketErrors)
				nm.mutex.Lock()
				nm.pollEnabled = true
				nm.mutex.Unlock()
				nm.startPolling()
			}

			// 如果route socket恢复正常，可以考虑禁用轮询
			if pollEnabled && routeSocketErrors == 0 {
				// 等待一段时间确保socket稳定后再禁用轮询
				time.Sleep(5 * time.Second)
				nm.mutex.RLock()
				stillNoErrors := nm.routeSocketErrors == 0
				nm.mutex.RUnlock()

				if stillNoErrors {
					fmt.Printf("Route socket appears stable, disabling polling\n")
					nm.mutex.Lock()
					nm.pollEnabled = false
					nm.mutex.Unlock()
					nm.stopPolling()
				}
			}
		}
	}
}

// startPolling starts polling
func (nm *NetworkMonitor) startPolling() {
	nm.mutex.Lock()
	defer nm.mutex.Unlock()

	if nm.pollTicker != nil {
		return // 已经在运行
	}

	nm.pollTicker = time.NewTicker(nm.pollInterval)
	nm.pollStopChan = make(chan struct{})

	go func() {
		defer nm.pollTicker.Stop()
		for {
			select {
			case <-nm.stopChan:
				return
			case <-nm.pollStopChan:
				return
			case <-nm.pollTicker.C:
				nm.checkGatewayChange()
			}
		}
	}()

	fmt.Printf("Polling started with interval %v\n", nm.pollInterval)
}

// stopPolling stops polling
func (nm *NetworkMonitor) stopPolling() {
	if nm.pollTicker != nil {
		close(nm.pollStopChan)
		nm.pollTicker = nil
		fmt.Printf("Polling stopped\n")
	}
}

// checkGatewayChange checks for both physical gateway changes and VPN state changes
func (nm *NetworkMonitor) checkGatewayChange() {
	// Check both physical gateway and current default route
	physicalGW, physicalIface, err1 := nm.routeManager.GetDefaultGateway()
	currentGW, currentIface, err2 := nm.routeManager.GetCurrentDefaultRoute()

	if err1 != nil {
		fmt.Printf("DEBUG: Failed to get physical gateway: %v\n", err1)
	}
	if err2 != nil {
		fmt.Printf("DEBUG: Failed to get current default route: %v\n", err2)
	}
	if err1 != nil && err2 != nil {
		return
	}

	nm.mutex.Lock()
	oldPhysicalGW := nm.gateway
	oldPhysicalIface := nm.defaultIface

	// Physical gateway change detection (for WiFi switching)
	physicalGWChanged := false
	physicalIfaceChanged := false
	if err1 == nil {
		physicalGWChanged = !nm.gateway.Equal(physicalGW)
		physicalIfaceChanged = nm.defaultIface != physicalIface
	}

	// VPN state detection (from current default route)
	currentIsVPN := false
	vpnStateChanged := false
	lastIsVPN := nm.lastVPNState
	lastVPNIface := nm.lastVPNInterface

	if err2 == nil {
		currentIsVPN = nm.isVPNInterface(currentIface)
		vpnStateChanged = currentIsVPN != lastIsVPN ||
			(currentIsVPN && currentIface != lastVPNIface)
	}

	// Debug logging
	if err1 == nil && err2 == nil {
		fmt.Printf("DEBUG: Physical: %s (%s), Current: %s (%s), PhysicalChanged: %t, VPN: %t->%t\n",
			physicalGW.String(), physicalIface, currentGW.String(), currentIface,
			physicalGWChanged || physicalIfaceChanged, lastIsVPN, currentIsVPN)
	}

	// Update internal state
	hasChanges := false
	var event NetworkEvent

	if physicalGWChanged || physicalIfaceChanged {
		// Physical gateway change (WiFi switching)
		nm.gateway = physicalGW
		nm.defaultIface = physicalIface
		hasChanges = true

		event = NetworkEvent{
			Type:            GatewayChanged,
			Interface:       physicalIface,
			PhysicalGateway: physicalGW,
			DefaultGateway:  currentGW,
			Timestamp:       time.Now(),
			IsVPNConnected:  currentIsVPN,
		}

		fmt.Printf("Physical Gateway Changed: %s (%s) -> %s (%s)\n",
			oldPhysicalGW.String(), oldPhysicalIface, physicalGW.String(), physicalIface)
	}

	if vpnStateChanged {
		// VPN state change
		nm.lastVPNState = currentIsVPN
		nm.lastVPNInterface = currentIface
		hasChanges = true

		var eventType EventType
		var vpnState string

		if currentIsVPN {
			eventType = VPNConnected
			vpnState = "Connected"
			fmt.Printf("VPN Connected: %s via %s\n", currentGW.String(), currentIface)
		} else {
			eventType = VPNDisconnected
			vpnState = "Disconnected"
			fmt.Printf("VPN Disconnected: %s via %s\n", currentGW.String(), currentIface)
		}

		// VPN events override physical gateway events
		event = NetworkEvent{
			Type:            eventType,
			Interface:       currentIface,
			PhysicalGateway: physicalGW,
			DefaultGateway:  currentGW,
			Timestamp:       time.Now(),
			IsVPNConnected:  currentIsVPN,
			VPNState:        vpnState,
		}
	}

	nm.mutex.Unlock()

	if hasChanges {
		select {
		case nm.eventChan <- event:
		case <-nm.stopChan:
			return
		}
	}
}

// parseRouteMessage parses the route message
func (nm *NetworkMonitor) parseRouteMessage(data []byte) *NetworkEvent {
	if len(data) < 4 {
		return nil
	}

	// Rate limit: only trigger checks every 200ms to avoid spam
	nm.mutex.Lock()
	now := time.Now()
	if nm.lastRouteCheck.IsZero() || now.Sub(nm.lastRouteCheck) > 200*time.Millisecond {
		nm.lastRouteCheck = now
		nm.mutex.Unlock()

		// Trigger immediate gateway check when route messages are received
		go func() {
			// Small delay to allow network stack to settle
			time.Sleep(100 * time.Millisecond)
			nm.checkGatewayChange()
		}()
	} else {
		nm.mutex.Unlock()
	}

	return nil // Don't return the unparsed event to avoid spam
}

// isVPNInterface checks if the given interface name is a VPN interface
func (nm *NetworkMonitor) isVPNInterface(interfaceName string) bool {
	// Common VPN interface patterns
	if len(interfaceName) >= 4 {
		prefix := interfaceName[:4]
		switch prefix {
		case "utun", "tun0", "tap0":
			return true
		}
	}

	// Check for other common VPN interface patterns
	if len(interfaceName) >= 3 {
		prefix := interfaceName[:3]
		switch prefix {
		case "tun", "tap", "ppp":
			return true
		}
	}

	return false
}

// GetMonitorStatus gets the monitor status
func (nm *NetworkMonitor) GetMonitorStatus() map[string]interface{} {
	nm.mutex.RLock()
	defer nm.mutex.RUnlock()

	return map[string]interface{}{
		"is_running":            nm.isRunning,
		"poll_enabled":          nm.pollEnabled,
		"route_socket":          nm.routeSocket > 0,
		"route_socket_errors":   nm.routeSocketErrors,
		"last_event_time":       nm.lastEventTime,
		"time_since_last_event": time.Since(nm.lastEventTime),
		"health_check_interval": nm.healthCheckInterval,
		"poll_interval":         nm.pollInterval,
	}
}
