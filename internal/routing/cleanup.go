package routing

import (
	"fmt"
	"net"

	"github.com/wesleywu/update-routes-native/internal/config"
	"github.com/wesleywu/update-routes-native/internal/logger"
)

// CleanupManager handles complete route cleanup operations
type CleanupManager struct {
	rm              RouteManager
	logger          *logger.Logger
	managedNetworks []net.IPNet
}

// NewCleanupManager creates a new cleanup manager
func NewCleanupManager(rm RouteManager, logger *logger.Logger, chnRoutesFile, chnDNSFile string) (*CleanupManager, error) {
	cm := &CleanupManager{
		rm:     rm,
		logger: logger,
	}
	
	// Load managed networks during initialization
	managedNetworks, err := cm.loadAllManagedNetworks(chnRoutesFile, chnDNSFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load managed networks: %w", err)
	}
	
	cm.managedNetworks = managedNetworks
	logger.Info("initialized cleanup manager", "managed_networks_count", len(managedNetworks))
	
	return cm, nil
}

// CleanupAllManagedRoutes removes all routes for networks defined in Chinese DNS and route files
func (cm *CleanupManager) CleanupAllManagedRoutes() error {
	cm.logger.Info("starting complete route cleanup", "managed_networks_count", len(cm.managedNetworks))

	// Step 2: Get all current routes from system
	currentRoutes, err := cm.rm.ListRoutes()
	if err != nil {
		return fmt.Errorf("failed to list current routes: %w", err)
	}

	cm.logger.Info("retrieved system routes", "total_count", len(currentRoutes))

	// Step 3: Find all routes that match our managed networks
	routesToDelete := cm.findMatchingRoutes(cm.managedNetworks, currentRoutes)

	cm.logger.Info("found routes to cleanup", "count", len(routesToDelete))

	// Step 4: Delete all matching routes
	if len(routesToDelete) > 0 {
		return cm.deleteRoutes(routesToDelete)
	}

	cm.logger.Info("no routes to cleanup")
	return nil
}

// loadAllManagedNetworks loads all networks from Chinese route and DNS config files
func (cm *CleanupManager) loadAllManagedNetworks(chnRoutesFile, chnDNSFile string) ([]net.IPNet, error) {
	var allNetworks []net.IPNet

	// Load CHN routes
	chnRoutes, err := config.LoadChnRoutes(chnRoutesFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load CHN routes: %w", err)
	}

	// Add CHN route networks
	chnNetworks := chnRoutes.GetNetworks()
	for _, network := range chnNetworks {
		allNetworks = append(allNetworks, network)
	}

	cm.logger.Debug("loaded CHN route networks", "count", len(chnNetworks))

	// Load CHN DNS
	chnDNS, err := config.LoadChnDNS(chnDNSFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load CHN DNS: %w", err)
	}

	// Add CHN DNS networks (convert IPs to /32 or /128 networks)
	dnsIPs := chnDNS.GetIPs()
	for _, ip := range dnsIPs {
		var network net.IPNet
		if ip.To4() != nil {
			// IPv4 - use /32
			network = net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(32, 32),
			}
		} else {
			// IPv6 - use /128
			network = net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(128, 128),
			}
		}
		allNetworks = append(allNetworks, network)
	}

	cm.logger.Debug("loaded CHN DNS networks", "count", len(dnsIPs))

	return allNetworks, nil
}

// findMatchingRoutes finds all system routes that match our managed networks
func (cm *CleanupManager) findMatchingRoutes(managedNetworks []net.IPNet, systemRoutes []Route) []Route {
	var matchingRoutes []Route

	for _, managedNetwork := range managedNetworks {
		for _, systemRoute := range systemRoutes {
			if cm.networksEqual(managedNetwork, systemRoute.Network) {
				matchingRoutes = append(matchingRoutes, systemRoute)
				cm.logger.Debug("found matching route", 
					"network", systemRoute.Network.String(),
					"gateway", systemRoute.Gateway.String(),
					"interface", systemRoute.Interface)
			}
		}
	}

	return matchingRoutes
}

// networksEqual checks if two networks are equivalent
func (cm *CleanupManager) networksEqual(net1, net2 net.IPNet) bool {
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

// deleteRoutes deletes all specified routes
func (cm *CleanupManager) deleteRoutes(routes []Route) error {
	cm.logger.Info("deleting routes", "count", len(routes))

	// Log all routes that will be deleted
	for _, route := range routes {
		cm.logger.Info("deleting route", 
			"network", route.Network.String(),
			"gateway", route.Gateway.String(),
			"interface", route.Interface)
	}

	// Delete routes using batch operation
	err := cm.rm.BatchDeleteRoutes(routes, cm.logger)
	if err != nil {
		cm.logger.Error("failed to delete routes", "error", err)
		return fmt.Errorf("failed to delete routes: %w", err)
	}

	cm.logger.Info("successfully deleted all routes", "count", len(routes))
	return nil
}

// GetManagedNetworksCount returns the number of managed networks
func (cm *CleanupManager) GetManagedNetworksCount() int {
	return len(cm.managedNetworks)
}