package routing

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/wesleywu/smart-route/internal/routing/entities"
)

// NetworkMonitor monitors network changes and VPN state using event-driven architecture
type NetworkMonitor struct {
	// Physical network state (for route management)
	physicalGateway   net.IP
	physicalInterface string
	
	// Route socket for real-time monitoring
	routeSocket    int
	eventChannel   chan NetworkEvent
	stopChannel    chan struct{}
	mutex          sync.RWMutex
	isRunning      bool
	
	// Polling fallback mechanism
	pollInterval        time.Duration
	pollEnabled         bool
	pollTicker          *time.Ticker
	pollStopChannel     chan struct{}
	
	// Health monitoring
	lastRouteEventTime  time.Time
	routeSocketErrors   int
	maxSocketErrors     int
	healthCheckInterval time.Duration
	
	// Route manager for gateway queries
	routeManager entities.RouteManager

	// VPN state tracking
	lastVPNInterface string
	lastVPNConnected bool
	
	// Rate limiting
	lastGatewayCheck time.Time
}

// NetworkEvent represents a network state change event
type NetworkEvent struct {
	EventType         EventType
	PhysicalInterface string    // Physical network interface (e.g., en0, eth0)
	VPNInterface      string    // VPN interface if applicable (e.g., utun0)
	PhysicalGateway   net.IP    // Physical network gateway (for route management)
	CurrentGateway    net.IP    // Current system default gateway (includes VPN)
	Timestamp         time.Time
	
	// VPN state information
	VPNConnected bool // True if VPN is currently connected
}

// EventType is the type of event
type EventType int

const (
	// PhysicalGatewayChanged indicates WiFi network switching
	PhysicalGatewayChanged EventType = iota
	// VPNConnected indicates VPN connection established
	VPNConnected
	// VPNDisconnected indicates VPN connection terminated
	VPNDisconnected
	// NetworkInterfaceUp indicates network interface activated
	NetworkInterfaceUp
	// NetworkInterfaceDown indicates network interface deactivated
	NetworkInterfaceDown
	// NetworkAddressChanged indicates network address change
	NetworkAddressChanged
)

// String returns the string representation of the event type
func (e EventType) String() string {
	switch e {
	case PhysicalGatewayChanged:
		return "PhysicalGatewayChanged"
	case VPNConnected:
		return "VPNConnected"
	case VPNDisconnected:
		return "VPNDisconnected"
	case NetworkInterfaceUp:
		return "NetworkInterfaceUp"
	case NetworkInterfaceDown:
		return "NetworkInterfaceDown"
	case NetworkAddressChanged:
		return "NetworkAddressChanged"
	default:
		return "UnknownEvent"
	}
}

// NewNetworkMonitor creates a new NetworkMonitor with event-driven architecture
func NewNetworkMonitor(pollInterval time.Duration, routeManager entities.RouteManager) (*NetworkMonitor, error) {
	if routeManager == nil {
		return nil, fmt.Errorf("route manager cannot be nil")
	}

	// Get initial physical gateway for route management
	physicalGW, physicalIface, err := routeManager.GetPhysicalGateway()
	if err != nil {
		return nil, fmt.Errorf("failed to get initial physical gateway: %w", err)
	}

	// Check current VPN state for initialization
	initialVPNState := false
	initialVPNInterface := ""
	if _, currentIface, err := routeManager.GetSystemDefaultRoute(); err == nil {
		if isVPNInterface(currentIface) {
			initialVPNState = true
			initialVPNInterface = currentIface
		}
	}

	return &NetworkMonitor{
		// Physical network state
		physicalGateway:   physicalGW,
		physicalInterface: physicalIface,
		
		// Event handling
		eventChannel:        make(chan NetworkEvent, 100),
		stopChannel:         make(chan struct{}),
		
		// Polling fallback
		pollInterval:    pollInterval,
		pollEnabled:     false, // Start with event-driven mode
		pollStopChannel: make(chan struct{}),
		
		// Health monitoring
		maxSocketErrors:     5,              // 增加容错次数
		healthCheckInterval: 60 * time.Second, // 延长检查间隔
		lastRouteEventTime:  time.Now(),
		
		// Dependencies
		routeManager: routeManager,
		
		// VPN state tracking
		lastVPNConnected: initialVPNState,
		lastVPNInterface: initialVPNInterface,
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

	close(nm.stopChannel)

	// 停止轮询
	nm.stopPolling()

	nm.closeRouteSocket()

	nm.isRunning = false
	return nil
}

// Events returns the read-only channel for network events
func (nm *NetworkMonitor) Events() <-chan NetworkEvent {
	return nm.eventChannel
}

// GetPhysicalGateway returns the current physical gateway and interface
func (nm *NetworkMonitor) GetPhysicalGateway() (net.IP, string) {
	nm.mutex.RLock()
	defer nm.mutex.RUnlock()

	// Create a copy to avoid data races
	gateway := make(net.IP, len(nm.physicalGateway))
	copy(gateway, nm.physicalGateway)

	return gateway, nm.physicalInterface
}

// monitorRouteSocket monitors the route socket
func (nm *NetworkMonitor) monitorRouteSocket() {
	buffer := make([]byte, 4096)
	for {
		select {
		case <-nm.stopChannel:
			return
		default:
			// Use blocking read to avoid busy waiting
			n, err := nm.readRouteSocket(buffer)
			if err != nil {
				// Check if the error is due to monitor stop
				select {
				case <-nm.stopChannel:
					return
				default:
				}

				// Only count actual read errors
				if nm.isSocketError(err) {
					nm.mutex.Lock()
					nm.routeSocketErrors++
					nm.mutex.Unlock()
				}

				// Short delay before retrying to avoid busy waiting
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// 成功读取数据，重置错误计数
			nm.mutex.Lock()
			nm.routeSocketErrors = 0
			nm.lastRouteEventTime = time.Now()
			nm.mutex.Unlock()

			if event := nm.parseRouteMessage(buffer[:n]); event != nil {
				select {
				case nm.eventChannel <- *event:
				case <-nm.stopChannel:
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
		case <-nm.stopChannel:
			return
		case <-ticker.C:
			nm.mutex.RLock()
			routeSocketErrors := nm.routeSocketErrors
			pollEnabled := nm.pollEnabled
			nm.mutex.RUnlock()

			// 只有在route socket连续出错时才启用轮询
			// 没有网络变化时不收到事件是正常的，不应该视为错误
			if !pollEnabled && routeSocketErrors >= nm.maxSocketErrors {
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
	nm.pollStopChannel = make(chan struct{})

	go func() {
		defer nm.pollTicker.Stop()
		for {
			select {
			case <-nm.stopChannel:
				return
			case <-nm.pollStopChannel:
				return
			case <-nm.pollTicker.C:
				nm.checkNetworkChanges()
			}
		}
	}()

	fmt.Printf("Polling started with interval %v\n", nm.pollInterval)
}

// stopPolling stops polling
func (nm *NetworkMonitor) stopPolling() {
	if nm.pollTicker != nil {
		close(nm.pollStopChannel)
		nm.pollTicker = nil
		fmt.Printf("Polling stopped\n")
	}
}

// checkNetworkChanges checks for both physical gateway changes and VPN state changes
func (nm *NetworkMonitor) checkNetworkChanges() {
	// Check both physical gateway and current default route
	physicalGW, physicalIface, err1 := nm.routeManager.GetPhysicalGateway()
	currentGW, currentIface, err2 := nm.routeManager.GetSystemDefaultRoute()

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
	oldPhysicalGW := nm.physicalGateway
	oldPhysicalIface := nm.physicalInterface

	// Physical gateway change detection (for WiFi switching)
	physicalGWChanged := false
	physicalIfaceChanged := false
	if err1 == nil {
		physicalGWChanged = !nm.physicalGateway.Equal(physicalGW)
		physicalIfaceChanged = nm.physicalInterface != physicalIface
	}

	// VPN state detection (from current default route)
	currentIsVPN := false
	vpnStateChanged := false
	lastIsVPN := nm.lastVPNConnected
	lastVPNIface := nm.lastVPNInterface

	if err2 == nil {
		currentIsVPN = isVPNInterface(currentIface)
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
		nm.physicalGateway = physicalGW
		nm.physicalInterface = physicalIface
		hasChanges = true

		event = NetworkEvent{
			EventType:         PhysicalGatewayChanged,
			PhysicalInterface: physicalIface,
			VPNInterface:      getVPNInterface(currentIface, currentIsVPN),
			PhysicalGateway:   physicalGW,
			CurrentGateway:    currentGW,
			Timestamp:         time.Now(),
			VPNConnected:      currentIsVPN,
		}

		fmt.Printf("Physical Gateway Changed: %s (%s) -> %s (%s)\n",
			oldPhysicalGW.String(), oldPhysicalIface, physicalGW.String(), physicalIface)
	}

	if vpnStateChanged {
		// VPN state change
		nm.lastVPNConnected = currentIsVPN
		nm.lastVPNInterface = currentIface
		hasChanges = true

		var eventType EventType

		if currentIsVPN {
			eventType = VPNConnected
			fmt.Printf("VPN Connected: %s via %s\n", currentGW.String(), currentIface)
		} else {
			eventType = VPNDisconnected
			fmt.Printf("VPN Disconnected: %s via %s\n", currentGW.String(), currentIface)
		}

		// VPN events override physical gateway events
		event = NetworkEvent{
			EventType:         eventType,
			PhysicalInterface: physicalIface,
			VPNInterface:      getVPNInterface(currentIface, currentIsVPN),
			PhysicalGateway:   physicalGW,
			CurrentGateway:    currentGW,
			Timestamp:         time.Now(),
			VPNConnected:      currentIsVPN,
		}
	}

	nm.mutex.Unlock()

	if hasChanges {
		select {
		case nm.eventChannel <- event:
		case <-nm.stopChannel:
			return
		}
	}
}

// parseRouteMessage parses route socket messages and triggers network checks
func (nm *NetworkMonitor) parseRouteMessage(data []byte) *NetworkEvent {
	if len(data) < 4 {
		return nil
	}

	// Rate limit: only trigger checks every 200ms to avoid spam
	nm.mutex.Lock()
	now := time.Now()
	if nm.lastGatewayCheck.IsZero() || now.Sub(nm.lastGatewayCheck) > 200*time.Millisecond {
		nm.lastGatewayCheck = now
		nm.mutex.Unlock()

		// Trigger immediate network check when route messages are received
		go func() {
			// Small delay to allow network stack to settle
			time.Sleep(100 * time.Millisecond)
			nm.checkNetworkChanges()
		}()
	} else {
		nm.mutex.Unlock()
	}

	return nil // Don't return unparsed events to avoid spam
}

// getVPNInterface returns the VPN interface name if VPN is connected, otherwise empty string
func getVPNInterface(currentIface string, isVPNConnected bool) string {
	if isVPNConnected && isVPNInterface(currentIface) {
		return currentIface
	}
	return ""
}

// isVPNInterface checks if the given interface name is a VPN interface
func isVPNInterface(interfaceName string) bool {
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

// GetMonitorStatus returns the current status of the network monitor
func (nm *NetworkMonitor) GetMonitorStatus() map[string]interface{} {
	nm.mutex.RLock()
	defer nm.mutex.RUnlock()

	return map[string]interface{}{
		"is_running":            nm.isRunning,
		"poll_enabled":          nm.pollEnabled,
		"route_socket":          nm.routeSocket > 0,
		"route_socket_errors":   nm.routeSocketErrors,
		"last_event_time":       nm.lastRouteEventTime,
		"time_since_last_event": time.Since(nm.lastRouteEventTime),
		"health_check_interval": nm.healthCheckInterval,
		"poll_interval":         nm.pollInterval,
		"physical_gateway":      nm.physicalGateway.String(),
		"physical_interface":    nm.physicalInterface,
		"vpn_connected":         nm.lastVPNConnected,
		"vpn_interface":         nm.lastVPNInterface,
	}
}
