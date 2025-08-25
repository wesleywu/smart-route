package network

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

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
	
	// 智能轮询控制
	pollEnabled       bool
	pollTicker        *time.Ticker
	pollStopChan      chan struct{}
	lastEventTime     time.Time
	routeSocketErrors int
	maxSocketErrors   int
	healthCheckInterval time.Duration
}

type NetworkEvent struct {
	Type      EventType
	Interface string
	Gateway   net.IP
	Timestamp time.Time
}

type EventType int

const (
	GatewayChanged EventType = iota
	InterfaceUp
	InterfaceDown
	AddressChanged
)

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
	default:
		return "Unknown"
	}
}

func NewNetworkMonitor(pollInterval time.Duration) (*NetworkMonitor, error) {
	gateway, iface, err := GetDefaultGateway()
	if err != nil {
		return nil, fmt.Errorf("failed to get initial gateway: %w", err)
	}

	return &NetworkMonitor{
		gateway:             gateway,
		defaultIface:        iface,
		eventChan:           make(chan NetworkEvent, 100),
		stopChan:            make(chan struct{}),
		pollInterval:        pollInterval,
		pollStopChan:        make(chan struct{}),
		pollEnabled:         false, // 默认禁用轮询
		maxSocketErrors:     3,     // 连续3次socket错误后启用轮询
		healthCheckInterval: 30 * time.Second, // 30秒健康检查间隔
		lastEventTime:       time.Now(),
	}, nil
}

func (nm *NetworkMonitor) Start() error {
	nm.mutex.Lock()
	defer nm.mutex.Unlock()

	if nm.isRunning {
		return fmt.Errorf("network monitor is already running")
	}

	// 尝试创建route socket进行实时监控
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		sock, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
		if err != nil {
			// socket创建失败，立即启用轮询作为备用
			fmt.Printf("Failed to create route socket, enabling polling: %v\n", err)
			nm.pollEnabled = true
		} else {
			nm.routeSocket = sock
			go nm.monitorRouteSocket()
		}
	} else {
		// 非支持平台，启用轮询
		nm.pollEnabled = true
	}

	// 启动健康检查协程
	go nm.healthCheck()
	
	// 如果需要轮询，则启动轮询
	if nm.pollEnabled {
		nm.startPolling()
	}
	
	nm.isRunning = true
	return nil
}

func (nm *NetworkMonitor) Stop() error {
	nm.mutex.Lock()
	defer nm.mutex.Unlock()

	if !nm.isRunning {
		return nil
	}

	close(nm.stopChan)
	
	// 停止轮询
	nm.stopPolling()
	
	if nm.routeSocket > 0 {
		unix.Close(nm.routeSocket)
		nm.routeSocket = 0
	}

	nm.isRunning = false
	return nil
}

func (nm *NetworkMonitor) Events() <-chan NetworkEvent {
	return nm.eventChan
}

func (nm *NetworkMonitor) GetCurrentGateway() (net.IP, string) {
	nm.mutex.RLock()
	defer nm.mutex.RUnlock()
	
	gateway := make(net.IP, len(nm.gateway))
	copy(gateway, nm.gateway)
	
	return gateway, nm.defaultIface
}

func (nm *NetworkMonitor) monitorRouteSocket() {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return
	}

	buffer := make([]byte, 4096)
	for {
		select {
		case <-nm.stopChan:
			return
		default:
			n, err := unix.Read(nm.routeSocket, buffer)
			if err != nil {
				nm.mutex.Lock()
				nm.routeSocketErrors++
				if nm.routeSocketErrors >= nm.maxSocketErrors && !nm.pollEnabled {
					fmt.Printf("Route socket errors exceeded threshold (%d), enabling polling\n", nm.routeSocketErrors)
					nm.pollEnabled = true
					nm.mutex.Unlock()
					nm.startPolling()
				} else {
					nm.mutex.Unlock()
				}
				continue
			}

			// 重置错误计数
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

// 健康检查：监控实时事件的有效性，必要时启用轮询
func (nm *NetworkMonitor) healthCheck() {
	ticker := time.NewTicker(nm.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-nm.stopChan:
			return
		case <-ticker.C:
			nm.mutex.RLock()
			timeSinceLastEvent := time.Since(nm.lastEventTime)
			pollEnabled := nm.pollEnabled
			routeSocketErrors := nm.routeSocketErrors
			nm.mutex.RUnlock()

			// 如果超过健康检查间隔的2倍时间没有收到事件，可能实时监控有问题
			if !pollEnabled && timeSinceLastEvent > 2*nm.healthCheckInterval {
				fmt.Printf("No route events for %v, enabling polling as backup\n", timeSinceLastEvent)
				nm.mutex.Lock()
				nm.pollEnabled = true
				nm.mutex.Unlock()
				nm.startPolling()
			}

			// 如果route socket已经恢复且轮询已启用一段时间，尝试恢复纯事件模式
			if pollEnabled && routeSocketErrors == 0 && timeSinceLastEvent < nm.healthCheckInterval/2 {
				fmt.Printf("Route socket appears healthy, disabling polling\n")
				nm.mutex.Lock()
				nm.pollEnabled = false
				nm.mutex.Unlock()
				nm.stopPolling()
			}
		}
	}
}

// 启动轮询
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

// 停止轮询
func (nm *NetworkMonitor) stopPolling() {
	if nm.pollTicker != nil {
		close(nm.pollStopChan)
		nm.pollTicker = nil
		fmt.Printf("Polling stopped\n")
	}
}

func (nm *NetworkMonitor) checkGatewayChange() {
	currentGateway, currentIface, err := GetDefaultGateway()
	if err != nil {
		// Add debug info - this should rarely happen now
		fmt.Printf("DEBUG: Failed to get gateway during check: %v\n", err)
		return
	}

	nm.mutex.Lock()
	oldGateway := nm.gateway
	oldIface := nm.defaultIface
	gatewayChanged := !nm.gateway.Equal(currentGateway)
	interfaceChanged := nm.defaultIface != currentIface
	
	// Add debug logging
	fmt.Printf("DEBUG: Gateway check - Old: %s (%s), Current: %s (%s), Changed: %t\n", 
		oldGateway.String(), oldIface, currentGateway.String(), currentIface, gatewayChanged || interfaceChanged)
	
	if gatewayChanged || interfaceChanged {
		nm.gateway = currentGateway
		nm.defaultIface = currentIface
		nm.mutex.Unlock()

		event := NetworkEvent{
			Type:      GatewayChanged,
			Interface: currentIface,
			Gateway:   currentGateway,
			Timestamp: time.Now(),
		}

		select {
		case nm.eventChan <- event:
		case <-nm.stopChan:
			return
		}
	} else {
		nm.mutex.Unlock()
	}
}

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

// 获取监控器状态
func (nm *NetworkMonitor) GetMonitorStatus() map[string]interface{} {
	nm.mutex.RLock()
	defer nm.mutex.RUnlock()

	return map[string]interface{}{
		"is_running":             nm.isRunning,
		"poll_enabled":           nm.pollEnabled,
		"route_socket":           nm.routeSocket > 0,
		"route_socket_errors":    nm.routeSocketErrors,
		"last_event_time":        nm.lastEventTime,
		"time_since_last_event":  time.Since(nm.lastEventTime),
		"health_check_interval":  nm.healthCheckInterval,
		"poll_interval":          nm.pollInterval,
	}
}