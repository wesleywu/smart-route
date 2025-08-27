package routing

import (
	"fmt"
	"net"
	"time"

	"github.com/wesleywu/smart-route/internal/config"
	"github.com/wesleywu/smart-route/internal/logger"
	"github.com/wesleywu/smart-route/internal/routing/entities"
)

// RouteSwitch handles the complete route switching logic used by both one-time and daemon modes
type RouteSwitch struct {
	rm         entities.RouteManager
	logger     *logger.Logger
	chnRoutes  *config.IPSet
	chnDNS     *config.DNSServers
}

// NewRouteSwitch creates a new route switch handler
func NewRouteSwitch(rm entities.RouteManager, chnRoutes *config.IPSet, chnDNS *config.DNSServers, logger *logger.Logger) (*RouteSwitch, error) {
	return &RouteSwitch{
		rm:         rm,
		chnRoutes:  chnRoutes,
		chnDNS:     chnDNS,
		logger:     logger,
	}, nil
}

// SetupRoutes performs complete route reset - used by both one-time and daemon modes
// This is the unified logic: always cleanup ALL managed routes, then setup for current gateway
func (rs *RouteSwitch) SetupRoutes(gateway net.IP) error {
	if gateway == nil {
		return fmt.Errorf("gateway cannot be nil")
	}

	rs.logger.Debug("Route reset started",
		"gateway", gateway.String())

	// Phase 1: Clean up ALL managed routes (completely gateway-independent)
	rs.logger.Debug("Phase 1: cleaning up all managed routes")
	if err := rs.CleanRoutes(); err != nil {
		rs.logger.Error("failed to cleanup managed routes", "error", err)
		return fmt.Errorf("failed to cleanup managed routes: %w", err)
	}

	// time.Sleep(5*time.Second)
	// currentRoutes, err := rs.rm.ListRoutes()
	// if err != nil {
	// 	return fmt.Errorf("failed to list current routes: %w", err)
	// }
	// rs.logger.Info("current system routes after cleanup", "total_count", len(currentRoutes))

	// Phase 2: Set up routes for current gateway
	rs.logger.Debug("Phase 2: setting up routes for current gateway")
	if err := rs.addRoutes(gateway); err != nil {
		rs.logger.Error("failed to setup routes for current gateway", "gateway", gateway.String(), "error", err)
		return fmt.Errorf("failed to setup routes for current gateway: %w", err)
	}

	// time.Sleep(5*time.Second)
	// currentRoutes, err = rs.rm.ListRoutes()
	// if err != nil {
	// 	return fmt.Errorf("failed to list current routes: %w", err)
	// }
	// rs.logger.Info("current system routes after setup", "total_count", len(currentRoutes))

	rs.logger.Info("Smart routing configured",
		"gateway", gateway.String())

	return nil
}

// addRoutes adds all managed routes for the specified gateway
func (rs *RouteSwitch) addRoutes(gateway net.IP) error {
	start := time.Now()

	routes := rs.buildRoutes(gateway)
	if len(routes) == 0 {
		return nil
	}

	rs.logger.Debug("Setting up routes", "gateway", gateway.String(), "count", len(routes))

	err := rs.rm.BatchAddRoutes(routes, rs.logger)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		rs.logger.Error("failed to setup routes", "gateway", gateway.String(), "error", err, "duration_ms", duration)
		return err
	}

	rs.logger.Debug("Routes added", "gateway", gateway.String(), "count", len(routes), "duration_ms", duration)
	return nil
}

// CleanRoutes removes all routes for networks defined in Chinese DNS and route files
func (rs *RouteSwitch) CleanRoutes() error {
	start := time.Now()

	rs.logger.Debug("Cleaning up managed routes")
	routes := rs.buildRoutes(nil)
	if len(routes) == 0 {
		return nil
	}

	rs.logger.Debug("Starting complete route cleanup", "managed_networks_count", len(routes))

	// Step 2: Get all current routes from system
	currentRoutes, err := rs.rm.ListSystemRoutes()
	if err != nil {
		return fmt.Errorf("failed to list current routes: %w", err)
	}

	rs.logger.Debug("Retrieved system routes", "total_count", len(currentRoutes))

	// Step 3: Find all routes that match our managed networks
	routesToDelete := rs.findMatchingRoutes(routes, currentRoutes)

	rs.logger.Debug("Found routes to cleanup", "count", len(routesToDelete))

	// Step 4: Delete all matching routes
	if len(routesToDelete) == 0 {
		rs.logger.Debug("No routes to clean up")
		return nil
	}

	err = rs.rm.BatchDeleteRoutes(routesToDelete, rs.logger)
	if err != nil {
		rs.logger.Error("failed to delete routes", "error", err)
		return fmt.Errorf("failed to delete routes: %w", err)
	}

	rs.logger.Debug("Routes deleted", "count", len(routesToDelete), "duration_ms", time.Since(start).Milliseconds())
	return nil
}

// buildRoutes creates route list for the specified gateway
func (rs *RouteSwitch) buildRoutes(gateway net.IP) []entities.Route {
	var routes []entities.Route

	// Add Chinese network routes
	networks := rs.chnRoutes.GetNetworks()
	for _, network := range networks {
		routes = append(routes, entities.Route{
			Destination: network,
			Gateway:     gateway,
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

		routes = append(routes, entities.Route{
			Destination: ipNet,
			Gateway:     gateway,
		})
	}

	return routes
}

// findMatchingRoutes finds all system routes that match our managed networks
func (rs *RouteSwitch) findMatchingRoutes(managedNetworks []entities.Route, systemRoutes []entities.Route) []entities.Route {
	var matchingRoutes []entities.Route

	for _, managedNetwork := range managedNetworks {
		for _, systemRoute := range systemRoutes {
			if rs.networksEqual(managedNetwork.Destination, systemRoute.Destination) {
				matchingRoutes = append(matchingRoutes, systemRoute)
				rs.logger.Debug("found matching route",
					"network", systemRoute.Destination.String(),
					"gateway", systemRoute.Gateway.String(),
					"interface", systemRoute.Interface)
			}
		}
	}

	return matchingRoutes
}

// networksEqual checks if two networks are equivalent
func (rs *RouteSwitch) networksEqual(net1, net2 net.IPNet) bool {
	// Check if IPs are equal
	if !net1.IP.Equal(net2.IP) {
		return false
	}

	// Get mask bit counts
	bits1, size1 := net1.Mask.Size()
	bits2, size2 := net2.Mask.Size()

	// Both should be the same type (IPv4 or IPv6) and have same mask bits
	return size1 == size2 && bits1 == bits2
}
