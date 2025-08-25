package routing

import (
	"net"
	"testing"
	"time"
)

func TestRouteError(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	gateway := net.ParseIP("192.168.1.1")
	
	err := &RouteError{
		Type:    ErrPermission,
		Network: *network,  // Dereference pointer to get value
		Gateway: gateway,
		Cause:   nil,
	}
	
	if err.Type != ErrPermission {
		t.Errorf("Expected error type %v, got %v", ErrPermission, err.Type)
	}
	
	if err.IsRetryable() {
		t.Error("Permission error should not be retryable")
	}
	
	// Test retryable error
	networkErr := &RouteError{
		Type:    ErrNetwork,
		Network: *network,  // Dereference pointer to get value
		Gateway: gateway,
		Cause:   nil,
	}
	
	if !networkErr.IsRetryable() {
		t.Error("Network error should be retryable")
	}
}

func TestErrorTypeString(t *testing.T) {
	tests := []struct {
		errorType ErrorType
		expected  string
	}{
		{ErrPermission, "Permission"},
		{ErrNetwork, "Network"},
		{ErrInvalidRoute, "InvalidRoute"},
		{ErrSystemCall, "SystemCall"},
		{ErrTimeout, "Timeout"},
	}
	
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.errorType.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.errorType.String())
			}
		})
	}
}

func TestNewWorkerPool(t *testing.T) {
	pool := NewWorkerPool(5)
	
	if pool.workers != 5 {
		t.Errorf("Expected 5 workers, got %d", pool.workers)
	}
	
	if pool.jobs == nil {
		t.Error("Jobs channel should be initialized")
	}
	
	if pool.results == nil {
		t.Error("Results channel should be initialized")
	}
}

func TestMetrics(t *testing.T) {
	metrics := NewMetrics()
	
	// Test initial state
	ops, success, failed, avgTime, changes := metrics.GetStats()
	if ops != 0 || success != 0 || failed != 0 || changes != 0 {
		t.Error("Initial metrics should be zero")
	}
	
	// Record successful operation
	metrics.RecordOperation(100*time.Millisecond, true)
	ops, success, failed, avgTime, changes = metrics.GetStats()
	
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
	metrics.RecordOperation(200*time.Millisecond, false)
	ops, success, failed, avgTime, changes = metrics.GetStats()
	
	if ops != 2 {
		t.Errorf("Expected 2 operations, got %d", ops)
	}
	
	if failed != 1 {
		t.Errorf("Expected 1 failure, got %d", failed)
	}
	
	// Record network change
	metrics.RecordNetworkChange()
	ops, success, failed, avgTime, changes = metrics.GetStats()
	
	if changes != 1 {
		t.Errorf("Expected 1 network change, got %d", changes)
	}
}

func TestRoute(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	gateway := net.ParseIP("192.168.1.1")
	
	route := Route{
		Network:   *network,  // Dereference pointer to get value
		Gateway:   gateway,
		Interface: "eth0",
		Metric:    1,
	}
	
	if !route.Network.IP.Equal(net.ParseIP("192.168.1.0")) {
		t.Error("Route network IP mismatch")
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

func TestRouteJob(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	gateway := net.ParseIP("192.168.1.1")
	
	job := RouteJob{
		Network: network,
		Gateway: gateway,
		Action:  ActionAdd,
	}
	
	if job.Action != ActionAdd {
		t.Errorf("Expected ActionAdd, got %v", job.Action)
	}
	
	if !job.Gateway.Equal(gateway) {
		t.Error("Job gateway mismatch")
	}
}