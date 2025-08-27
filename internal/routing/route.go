package routing

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/wesleywu/smart-route/internal/logger"
	"github.com/wesleywu/smart-route/internal/routing/entities"
	"github.com/wesleywu/smart-route/internal/routing/platform"
)

// RouteWorkerPool manages concurrent route operations using a worker pool pattern
type RouteWorkerPool struct {
	workerCount    int
	jobChannel     chan RouteOperation
	resultChannel  chan RouteOperationResult
	workerGroup    sync.WaitGroup
}

// RouteOperation represents a route operation to be executed by workers
type RouteOperation struct {
	Destination *net.IPNet      // Target network
	Gateway     net.IP          // Gateway for the route
	Action      entities.RouteAction // Add or delete operation
}

// RouteOperationResult represents the outcome of a route operation
type RouteOperationResult struct {
	Operation RouteOperation // The executed operation
	Error     error         // Error if operation failed, nil if successful
}


// NewRouteWorkerPool creates a new concurrent route operation worker pool
func NewRouteWorkerPool(workerCount int) *RouteWorkerPool {
	bufferSize := workerCount * 2 // Buffer size for channels
	return &RouteWorkerPool{
		workerCount:   workerCount,
		jobChannel:    make(chan RouteOperation, bufferSize),
		resultChannel: make(chan RouteOperationResult, bufferSize),
	}
}

// Start initializes and starts all workers in the pool
func (rwp *RouteWorkerPool) Start(routeManager entities.RouteManager, logger *logger.Logger) {
	for i := 0; i < rwp.workerCount; i++ {
		rwp.workerGroup.Add(1)
		go rwp.routeWorker(routeManager, logger)
	}
}

// Stop gracefully shuts down the worker pool
func (rwp *RouteWorkerPool) Stop() {
	close(rwp.jobChannel)        // Signal workers to stop accepting new jobs
	rwp.workerGroup.Wait()       // Wait for all workers to complete
	close(rwp.resultChannel)     // Close results channel after all workers finish
}

// SubmitOperation adds a route operation to the worker pool queue
func (rwp *RouteWorkerPool) SubmitOperation(operation RouteOperation) {
	rwp.jobChannel <- operation
}

// GetResultChannel returns the read-only channel for operation results
func (rwp *RouteWorkerPool) GetResultChannel() <-chan RouteOperationResult {
	return rwp.resultChannel
}

// routeWorker is the worker function that executes route operations
func (rwp *RouteWorkerPool) routeWorker(routeManager entities.RouteManager, logger *logger.Logger) {
	defer rwp.workerGroup.Done()
	
	for operation := range rwp.jobChannel {
		var err error
		
		// Execute the route operation based on action type
		switch operation.Action {
		case entities.RouteActionAdd:
			err = routeManager.AddRoute(operation.Destination, operation.Gateway, logger)
		case entities.RouteActionDelete:
			err = routeManager.DeleteRoute(operation.Destination, operation.Gateway, logger)
		default:
			err = fmt.Errorf("unknown route action: %v", operation.Action)
		}
		
		// Send result back through results channel
		result := RouteOperationResult{
			Operation: operation,
			Error:     err,
		}
		
		rwp.resultChannel <- result
	}
}

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

// networksEqual checks if two IP networks are equivalent
func networksEqual(network1, network2 net.IPNet) bool {
	// Compare network addresses and subnet masks
	return network1.IP.Equal(network2.IP) && 
		   len(network1.Mask) == len(network2.Mask) &&
		   network1.Mask.String() == network2.Mask.String()
}