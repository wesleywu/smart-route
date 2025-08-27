package routing

import (
	"net"
	"testing"
	"time"

	"github.com/wesleywu/smart-route/internal/routing/entities"
)

func TestRouteOperationError(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	gateway := net.ParseIP("192.168.1.1")
	
	err := &entities.RouteOperationError{
		ErrorType:   entities.RouteErrPermission,
		Destination: *network,  // Dereference pointer to get value
		Gateway:     gateway,
		Cause:       nil,
	}
	
	if err.ErrorType != entities.RouteErrPermission {
		t.Errorf("Expected error type %v, got %v", entities.RouteErrPermission, err.ErrorType)
	}
	
	if err.IsRetryable() {
		t.Error("Permission error should not be retryable")
	}
	
	// Test retryable error
	networkErr := &entities.RouteOperationError{
		ErrorType:   entities.RouteErrNetwork,
		Destination: *network,  // Dereference pointer to get value
		Gateway:     gateway,
		Cause:       nil,
	}
	
	if !networkErr.IsRetryable() {
		t.Error("Network error should be retryable")
	}
}

func TestRouteErrorTypeString(t *testing.T) {
	tests := []struct {
		errorType entities.RouteErrorType
		expected  string
	}{
		{entities.RouteErrPermission, "Permission"},
		{entities.RouteErrNetwork, "Network"},
		{entities.RouteErrInvalidRoute, "InvalidRoute"},
		{entities.RouteErrSystemCall, "SystemCall"},
		{entities.RouteErrTimeout, "Timeout"},
		{entities.RouteErrNotFound, "NotFound"},
	}
	
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.errorType.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.errorType.String())
			}
		})
	}
}

func TestNewRouteWorkerPool(t *testing.T) {
	pool := NewRouteWorkerPool(5)
	
	if pool.workerCount != 5 {
		t.Errorf("Expected 5 workers, got %d", pool.workerCount)
	}
	
	if pool.jobChannel == nil {
		t.Error("Job channel should be initialized")
	}
	
	if pool.resultChannel == nil {
		t.Error("Result channel should be initialized")
	}
}

func TestRouteManagerMetrics(t *testing.T) {
	metrics := NewRouteManagerMetrics()
	
	// Test initial state
	ops, success, failed, avgTime, changes := metrics.GetStatistics()
	if ops != 0 || success != 0 || failed != 0 || changes != 0 {
		t.Error("Initial metrics should be zero")
	}
	
	// Record successful operation
	metrics.RecordRouteOperation(100*time.Millisecond, true)
	ops, success, failed, avgTime, changes = metrics.GetStatistics()
	
	if ops != 1 {
		t.Errorf("Expected 1 operation, got %d", ops)
	}
	
	if success != 1 {
		t.Errorf("Expected 1 success, got %d", success)
	}
	
	if failed != 0 {
		t.Errorf("Expected 0 failures, got %d", failed)
	}
	
	if avgTime != 100*time.Millisecond {
		t.Errorf("Expected 100ms avg time, got %v", avgTime)
	}
	
	// Record failed operation
	metrics.RecordRouteOperation(200*time.Millisecond, false)
	ops, success, failed, avgTime, changes = metrics.GetStatistics()
	
	if ops != 2 {
		t.Errorf("Expected 2 operations, got %d", ops)
	}
	
	if failed != 1 {
		t.Errorf("Expected 1 failure, got %d", failed)
	}
	
	// Record network change
	metrics.RecordNetworkChange()
	ops, success, failed, avgTime, changes = metrics.GetStatistics()
	
	if changes != 1 {
		t.Errorf("Expected 1 network change, got %d", changes)
	}
}

func TestRoute(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	gateway := net.ParseIP("192.168.1.1")
	
	route := entities.Route{
		Destination: *network,  // Dereference pointer to get value
		Gateway:     gateway,
		Interface:   "eth0",
		Metric:      1,
	}
	
	if !route.Destination.IP.Equal(net.ParseIP("192.168.1.0")) {
		t.Error("Route destination IP mismatch")
	}
	
	if !route.Gateway.Equal(gateway) {
		t.Error("Route gateway mismatch")
	}
	
	if route.Interface != "eth0" {
		t.Errorf("Expected interface eth0, got %s", route.Interface)
	}
	
	if route.Metric != 1 {
		t.Errorf("Expected metric 1, got %d", route.Metric)
	}
}

func TestRouteOperation(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	gateway := net.ParseIP("192.168.1.1")
	
	operation := RouteOperation{
		Destination: network,
		Gateway:     gateway,
		Action:      entities.RouteActionAdd,
	}
	
	if operation.Action != entities.RouteActionAdd {
		t.Errorf("Expected RouteActionAdd, got %v", operation.Action)
	}
	
	if !operation.Gateway.Equal(gateway) {
		t.Error("Operation gateway mismatch")
	}
}