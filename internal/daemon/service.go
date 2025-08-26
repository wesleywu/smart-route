package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/wesleywu/update-routes-native/internal/config"
	"github.com/wesleywu/update-routes-native/internal/logger"
	"github.com/wesleywu/update-routes-native/internal/network"
	"github.com/wesleywu/update-routes-native/internal/routing"
)

// ServiceManager is a manager for the service
type ServiceManager struct {
	config       *config.Config
	logger       *logger.Logger
	monitor      *network.NetworkMonitor
	router       routing.RouteManager
	routeSwitch  *routing.RouteSwitch
	chnRoutes    *config.IPSet
	chnDNS       *config.DNSServers
	stopChan     chan os.Signal
	doneChan     chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	mutex        sync.RWMutex
	isRunning    bool
	currentGW    net.IP
	currentIface string
	lastCheck    time.Time
}

// NewServiceManager creates a new ServiceManager
func NewServiceManager(cfg *config.Config, log *logger.Logger) (*ServiceManager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	sm := &ServiceManager{
		config:   cfg,
		logger:   log.WithComponent("service"),
		stopChan: make(chan os.Signal, 1),
		doneChan: make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	var err error
	sm.router, err = routing.NewRouteManager(cfg.ConcurrencyLimit, cfg.RetryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to create route manager: %w", err)
	}

	sm.monitor, err = network.NewNetworkMonitor(cfg.MonitorInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to create network monitor: %w", err)
	}

	sm.chnRoutes, err = config.LoadChnRoutes(cfg.ChnRouteFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load Chinese routes: %w", err)
	}

	sm.chnDNS, err = config.LoadChnDNS(cfg.ChnDNSFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load Chinese DNS: %w", err)
	}

	// Initialize route switch with unified logic
	sm.routeSwitch, err = routing.NewRouteSwitch(sm.router, sm.chnRoutes, sm.chnDNS, sm.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create route switch: %w", err)
	}

	return sm, nil
}

// Start starts the service
func (sm *ServiceManager) Start() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.isRunning {
		return fmt.Errorf("service is already running")
	}

	if os.Getuid() != 0 {
		return fmt.Errorf("root privileges required")
	}

	signal.Notify(sm.stopChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	sm.logger.ServiceStart("1.0.0", fmt.Sprintf("%d", os.Getpid()))
	sm.logger.ConfigLoaded(sm.config.ChnRouteFile, sm.chnRoutes.Size(), sm.chnDNS.Size())

	gw, iface, err := network.GetDefaultGateway()
	if err != nil {
		return fmt.Errorf("failed to get default gateway: %w", err)
	}
	sm.currentGW = gw
	sm.currentIface = iface

	if err := sm.setupInitialRoutes(); err != nil {
		return fmt.Errorf("failed to setup initial routes: %w", err)
	}

	if err := sm.monitor.Start(); err != nil {
		return fmt.Errorf("failed to start network monitor: %w", err)
	}

	sm.logger.MonitorStart(sm.config.MonitorInterval.String())

	go sm.serviceLoop()
	sm.isRunning = true

	return nil
}

// Stop stops the service
func (sm *ServiceManager) Stop() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if !sm.isRunning {
		return nil
	}

	sm.logger.ServiceStop()
	
	sm.cancel()
	close(sm.stopChan)

	if err := sm.monitor.Stop(); err != nil {
		sm.logger.Error("failed to stop network monitor", "error", err)
	}

	if err := sm.router.Close(); err != nil {
		sm.logger.Error("failed to close route manager", "error", err)
	}

	sm.logger.MonitorStop()
	sm.isRunning = false

	select {
	case <-sm.doneChan:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("service stop timeout")
	}
}

// Wait waits for the service to stop
func (sm *ServiceManager) Wait() error {
	select {
	case <-sm.ctx.Done():
		return sm.ctx.Err()
	case sig := <-sm.stopChan:
		sm.logger.Info("received signal", "signal", sig.String())
		return sm.Stop()
	}
}

// serviceLoop is the main loop for the service
func (sm *ServiceManager) serviceLoop() {
	defer close(sm.doneChan)
	
	for {
		select {
		case <-sm.ctx.Done():
			return
		case event := <-sm.monitor.Events():
			sm.handleNetworkEvent(event)
		}
	}
}

// handleNetworkEvent handles network events
func (sm *ServiceManager) handleNetworkEvent(event network.NetworkEvent) {
	gwStr := "<nil>"
	if event.Gateway != nil {
		gwStr = event.Gateway.String()
	}
	
	sm.logger.NetworkChange(
		event.Type.String(),
		event.Interface,
		sm.currentGW.String(),
		gwStr,
	)

	switch event.Type {
	case network.GatewayChanged:
		if event.Gateway != nil {
			if err := sm.handleGatewayChange(event.Gateway); err != nil {
				sm.logger.Error("failed to handle gateway change", "error", err)
			}
		}
	case network.AddressChanged:
		// For address changes, also check if gateway has changed
		// This is a backup mechanism in case gateway change detection is not perfect
		go func() {
			time.Sleep(500 * time.Millisecond) // Allow network to stabilize
			sm.checkAndHandleGatewayChange()
		}()
	}
}

// handleGatewayChange handles gateway changes
func (sm *ServiceManager) handleGatewayChange(newGW net.IP) error {
	sm.mutex.Lock()
	oldGW := sm.currentGW
	oldIface := sm.currentIface
	sm.mutex.Unlock()

	// Use unified route switch logic
	if err := sm.routeSwitch.SetupRoutes(newGW); err != nil {
		sm.logger.Error("failed to switch routes", "error", err)
		return err
	}

	// Update current gateway after successful transition
	sm.mutex.Lock()
	sm.currentGW = newGW
	sm.mutex.Unlock()

	// Note: Removed route cache flush as it was clearing all routes including the ones we just added
	// The route changes should take effect immediately without flushing the entire route cache

	sm.logger.Info("gateway change completed successfully",
		"old_gateway", sm.ipToString(oldGW),
		"old_interface", oldIface,
		"new_gateway", newGW.String())

	return nil
}

// setupInitialRoutes sets up initial routes
func (sm *ServiceManager) setupInitialRoutes() error {
	// Use unified route switch logic for initial setup
	return sm.routeSwitch.SetupRoutes(sm.currentGW)
}


// checkAndHandleGatewayChange checks and handles gateway changes
func (sm *ServiceManager) checkAndHandleGatewayChange() {
	sm.mutex.Lock()
	now := time.Now()
	// Prevent too frequent checks (minimum 2 seconds between checks)
	if now.Sub(sm.lastCheck) < 2*time.Second {
		sm.mutex.Unlock()
		return
	}
	sm.lastCheck = now
	sm.mutex.Unlock()

	currentGW, currentIface, err := network.GetDefaultGateway()
	if err != nil {
		sm.logger.Error("failed to get current gateway during check", "error", err)
		return
	}

	sm.mutex.RLock()
	gatewayChanged := !sm.currentGW.Equal(currentGW)
	interfaceChanged := sm.currentIface != currentIface
	oldGW := sm.currentGW
	oldIface := sm.currentIface
	sm.mutex.RUnlock()

	if gatewayChanged || interfaceChanged {
		sm.logger.Info("detected gateway change during check",
			"old_gateway", oldGW.String(),
			"old_interface", oldIface,
			"new_gateway", currentGW.String(),
			"new_interface", currentIface)

		if err := sm.handleGatewayChange(currentGW); err != nil {
			sm.logger.Error("failed to handle detected gateway change", "error", err)
		}
	}
}


// flushRouteCache was removed because it was causing route loss
// The 'route -n flush' command clears ALL routes from the system,
// including the ones we just added, which is not what we want.
// Route changes should take effect immediately without cache flushing.
//
// func (sm *ServiceManager) flushRouteCache() error {
// 	if runtime.GOOS == "darwin" {
// 		cmd := exec.Command("route", "-n", "flush")
// 		if err := cmd.Run(); err != nil {
// 			return fmt.Errorf("failed to flush route cache: %w", err)
// 		}
// 	}
// 	return nil
// }


// ipToString safely converts IP to string, handling nil
func (sm *ServiceManager) ipToString(ip net.IP) string {
	if ip == nil {
		return "<nil>"
	}
	return ip.String()
}

// IsRunning checks if the service is running
func (sm *ServiceManager) IsRunning() bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.isRunning
}

// GetStatus gets the status of the service
func (sm *ServiceManager) GetStatus() map[string]interface{} {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return map[string]interface{}{
		"running":           sm.isRunning,
		"current_gateway":   sm.currentGW.String(),
		"current_interface": sm.currentIface,
		"chn_routes":        sm.chnRoutes.Size(),
		"chn_dns":           sm.chnDNS.Size(),
	}
}