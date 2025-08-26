package entities

import (
	"fmt"
	"net"

	"github.com/wesleywu/update-routes-native/internal/logger"
)

// Route represents a route entry in the system
type Route struct {
	Network   net.IPNet
	Gateway   net.IP
	Interface string
	Metric    int
}

// ActionType represents the type of action to be performed on a route
type ActionType int

// ActionType constants
const (
	ActionAdd ActionType = iota
	ActionDelete
)

// RouteError represents an error that occurred while managing routes
type RouteError struct {
	Type    ErrorType
	Network net.IPNet
	Gateway net.IP
	Cause   error
}

// RouteManager is an interface that defines the methods for managing routes
type RouteManager interface {
	AddRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error
	DeleteRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error
	BatchAddRoutes(routes []Route, log *logger.Logger) error
	BatchDeleteRoutes(routes []Route, log *logger.Logger) error
	GetDefaultGateway() (net.IP, string, error)
	ListRoutes() ([]Route, error)
	Close() error
}

// ErrorType represents the type of error that occurred
type ErrorType int

// ErrorType constants
const (
	ErrPermission ErrorType = iota
	ErrNetwork
	ErrInvalidRoute
	ErrSystemCall
	ErrTimeout
)

// String returns a string representation of the error type
func (e ErrorType) String() string {
	switch e {
	case ErrPermission:
		return "Permission"
	case ErrNetwork:
		return "Network"
	case ErrInvalidRoute:
		return "InvalidRoute"
	case ErrSystemCall:
		return "SystemCall"
	case ErrTimeout:
		return "Timeout"
	default:
		return "Unknown"
	}
}

// Error returns a string representation of the error
func (re *RouteError) Error() string {
	return fmt.Sprintf("route error [%s]: %v", re.Type.String(), re.Cause)
}

// IsRetryable checks if the error is retryable
func (re *RouteError) IsRetryable() bool {
	return re.Type == ErrNetwork || re.Type == ErrTimeout
}