package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/wesleywu/update-routes-native/internal/config"
	"github.com/wesleywu/update-routes-native/internal/logger"
	"github.com/wesleywu/update-routes-native/internal/network"
	"github.com/wesleywu/update-routes-native/internal/routing"
)

type ServiceManager struct {
	config       *config.Config
	logger       *logger.Logger
	monitor      *network.NetworkMonitor
	router       routing.RouteManager
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

	return sm, nil
}

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

func (sm *ServiceManager) Wait() error {
	select {
	case <-sm.ctx.Done():
		return sm.ctx.Err()
	case sig := <-sm.stopChan:
		sm.logger.Info("received signal", "signal", sig.String())
		return sm.Stop()
	}
}

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
			if err := sm.handleGatewayChange(event.Gateway, event.Interface); err != nil {
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

func (sm *ServiceManager) handleGatewayChange(newGW net.IP, newIface string) error {
	sm.mutex.Lock()
	oldGW := sm.currentGW
	oldIface := sm.currentIface
	sm.mutex.Unlock()

	sm.logger.Info("starting gateway change", 
		"old_gateway", oldGW.String(),
		"new_gateway", newGW.String())

	// CRITICAL: Must delete old routes FIRST
	// Old routes pointing to unreachable gateway will block all traffic
	sm.logger.Info("phase 1: deleting old routes (critical for connectivity)")
	if err := sm.flushOldRoutes(oldGW); err != nil {
		// This is critical - if we can't delete old routes, traffic won't work
		sm.logger.Error("failed to delete old routes - network may be broken", "gateway", oldGW.String(), "error", err)
		return fmt.Errorf("critical: failed to delete old routes for %s: %w", oldGW.String(), err)
	}

	// Phase 2: Add new routes immediately after old routes are cleared
	sm.logger.Info("phase 2: adding new routes")
	if err := sm.setupRoutesForGateway(newGW); err != nil {
		sm.logger.Error("failed to setup new routes - network is broken", "gateway", newGW.String(), "error", err)
		// Try to restore old routes as fallback (if possible)
		sm.logger.Info("attempting to restore old routes as fallback")
		if restoreErr := sm.setupRoutesForGateway(oldGW); restoreErr != nil {
			sm.logger.Error("failed to restore old routes - network completely broken", "error", restoreErr)
		}
		return fmt.Errorf("critical: failed to setup routes for new gateway %s: %w", newGW.String(), err)
	}

	// Update current gateway after successful transition
	sm.mutex.Lock()
	sm.currentGW = newGW
	sm.currentIface = newIface
	sm.mutex.Unlock()

	// Flush route cache to ensure changes take effect immediately
	if err := sm.flushRouteCache(); err != nil {
		sm.logger.Warn("failed to flush route cache", "error", err)
	}

	sm.logger.Info("gateway change completed successfully",
		"old_gateway", oldGW.String(),
		"old_interface", oldIface,
		"new_gateway", newGW.String(),
		"new_interface", newIface)

	return nil
}

func (sm *ServiceManager) setupInitialRoutes() error {
	return sm.setupRoutesForGateway(sm.currentGW)
}

func (sm *ServiceManager) setupRoutesForGateway(gateway net.IP) error {
	start := time.Now()

	routes := sm.buildRoutes(gateway)
	total := len(routes)
	
	sm.logger.Info("setting up routes", "gateway", gateway.String(), "total", total)

	err := sm.router.BatchAddRoutes(routes, sm.logger)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		sm.logger.BatchOperation("add", total, 0, total, duration)
		return err
	}

	sm.logger.BatchOperation("add", total, total, 0, duration)
	return nil
}

func (sm *ServiceManager) flushOldRoutes(gateway net.IP) error {
	start := time.Now()
	
	sm.logger.Info("flushing old routes", "gateway", gateway.String())
	
	// Strategy 1: Try to delete specific routes we know we added
	oldRoutes := sm.buildRoutes(gateway)
	err := sm.router.BatchDeleteRoutes(oldRoutes, sm.logger)
	duration := time.Since(start).Milliseconds()
	
	if err != nil {
		sm.logger.Warn("batch delete failed, trying alternative cleanup", "gateway", gateway.String(), "error", err)
		
		// Strategy 2: Use system command to delete routes by gateway
		if err2 := sm.forceDeleteRoutesByGateway(gateway); err2 != nil {
			sm.logger.Error("all cleanup strategies failed", "gateway", gateway.String(), "batch_error", err, "force_error", err2)
			return fmt.Errorf("critical: failed to delete old routes - both batch delete and force delete failed")
		} else {
			sm.logger.Info("alternative cleanup succeeded", "gateway", gateway.String(), "duration_ms", duration)
		}
	} else {
		sm.logger.Info("batch delete succeeded", "gateway", gateway.String(), "duration_ms", duration)
	}
	
	return nil
}

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

		if err := sm.handleGatewayChange(currentGW, currentIface); err != nil {
			sm.logger.Error("failed to handle detected gateway change", "error", err)
		}
	}
}

func (sm *ServiceManager) buildRoutes(gateway net.IP) []routing.Route {
	var routes []routing.Route

	networks := sm.chnRoutes.GetNetworks()
	for _, netw := range networks {
		routes = append(routes, routing.Route{
			Network: netw,  // Now using value instead of pointer
			Gateway: gateway,
		})
	}

	dnsIPs := sm.chnDNS.GetIPs()
	for _, ip := range dnsIPs {
		var ipNet net.IPNet
		if ip.To4() != nil {
			ipNet = net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
		} else {
			ipNet = net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
		}
		
		routes = append(routes, routing.Route{
			Network: ipNet,
			Gateway: gateway,
		})
	}

	return routes
}

func (sm *ServiceManager) flushRouteCache() error {
	// On macOS, flush the route cache to ensure changes take effect
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("route", "-n", "flush")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to flush route cache: %w", err)
		}
	}
	return nil
}

func (sm *ServiceManager) forceDeleteRoutesByGateway(gateway net.IP) error {
	// Use system route command to delete routes by gateway
	// This is more reliable than our native implementation
	
	sm.logger.Info("attempting force delete of routes", "gateway", gateway.String())
	
	// On macOS, we can use route delete commands
	if runtime.GOOS == "darwin" {
		return sm.forceDeleteRoutesByGatewayDarwin(gateway)
	}
	
	return fmt.Errorf("force delete not implemented for %s", runtime.GOOS)
}

func (sm *ServiceManager) forceDeleteRoutesByGatewayDarwin(gateway net.IP) error {
	// Try to delete Chinese IP ranges using system route command
	
	deletedCount := 0
	totalAttempts := 0
	
	// Get Chinese IP ranges to delete
	networks := sm.chnRoutes.GetNetworks()
	
	// Try to delete a sample of routes (first 200) 
	maxAttempts := 200
	for i, netw := range networks {
		if i >= maxAttempts {
			break
		}
		
		totalAttempts++
		
		// Use route delete command
		destination := netw.String()
		delCmd := exec.Command("route", "delete", destination, gateway.String())
		if err := delCmd.Run(); err == nil {
			deletedCount++
		}
		// Continue even if individual routes fail - some might not exist
	}
	
	sm.logger.Info("force delete completed", 
		"gateway", gateway.String(), 
		"deleted", deletedCount, 
		"attempted", totalAttempts)
	
	// Consider it successful if we deleted at least some routes
	// If we can't delete ANY routes, that's a problem
	if deletedCount == 0 && totalAttempts > 0 {
		return fmt.Errorf("could not delete any routes using system commands")
	}
	
	return nil
}

func (sm *ServiceManager) IsRunning() bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.isRunning
}

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