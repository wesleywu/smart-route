package metrics

import (
	"sync"
	"time"
)

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