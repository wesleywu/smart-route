package routing

import (
	"fmt"
	"net"
	"time"

	"github.com/wesleywu/update-routes-native/internal/config"
	"github.com/wesleywu/update-routes-native/internal/logger"
)

// RouteSwitch handles the complete route switching logic used by both one-time and daemon modes
type RouteSwitch struct {
	rm            RouteManager
	cleanupMgr    *CleanupManager
	logger        *logger.Logger
	chnRoutes     *config.IPSet
	chnDNS        *config.DNSServers
}

// NewRouteSwitch creates a new route switch handler
func NewRouteSwitch(rm RouteManager, chnRoutes *config.IPSet, chnDNS *config.DNSServers, logger *logger.Logger, chnRoutesFile, chnDNSFile string) (*RouteSwitch, error) {
	cleanupMgr, err := NewCleanupManager(rm, logger, chnRoutesFile, chnDNSFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create cleanup manager: %w", err)
	}
	
	return &RouteSwitch{
		rm:            rm,
		chnRoutes:     chnRoutes,
		chnDNS:        chnDNS,
		logger:        logger,
		cleanupMgr:    cleanupMgr,
	}, nil
}

// SetupRoutes performs complete route reset - used by both one-time and daemon modes
// This is the unified logic: always cleanup ALL managed routes, then setup for current gateway
func (rs *RouteSwitch) SetupRoutes(gateway net.IP, iface string) error {
	if gateway == nil {
		return fmt.Errorf("gateway cannot be nil")
	}

	rs.logger.Info("performing complete route reset",
		"gateway", gateway.String(),
		"interface", iface)

	// Phase 1: Clean up ALL managed routes (completely gateway-independent)
	rs.logger.Info("phase 1: cleaning up all managed routes")
	if err := rs.cleanupAllManagedRoutes(); err != nil {
		rs.logger.Error("failed to cleanup managed routes", "error", err)
		return fmt.Errorf("failed to cleanup managed routes: %w", err)
	}

	// Phase 2: Set up routes for current gateway
	rs.logger.Info("phase 2: setting up routes for current gateway")
	if err := rs.setupRoutes(gateway); err != nil {
		rs.logger.Error("failed to setup routes for current gateway", "gateway", gateway.String(), "error", err)
		return fmt.Errorf("failed to setup routes for current gateway: %w", err)
	}

	rs.logger.Info("route reset completed successfully",
		"gateway", gateway.String(),
		"interface", iface)

	return nil
}

// cleanupAllManagedRoutes removes ALL routes in the system that match networks from config files
// This has NOTHING to do with any specific gateway - it deletes ALL matching routes regardless of gateway
func (rs *RouteSwitch) cleanupAllManagedRoutes() error {
	start := time.Now()
	
	rs.logger.Info("cleaning up ALL managed routes from system (gateway-independent)")
	
	// Use the new complete cleanup manager - this is completely gateway-agnostic
	err := rs.cleanupMgr.CleanupAllManagedRoutes()
	duration := time.Since(start).Milliseconds()
	
	if err != nil {
		rs.logger.Error("failed to cleanup managed routes", "error", err, "duration_ms", duration)
		return err
	}
	
	rs.logger.Info("complete route cleanup finished", "duration_ms", duration)
	return nil
}

// setupRoutes adds all managed routes for the specified gateway
func (rs *RouteSwitch) setupRoutes(gateway net.IP) error {
	start := time.Now()
	
	routes := rs.buildRoutes(gateway)
	if len(routes) == 0 {
		return nil
	}

	rs.logger.Info("setting up routes", "gateway", gateway.String(), "count", len(routes))

	err := rs.rm.BatchAddRoutes(routes, rs.logger)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		rs.logger.Error("failed to setup routes", "gateway", gateway.String(), "error", err, "duration_ms", duration)
		return err
	}

	rs.logger.Info("route setup completed", "gateway", gateway.String(), "count", len(routes), "duration_ms", duration)
	return nil
}

// buildRoutes creates route list for the specified gateway
func (rs *RouteSwitch) buildRoutes(gateway net.IP) []Route {
	var routes []Route

	// Add Chinese network routes
	networks := rs.chnRoutes.GetNetworks()
	for _, network := range networks {
		routes = append(routes, Route{
			Network: network,
			Gateway: gateway,
		})
	}

	// Add Chinese DNS routes
	dnsIPs := rs.chnDNS.GetIPs()
	for _, ip := range dnsIPs {
		var ipNet net.IPNet
		if ip.To4() != nil {
			ipNet = net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
		} else {
			ipNet = net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
		}
		
		routes = append(routes, Route{
			Network: ipNet,
			Gateway: gateway,
		})
	}

	return routes
}

