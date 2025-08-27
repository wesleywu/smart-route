package entities

import (
	"fmt"
	"net"

	"github.com/wesleywu/update-routes-native/internal/logger"
)

// Route represents a system route table entry
type Route struct {
	Destination net.IPNet // Destination network
	Gateway     net.IP    // Gateway IP address
	Interface   string    // Network interface name
	Metric      int       // Route metric/priority
}

// RouteAction represents the type of operation to be performed on a route
type RouteAction int

// Route action constants
const (
	// RouteActionAdd adds a route to the system routing table
	RouteActionAdd RouteAction = iota
	// RouteActionDelete removes a route from the system routing table
	RouteActionDelete
)

// RouteOperationError represents an error that occurred during route operations
type RouteOperationError struct {
	ErrorType   RouteErrorType
	Destination net.IPNet // The network that caused the error
	Gateway     net.IP    // The gateway involved in the error
	Cause       error     // Underlying error
}

// RouteManager defines the interface for system routing table management
type RouteManager interface {
	// Single route operations
	AddRoute(destination *net.IPNet, gateway net.IP, logger *logger.Logger) error
	DeleteRoute(destination *net.IPNet, gateway net.IP, logger *logger.Logger) error
	
	// Batch route operations for performance
	BatchAddRoutes(routes []Route, logger *logger.Logger) error
	BatchDeleteRoutes(routes []Route, logger *logger.Logger) error
	
	// Gateway information retrieval
	// GetPhysicalGateway returns the underlying physical network gateway (for route management)
	GetPhysicalGateway() (gateway net.IP, interfaceName string, err error)
	// GetSystemDefaultRoute returns the current system default route (may include VPN)
	GetSystemDefaultRoute() (gateway net.IP, interfaceName string, err error)
	
	// Route table query operations
	ListSystemRoutes() (routes []Route, err error)
	
	// Resource management
	Close() error
}

// RouteErrorType represents the category of routing operation error
type RouteErrorType int

// Route error type constants
const (
	// RouteErrPermission indicates insufficient privileges for route operations
	RouteErrPermission RouteErrorType = iota
	// RouteErrNetwork indicates network-related errors
	RouteErrNetwork
	// RouteErrInvalidRoute indicates malformed or invalid route parameters
	RouteErrInvalidRoute
	// RouteErrSystemCall indicates system call failures
	RouteErrSystemCall
	// RouteErrTimeout indicates operation timeout
	RouteErrTimeout
	// RouteErrNotFound indicates route not found in system table
	RouteErrNotFound
)

// String returns a string representation of the route error type
func (e RouteErrorType) String() string {
	switch e {
	case RouteErrPermission:
		return "Permission"
	case RouteErrNetwork:
		return "Network"
	case RouteErrInvalidRoute:
		return "InvalidRoute"
	case RouteErrSystemCall:
		return "SystemCall"
	case RouteErrTimeout:
		return "Timeout"
	case RouteErrNotFound:
		return "NotFound"
	default:
		return "UnknownError"
	}
}

// Error implements the error interface for RouteOperationError
func (roe *RouteOperationError) Error() string {
	return fmt.Sprintf("route operation failed [%s] for %s via %s: %v", 
		roe.ErrorType.String(), 
		roe.Destination.String(),
		roe.Gateway.String(), 
		roe.Cause)
}

// IsRetryable returns true if the error condition might be temporary
func (roe *RouteOperationError) IsRetryable() bool {
	return roe.ErrorType == RouteErrNetwork || roe.ErrorType == RouteErrTimeout
}

// IsPermissionError returns true if the error is due to insufficient privileges
func (roe *RouteOperationError) IsPermissionError() bool {
	return roe.ErrorType == RouteErrPermission
}