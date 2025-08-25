package routing

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/wesleywu/update-routes-native/internal/logger"
)

type RouteManager interface {
	AddRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error
	DeleteRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error
	BatchAddRoutes(routes []Route, log *logger.Logger) error
	BatchDeleteRoutes(routes []Route, log *logger.Logger) error
	GetDefaultGateway() (net.IP, string, error)
	ListRoutes() ([]Route, error)
	FlushRoutes(gateway net.IP) error
	Close() error
}

type Route struct {
	Network   net.IPNet  // Changed from pointer to value
	Gateway   net.IP
	Interface string
	Metric    int
}

type RouteError struct {
	Type    ErrorType
	Network net.IPNet  // Changed from pointer to value
	Gateway net.IP
	Cause   error
}

type ErrorType int

const (
	ErrPermission ErrorType = iota
	ErrNetwork
	ErrInvalidRoute
	ErrSystemCall
	ErrTimeout
)

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

func (re *RouteError) Error() string {
	return fmt.Sprintf("route error [%s]: %v", re.Type.String(), re.Cause)
}

func (re *RouteError) IsRetryable() bool {
	return re.Type == ErrNetwork || re.Type == ErrTimeout
}

type WorkerPool struct {
	workers int
	jobs    chan RouteJob
	results chan RouteResult
	wg      sync.WaitGroup
}

type RouteJob struct {
	Network *net.IPNet
	Gateway net.IP
	Action  ActionType
}

type RouteResult struct {
	Job   RouteJob
	Error error
}

type ActionType int

const (
	ActionAdd ActionType = iota
	ActionDelete
)

func NewWorkerPool(workers int) *WorkerPool {
	return &WorkerPool{
		workers: workers,
		jobs:    make(chan RouteJob, workers*2),
		results: make(chan RouteResult, workers*2),
	}
}

func (wp *WorkerPool) Start(rm RouteManager, log *logger.Logger) {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(rm, log)
	}
}

func (wp *WorkerPool) Stop() {
	close(wp.jobs)
	wp.wg.Wait()
	close(wp.results)
}

func (wp *WorkerPool) AddJob(job RouteJob) {
	wp.jobs <- job
}

func (wp *WorkerPool) Results() <-chan RouteResult {
	return wp.results
}

func (wp *WorkerPool) worker(rm RouteManager, log *logger.Logger) {
	defer wp.wg.Done()
	
	for job := range wp.jobs {
		var err error
		
		switch job.Action {
		case ActionAdd:
			err = rm.AddRoute(job.Network, job.Gateway, log)
		case ActionDelete:
			err = rm.DeleteRoute(job.Network, job.Gateway, log)
		}
		
		result := RouteResult{
			Job:   job,
			Error: err,
		}
		
		wp.results <- result
	}
}

func NewRouteManager(concurrencyLimit int, maxRetries int) (RouteManager, error) {
	return newPlatformRouteManager(concurrencyLimit, maxRetries)
}

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

func NewMetrics() *Metrics {
	return &Metrics{
		LastUpdate: time.Now(),
	}
}

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

func (m *Metrics) RecordNetworkChange() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.NetworkChanges++
}

func (m *Metrics) GetStats() (int64, int64, int64, time.Duration, int64) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	return m.RouteOperations, m.SuccessfulOps, m.FailedOps, m.AverageOpTime, m.NetworkChanges
}