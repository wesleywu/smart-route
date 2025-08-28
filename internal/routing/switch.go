package routing

import (
	"fmt"
	"net"
	"time"

	"github.com/wesleywu/smart-route/internal/config"
	"github.com/wesleywu/smart-route/internal/logger"
	"github.com/wesleywu/smart-route/internal/routing/entities"
	"github.com/wesleywu/smart-route/internal/utils"
)

// RouteSwitch handles the complete route switching logic used by both one-time and daemon modes
type RouteSwitch struct {
	rm           entities.RouteManager
	managedIPSet *config.IPSet
	logger       *logger.Logger
}

// NewRouteSwitch creates a new route switch handler
func NewRouteSwitch(rm entities.RouteManager, managedIPSet *config.IPSet, logger *logger.Logger) (*RouteSwitch, error) {
	return &RouteSwitch{
		rm:           rm,
		managedIPSet: managedIPSet,
		logger:       logger,
	}, nil
}

// InitRoutes sets up initial routes only if VPN is already connected, or clean up routes if VPN is not connected
func (rs *RouteSwitch) InitRoutes() error {

	// Check current VPN state - only setup routes if VPN is connected
	currentGW, currentIface, err := rs.rm.GetSystemDefaultRoute()
	if err != nil {
		rs.logger.Error("failed to check VPN state during initial setup", "error", err)
		return fmt.Errorf("failed to check VPN state: %w", err)
	}

	// Check if VPN is connected by examining the interface
	isVPNConnected := utils.IsVPNInterface(currentIface)

	if !isVPNConnected {
		rs.logger.Info("VPN not connected - skipping route setup",
			"current_interface", currentIface,
			"current_gateway", currentGW.String())
		return rs.CleanRoutes()
	}

	rs.logger.Info("VPN detected - setting up routes",
		"vpn_interface", currentIface,
		"physical_gateway", currentGW.String())

	// VPN is connected, use physical gateway for route setup
	physicalGateway, _, err := rs.rm.GetPhysicalGateway()
	if err != nil {
		return fmt.Errorf("failed to get physical gateway: %w", err)
	}
	return rs.SetupRoutes(physicalGateway)
}

// SetupRoutes performs complete route reset - used by both one-time and daemon modes
// This is the unified logic: always cleanup ALL managed routes, then setup for current gateway
func (rs *RouteSwitch) SetupRoutes(physicalGateway net.IP) error {
	if physicalGateway == nil {
		return fmt.Errorf("gateway cannot be nil")
	}

	rs.logger.Debug("Route reset started",
		"physical_gateway", physicalGateway.String())

	// Phase 1: Clean up ALL managed routes (completely gateway-independent)
	rs.logger.Debug("Phase 1: cleaning up system routes within managed routes")

	systemRoutes, err := rs.rm.ListSystemRoutes()
	if err != nil {
		return fmt.Errorf("failed to fetch current system routes: %w", err)
	}
	rs.logger.Debug("Retrieved system routes", "total_count", len(systemRoutes))

	existingRoutes := findMatchingRoute(systemRoutes, rs.managedIPSet)

	if err := rs.cleanRoutes(existingRoutes); err != nil {
		rs.logger.Error("failed to cleanup managed routes", "error", err)
		return fmt.Errorf("failed to cleanup managed routes: %w", err)
	}

	// Phase 2: Set up routes for current gateway
	rs.logger.Debug("Phase 2: setting up routes for current gateway")

	routesToAdd := buildRoutesFromIPSet(rs.managedIPSet, physicalGateway)

	if err := rs.addRoutes(routesToAdd); err != nil {
		rs.logger.Error("failed to setup routes for current gateway", "gateway", physicalGateway.String(), "error", err)
		return fmt.Errorf("failed to setup routes for current gateway: %w", err)
	}

	rs.logger.Info("Smart routing configured",
		"gateway", physicalGateway.String())

	return nil
}

// CleanRoutes cleans up all routes that are managed by the route switch
func (rs *RouteSwitch) CleanRoutes() error {
	rs.logger.Debug("Starting complete route cleanup")

	systemRoutes, err := rs.rm.ListSystemRoutes()
	if err != nil {
		return fmt.Errorf("failed to fetch current system routes: %w", err)
	}
	rs.logger.Debug("Retrieved system routes", "total_count", len(systemRoutes))

	existingRoutes := findMatchingRoute(systemRoutes, rs.managedIPSet)

	return rs.cleanRoutes(existingRoutes)
}

// addRoutes adds all managed routes for the specified gateway
func (rs *RouteSwitch) addRoutes(routesToAdd []*entities.Route) error {
	start := time.Now()

	rs.logger.Debug("Setting up routes", "routes to setup:", len(routesToAdd))

	err := rs.rm.BatchAddRoutes(routesToAdd, rs.logger)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		rs.logger.Error("failed to setup routes", "error", err, "duration_ms", duration)
		return err
	}

	rs.logger.Debug("Routes added", "count", len(routesToAdd), "duration_ms", duration)
	return nil
}

// CleanRoutes removes all routes for networks defined in Chinese DNS and route files
func (rs *RouteSwitch) cleanRoutes(routesToDelete []*entities.Route) error {
	start := time.Now()

	rs.logger.Debug("Starting complete route cleanup", "routes to delete: ", len(routesToDelete))

	if len(routesToDelete) == 0 {
		rs.logger.Debug("No routes to clean up")
		return nil
	}

	err := rs.rm.BatchDeleteRoutes(routesToDelete, rs.logger)
	if err != nil {
		rs.logger.Error("failed to delete routes", "error", err)
		return fmt.Errorf("failed to delete routes: %w", err)
	}

	rs.logger.Debug("Routes deleted", "count", len(routesToDelete), "duration_ms", time.Since(start).Milliseconds())
	return nil
}

func findMatchingRoute(systemRoutes []*entities.Route, managedRouteSet *config.IPSet) []*entities.Route {
	matchingRoutes := make([]*entities.Route, 0)
	for _, route := range systemRoutes {
		if managedRouteSet.ContainsIPNet(route.Destination) {
			matchingRoutes = append(matchingRoutes, route)
		}
	}
	return matchingRoutes
}

func buildRoutesFromIPSet(ipSet *config.IPSet, gateway net.IP) []*entities.Route {
	routes := make([]*entities.Route, 0)
	for _, network := range ipSet.IPNets() {
		routes = append(routes, &entities.Route{
			Destination: *network,
			Gateway:     gateway,
		})
	}
	return routes
}
