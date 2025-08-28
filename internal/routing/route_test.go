package routing

import (
	"net"
	"testing"
	"time"

	"github.com/wesleywu/smart-route/internal/routing/types"
)

func TestRouteOperationError(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	gateway := net.ParseIP("192.168.1.1")
	
	err := &types.RouteOperationError{
		ErrorType:   types.RouteErrPermission,
		Destination: *network,  // Dereference pointer to get value
		Gateway:     gateway,
		Cause:       nil,
	}
	
	if err.ErrorType != types.RouteErrPermission {
		t.Errorf("Expected error type %v, got %v", types.RouteErrPermission, err.ErrorType)
	}
	
	if err.IsRetryable() {
		t.Error("Permission error should not be retryable")
	}
	
	// Test retryable error
	networkErr := &types.RouteOperationError{
		ErrorType:   types.RouteErrNetwork,
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
		errorType types.RouteErrorType
		expected  string
	}{
		{types.RouteErrPermission, "Permission"},
		{types.RouteErrNetwork, "Network"},
		{types.RouteErrInvalidRoute, "InvalidRoute"},
		{types.RouteErrSystemCall, "SystemCall"},
		{types.RouteErrTimeout, "Timeout"},
		{types.RouteErrNotFound, "NotFound"},
	}
	
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.errorType.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.errorType.String())
			}
		})
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
	
	route := types.Route{
		Destination: *network,  // Dereference pointer to get value
		Gateway:     gateway,
		Metric:      1,
	}
	
	if !route.Destination.IP.Equal(net.ParseIP("192.168.1.0")) {
		t.Error("Route destination IP mismatch")
	}
	
	if !route.Gateway.Equal(gateway) {
		t.Error("Route gateway mismatch")
	}
		
	if route.Metric != 1 {
		t.Errorf("Expected metric 1, got %d", route.Metric)
	}
}

