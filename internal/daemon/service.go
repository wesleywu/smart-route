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

	gw, iface, err := sm.router.GetDefaultGateway()
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
	sm.logger.NetworkChange(
		event.Type.String(),
		event.Interface,
		sm.currentGW.String(),
		event.Gateway.String(),
	)

	switch event.Type {
	case network.GatewayChanged:
		if err := sm.handleGatewayChange(event.Gateway, event.Interface); err != nil {
			sm.logger.Error("failed to handle gateway change", "error", err)
		}
	}
}

func (sm *ServiceManager) handleGatewayChange(newGW net.IP, newIface string) error {
	sm.mutex.Lock()
	oldGW := sm.currentGW
	oldIface := sm.currentIface
	sm.currentGW = newGW
	sm.currentIface = newIface
	sm.mutex.Unlock()

	if err := sm.flushOldRoutes(oldGW); err != nil {
		sm.logger.Error("failed to flush old routes", "gateway", oldGW.String(), "error", err)
	}

	if err := sm.setupRoutesForGateway(newGW); err != nil {
		return fmt.Errorf("failed to setup routes for new gateway %s: %w", newGW.String(), err)
	}

	sm.logger.Info("gateway change handled successfully",
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

	err := sm.router.BatchAddRoutes(routes)
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
	
	err := sm.router.FlushRoutes(gateway)
	duration := time.Since(start).Milliseconds()
	
	if err != nil {
		sm.logger.Error("failed to flush routes", "gateway", gateway.String(), "duration_ms", duration, "error", err)
		return err
	}

	sm.logger.Info("old routes flushed", "gateway", gateway.String(), "duration_ms", duration)
	return nil
}

func (sm *ServiceManager) buildRoutes(gateway net.IP) []routing.Route {
	var routes []routing.Route

	networks := sm.chnRoutes.GetNetworks()
	for _, network := range networks {
		routes = append(routes, routing.Route{
			Network: &network,
			Gateway: gateway,
		})
	}

	dnsIPs := sm.chnDNS.GetIPs()
	for _, ip := range dnsIPs {
		var network *net.IPNet
		if ip.To4() != nil {
			network = &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
		} else {
			network = &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
		}
		
		routes = append(routes, routing.Route{
			Network: network,
			Gateway: gateway,
		})
	}

	return routes
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