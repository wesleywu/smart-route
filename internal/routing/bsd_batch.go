//go:build darwin || freebsd

package routing

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// High-performance batch operation using native system calls
func (rm *BSDRouteManager) batchOperationNative(routes []Route, action ActionType) error {
	if len(routes) == 0 {
		return nil
	}
	
	start := time.Now()
	
	// Use optimized batch processing for large route sets
	if len(routes) > 1000 {
		return rm.largeBatchOperation(routes, action)
	}
	
	// Use concurrent processing for smaller batches
	return rm.concurrentBatchOperation(routes, action, start)
}

func (rm *BSDRouteManager) largeBatchOperation(routes []Route, action ActionType) error {
	// For very large batches (3000+ routes), use a different strategy
	// Process in chunks to avoid overwhelming the kernel
	chunkSize := 500 // Process 500 routes at a time
	
	for i := 0; i < len(routes); i += chunkSize {
		end := i + chunkSize
		if end > len(routes) {
			end = len(routes)
		}
		
		chunk := routes[i:end]
		if err := rm.processChunkSequentially(chunk, action); err != nil {
			return fmt.Errorf("failed to process chunk %d-%d: %w", i, end-1, err)
		}
		
		// Small delay between chunks to be kernel-friendly
		time.Sleep(10 * time.Millisecond)
	}
	
	return nil
}

func (rm *BSDRouteManager) processChunkSequentially(routes []Route, action ActionType) error {
	for _, route := range routes {
		var err error
		switch action {
		case ActionAdd:
			err = rm.addRouteNative(route.Network, route.Gateway)
		case ActionDelete:
			err = rm.deleteRouteNative(route.Network, route.Gateway)
		}
		
		if err != nil {
			// For batch operations, we might want to continue on certain errors
			if routeErr, ok := err.(*RouteError); ok {
				if routeErr.Type == ErrInvalidRoute {
					// Skip invalid routes but continue
					continue
				}
			}
			return err
		}
	}
	
	return nil
}

func (rm *BSDRouteManager) concurrentBatchOperation(routes []Route, action ActionType, start time.Time) error {
	semaphore := make(chan struct{}, rm.concurrencyLimit)
	var wg sync.WaitGroup
	errChan := make(chan error, len(routes))
	
	for _, route := range routes {
		wg.Add(1)
		go func(r Route) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			var err error
			switch action {
			case ActionAdd:
				err = rm.addRouteNative(r.Network, r.Gateway)
			case ActionDelete:
				err = rm.deleteRouteNative(r.Network, r.Gateway)
			}
			
			if err != nil {
				errChan <- err
			}
		}(route)
	}
	
	wg.Wait()
	close(errChan)
	
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}
	
	// Update metrics
	duration := time.Since(start)
	rm.metrics.RecordOperation(duration, len(errors) == 0)
	
	if len(errors) > 0 {
		return fmt.Errorf("batch operation failed: %d/%d routes failed", len(errors), len(routes))
	}
	
	return nil
}

// Optimized single route operation with minimal overhead
func (rm *BSDRouteManager) fastAddRoute(network *net.IPNet, gateway net.IP) error {
	// Skip retry logic for batch operations to maximize speed
	return rm.addRouteNative(network, gateway)
}

func (rm *BSDRouteManager) fastDeleteRoute(network *net.IPNet, gateway net.IP) error {
	// Skip retry logic for batch operations to maximize speed  
	return rm.deleteRouteNative(network, gateway)
}

// Pre-allocate and reuse route message buffers for better performance
type routeMessagePool struct {
	pool sync.Pool
}

func newRouteMessagePool() *routeMessagePool {
	return &routeMessagePool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 256) // Pre-allocate common message size
			},
		},
	}
}

func (p *routeMessagePool) get() []byte {
	return p.pool.Get().([]byte)
}

func (p *routeMessagePool) put(buf []byte) {
	if len(buf) <= 512 { // Only reuse reasonably sized buffers
		p.pool.Put(buf)
	}
}

var globalMessagePool = newRouteMessagePool()