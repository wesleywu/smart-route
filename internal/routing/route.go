package routing

import (
	"sync"
	"time"

	"github.com/wesleywu/smart-route/internal/routing/entities"
	"github.com/wesleywu/smart-route/internal/routing/platform"
)

// NewPlatformRouteManager creates a platform-specific route manager instance
func NewPlatformRouteManager(concurrencyLimit int, maxRetries int) (entities.RouteManager, error) {
	return platform.NewPlatformRouteManager(concurrencyLimit, maxRetries)
}

// RouteManagerMetrics collects performance and operational statistics
type RouteManagerMetrics struct {
	totalOperations     int64         // Total number of route operations
	successfulOperations int64         // Number of successful operations
	failedOperations     int64         // Number of failed operations
	averageOperationTime time.Duration // Average time per operation
	networkChangeCount   int64         // Number of network changes detected
	lastUpdateTime       time.Time     // Timestamp of last metric update
	memoryUsageBytes     int64         // Memory usage in bytes
	metricsLock          sync.RWMutex  // Concurrent access protection
}

// NewRouteManagerMetrics creates a new metrics collection instance
func NewRouteManagerMetrics() *RouteManagerMetrics {
	return &RouteManagerMetrics{
		lastUpdateTime: time.Now(),
	}
}

// RecordRouteOperation records metrics for a completed route operation
func (rmm *RouteManagerMetrics) RecordRouteOperation(operationDuration time.Duration, successful bool) {
	rmm.metricsLock.Lock()
	defer rmm.metricsLock.Unlock()
	
	rmm.totalOperations++
	if successful {
		rmm.successfulOperations++
	} else {
		rmm.failedOperations++
	}
	
	// Update running average operation time
	if rmm.averageOperationTime == 0 {
		rmm.averageOperationTime = operationDuration
	} else {
		// Simple running average calculation
		rmm.averageOperationTime = (rmm.averageOperationTime + operationDuration) / 2
	}
	
	rmm.lastUpdateTime = time.Now()
}

// RecordNetworkChange increments the network change counter
func (rmm *RouteManagerMetrics) RecordNetworkChange() {
	rmm.metricsLock.Lock()
	defer rmm.metricsLock.Unlock()
	
	rmm.networkChangeCount++
	rmm.lastUpdateTime = time.Now()
}

// GetStatistics returns a snapshot of current metrics
func (rmm *RouteManagerMetrics) GetStatistics() (total, successful, failed int64, avgTime time.Duration, networkChanges int64) {
	rmm.metricsLock.RLock()
	defer rmm.metricsLock.RUnlock()
	
	return rmm.totalOperations, rmm.successfulOperations, rmm.failedOperations, 
		   rmm.averageOperationTime, rmm.networkChangeCount
}

// GetSuccessRate returns the success rate as a percentage (0-100)
func (rmm *RouteManagerMetrics) GetSuccessRate() float64 {
	rmm.metricsLock.RLock()
	defer rmm.metricsLock.RUnlock()
	
	if rmm.totalOperations == 0 {
		return 0.0
	}
	return float64(rmm.successfulOperations) / float64(rmm.totalOperations) * 100.0
}

