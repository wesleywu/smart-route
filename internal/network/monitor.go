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
		gateway:      gateway,
		defaultIface: iface,
		eventChan:    make(chan NetworkEvent, 100),
		stopChan:     make(chan struct{}),
		pollInterval: pollInterval,
	}, nil
}

func (nm *NetworkMonitor) Start() error {
	nm.mutex.Lock()
	defer nm.mutex.Unlock()

	if nm.isRunning {
		return fmt.Errorf("network monitor is already running")
	}

	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		sock, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
		if err != nil {
			return fmt.Errorf("failed to create route socket: %w", err)
		}
		nm.routeSocket = sock
		go nm.monitorRouteSocket()
	}

	go nm.monitorPolling()
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
				continue
			}

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

func (nm *NetworkMonitor) monitorPolling() {
	ticker := time.NewTicker(nm.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-nm.stopChan:
			return
		case <-ticker.C:
			nm.checkGatewayChange()
		}
	}
}

func (nm *NetworkMonitor) checkGatewayChange() {
	currentGateway, currentIface, err := GetDefaultGateway()
	if err != nil {
		return
	}

	nm.mutex.Lock()
	gatewayChanged := !nm.gateway.Equal(currentGateway)
	interfaceChanged := nm.defaultIface != currentIface
	
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

	return &NetworkEvent{
		Type:      AddressChanged,
		Interface: "",
		Gateway:   nil,
		Timestamp: time.Now(),
	}
}