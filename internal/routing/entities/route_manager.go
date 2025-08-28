package entities

import (
	"net"

	"github.com/wesleywu/smart-route/internal/logger"
)

// RouteManager defines the interface for system routing table management
type RouteManager interface {
	// Single route operations
	AddRoute(destination *net.IPNet, gateway net.IP, logger *logger.Logger) error
	DeleteRoute(destination *net.IPNet, gateway net.IP, logger *logger.Logger) error

	// Batch route operations for performance
	BatchAddRoutes(routes []*Route, logger *logger.Logger) error
	BatchDeleteRoutes(routes []*Route, logger *logger.Logger) error

	// GetPhysicalGateway returns the underlying physical network gateway (for route management)
	GetPhysicalGateway() (gateway net.IP, interfaceName string, err error)
	// GetSystemDefaultRoute returns the current system default route (may include VPN)
	GetSystemDefaultRoute() (gateway net.IP, interfaceName string, err error)

	// Route table query operations
	ListSystemRoutes() (routes []*Route, err error)

	// Resource management
	Close() error
}


