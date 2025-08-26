package routing

import (
	"net"
	"sync"
	"time"

	"github.com/wesleywu/update-routes-native/internal/logger"
	"github.com/wesleywu/update-routes-native/internal/routing/entities"
	"github.com/wesleywu/update-routes-native/internal/routing/platform"
)

// WorkerPool is a pool of workers that can be used to manage routes
type WorkerPool struct {
	workers int
	jobs    chan RouteJob
	results chan RouteResult
	wg      sync.WaitGroup
}

// RouteJob represents a job to be performed on a route
type RouteJob struct {
	Network *net.IPNet
	Gateway net.IP
	Action  entities.ActionType
}

// RouteResult represents the result of a route job
type RouteResult struct {
	Job   RouteJob
	Error error
}


// NewWorkerPool creates a new worker pool
func NewWorkerPool(workers int) *WorkerPool {
	return &WorkerPool{
		workers: workers,
		jobs:    make(chan RouteJob, workers*2),
		results: make(chan RouteResult, workers*2),
	}
}

// Start starts the worker pool
func (wp *WorkerPool) Start(rm entities.RouteManager, log *logger.Logger) {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(rm, log)
	}
}

// Stop stops the worker pool
func (wp *WorkerPool) Stop() {
	close(wp.jobs)
	wp.wg.Wait()
	close(wp.results)
}

// AddJob adds a job to the worker pool
func (wp *WorkerPool) AddJob(job RouteJob) {
	wp.jobs <- job
}

// Results returns the results channel
func (wp *WorkerPool) Results() <-chan RouteResult {
	return wp.results
}

// worker is a worker function that performs the actual route management
func (wp *WorkerPool) worker(rm entities.RouteManager, log *logger.Logger) {
	defer wp.wg.Done()
	
	for job := range wp.jobs {
		var err error
		
		switch job.Action {
		case entities.ActionAdd:
			err = rm.AddRoute(job.Network, job.Gateway, log)
		case entities.ActionDelete:
			err = rm.DeleteRoute(job.Network, job.Gateway, log)
		}
		
		result := RouteResult{
			Job:   job,
			Error: err,
		}
		
		wp.results <- result
	}
}

// NewRouteManager creates a new route manager
func NewRouteManager(concurrencyLimit int, maxRetries int) (entities.RouteManager, error) {
	return platform.NewPlatformRouteManager(concurrencyLimit, maxRetries)
}

// Metrics represents the metrics for the route manager
type Metrics struct {
	RouteOperations int64
	SuccessfulOps   int64
	FailedOps       int64
	AverageOpTime   time.Duration
	NetworkChanges  int64
	LastUpdate      time.Time
	MemoryUsage     int64
	mutex           sync.RWMutex
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		LastUpdate: time.Now(),
	}
}

// RecordOperation records the operation metrics
func (m *Metrics) RecordOperation(duration time.Duration, success bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.RouteOperations++
	if success {
		m.SuccessfulOps++
	} else {
		m.FailedOps++
	}
	
	if m.AverageOpTime == 0 {
		m.AverageOpTime = duration
	} else {
		m.AverageOpTime = (m.AverageOpTime + duration) / 2
	}
	
	m.LastUpdate = time.Now()
}

// RecordNetworkChange records the network change metrics
func (m *Metrics) RecordNetworkChange() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.NetworkChanges++
}

// GetStats returns the metrics statistics
func (m *Metrics) GetStats() (int64, int64, int64, time.Duration, int64) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	return m.RouteOperations, m.SuccessfulOps, m.FailedOps, m.AverageOpTime, m.NetworkChanges
}

// routesMatch checks if two networks are equivalent
func routesMatch(net1, net2 net.IPNet) bool {
	return net1.IP.Equal(net2.IP) && 
		   len(net1.Mask) == len(net2.Mask) &&
		   net1.Mask.String() == net2.Mask.String()
}