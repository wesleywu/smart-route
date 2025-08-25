package routing

import (
	"fmt"
	"net"

	"github.com/wesleywu/update-routes-native/internal/config"
	"github.com/wesleywu/update-routes-native/internal/logger"
)

// CleanupRoutesForGateway cleans up routes for a specific gateway
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
			Network: network,
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

	// Delete routes
	if len(routesToDelete) > 0 {
		log.Info("Attempting to delete routes", "gateway", gateway.String(), "count", len(routesToDelete))
		err := rm.BatchDeleteRoutes(routesToDelete, log)
		if err != nil {
			return fmt.Errorf("failed to delete routes for gateway %s: %w", gateway.String(), err)
		}
		log.Info("Successfully cleaned routes", "gateway", gateway.String(), "count", len(routesToDelete))
	}

	return nil
}
