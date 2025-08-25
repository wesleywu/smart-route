package routing

import (
	"fmt"
	"net"

	"github.com/wesleywu/update-routes-native/internal/config"
	"github.com/wesleywu/update-routes-native/internal/logger"
)

// CleanupRoutesForGateway is a function that cleans up routes for a specific gateway
// It deletes all routes that are associated with the gateway
// It also deletes all routes that are associated with the Chinese network and DNS
// It is used to clean up routes when the gateway changes
// cleanupRoutesForGatewayImpl is the working implementation copied from main.go
func CleanupRoutesForGateway(rm RouteManager, chnRoutes *config.IPSet, chnDNS *config.DNSServers, gateway net.IP, log *logger.Logger) error {
	if gateway == nil {
		return fmt.Errorf("gateway cannot be nil")
	}

	log.Info("Cleaning routes for specific gateway", "gateway", gateway.String())

	// Build routes to delete for this specific gateway
	var routesToDelete []Route

	// Add Chinese network routes
	for _, network := range chnRoutes.GetNetworks() {
		routesToDelete = append(routesToDelete, Route{
			Network: network,  // Now using value instead of pointer
			Gateway: gateway,
		})
	}

	// Add Chinese DNS routes
	for _, ip := range chnDNS.GetIPs() {
		var network net.IPNet
		if ip.To4() != nil {
			network = net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
		} else {
			network = net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
		}
		routesToDelete = append(routesToDelete, Route{
			Network: network,
			Gateway: gateway,
		})
	}

	// Remove duplicates before deleting
	uniqueRoutes := removeDuplicateRoutes(routesToDelete)
	if len(uniqueRoutes) != len(routesToDelete) {
		log.Debug("Removed duplicate routes", "original_count", len(routesToDelete), "unique_count", len(uniqueRoutes))
	}
	
	// Try to delete these routes
	if len(uniqueRoutes) > 0 {
		log.Info("Attempting to delete routes", "gateway", gateway.String(), "count", len(uniqueRoutes))
		err := rm.BatchDeleteRoutes(uniqueRoutes, log)
		if err != nil {
			return fmt.Errorf("failed to delete routes for gateway %s: %w", gateway.String(), err)
		}
		log.Info("Successfully cleaned routes", "gateway", gateway.String(), "count", len(uniqueRoutes))
	}

	return nil
}

// removeDuplicateRoutes removes duplicate routes based on network and gateway
func removeDuplicateRoutes(routes []Route) []Route {
	seen := make(map[string]bool)
	var uniqueRoutes []Route
	
	for _, route := range routes {
		// Create a unique key based on network and gateway
		key := fmt.Sprintf("%s->%s", route.Network.String(), route.Gateway.String())
		if !seen[key] {
			seen[key] = true
			uniqueRoutes = append(uniqueRoutes, route)
		}
	}
	
	return uniqueRoutes
}
