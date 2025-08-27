//go:build darwin || freebsd

package platform

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/wesleywu/smart-route/internal/logger"
	"github.com/wesleywu/smart-route/internal/routing/entities"
)

// High-performance batch operation using native system calls
func (rm *BSDRouteManager) batchOperationNative(routes []entities.Route, action entities.RouteAction, log *logger.Logger) error {
	if len(routes) == 0 {
		return nil
	}
	
	start := time.Now()
	
	// Use optimized batch processing for large route sets
	if len(routes) > 1000 {
		return rm.largeBatchOperation(routes, action, log)
	}
	
	// Use concurrent processing for smaller batches
	return rm.concurrentBatchOperation(routes, action, start, log)
}

func (rm *BSDRouteManager) largeBatchOperation(routes []entities.Route, action entities.RouteAction, log *logger.Logger) error {
	// For very large batches (3000+ routes), use a different strategy
	// Process in chunks to avoid overwhelming the kernel
	chunkSize := 500 // Process 500 routes at a time
	
	for i := 0; i < len(routes); i += chunkSize {
		end := i + chunkSize
		if end > len(routes) {
			end = len(routes)
		}
		
		chunk := routes[i:end]
		if err := rm.processChunkSequentially(chunk, action, log); err != nil {
			return fmt.Errorf("failed to process chunk %d-%d: %w", i, end-1, err)
		}
		
		// Small delay between chunks to be kernel-friendly
		time.Sleep(10 * time.Millisecond)
	}
	
	return nil
}

func (rm *BSDRouteManager) processChunkSequentially(routes []entities.Route, action entities.RouteAction, log *logger.Logger) error {
	for _, route := range routes {
		var err error
		switch action {
		case entities.RouteActionAdd:
			err = rm.addRouteNative(&route.Destination, route.Gateway, log)
		case entities.RouteActionDelete:
			err = rm.deleteRouteNative(&route.Destination, route.Gateway, log)
		}
		
		if err != nil {
			// For batch operations, we might want to continue on certain errors
			if routeErr, ok := err.(*entities.RouteOperationError); ok {
				if routeErr.ErrorType == entities.RouteErrInvalidRoute {
					// Skip invalid routes but continue
					continue
				}
			}
			
			// Check if this is a "file exists" error for route addition
			if action == entities.RouteActionAdd && isRouteExistsError(err) {
				// Route already exists, this is acceptable for batch add operations
				continue
			}
			
			// Check if this is a "no such file or directory" error for route deletion
			if action == entities.RouteActionDelete && isRouteNotFoundError(err) {
				// Route doesn't exist, this is acceptable for batch delete operations
				continue
			}
			
			return err
		}
	}
	
	return nil
}

func (rm *BSDRouteManager) concurrentBatchOperation(routes []entities.Route, action entities.RouteAction, start time.Time, log *logger.Logger) error {
	semaphore := make(chan struct{}, rm.concurrencyLimit)
	var wg sync.WaitGroup
	errChan := make(chan error, len(routes))
	
	for _, route := range routes {
		wg.Add(1)
		go func(r entities.Route) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			var err error
			switch action {
			case entities.RouteActionAdd:
				err = rm.addRouteNative(&r.Destination, r.Gateway, log)
			case entities.RouteActionDelete:
				err = rm.deleteRouteNative(&r.Destination, r.Gateway, log)
			}
			
			if err != nil {
				// Apply the same error filtering logic as in sequential processing
				if routeErr, ok := err.(*entities.RouteOperationError); ok {
					if routeErr.ErrorType == entities.RouteErrInvalidRoute {
						// Skip invalid routes but continue
						return
					}
				}
				
				// Check if this is a "file exists" error for route addition
				if action == entities.RouteActionAdd && isRouteExistsError(err) {
					// Route already exists, this is acceptable for batch add operations
					return
				}
				
				// Check if this is a "no such file or directory" error for route deletion
				if action == entities.RouteActionDelete && isRouteNotFoundError(err) {
					// Route doesn't exist, this is acceptable for batch delete operations
					return
				}
				
				// Special handling for "no such process" error in delete operations
				if action == entities.RouteActionDelete && strings.Contains(err.Error(), "no such process") {
					log.Debug("Native batch delete failed with 'no such process', trying command line fallback", 
						"network", r.Destination.String(), "gateway", r.Gateway.String())
					
					// Try command line fallback
					fallbackErr := rm.deleteRouteCommand(&r.Destination, r.Gateway, log)
					if fallbackErr == nil {
						// Command line succeeded, don't report the native error
						return
					}
					
					// Both native and command line failed, report the original error
					log.Debug("Command line fallback also failed", 
						"network", r.Destination.String(), "fallback_error", fallbackErr)
				}
				
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
func (rm *BSDRouteManager) fastAddRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	// Skip retry logic for batch operations to maximize speed
	return rm.addRouteNative(network, gateway, log)
}

func (rm *BSDRouteManager) fastDeleteRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	// Skip retry logic for batch operations to maximize speed  
	return rm.deleteRouteNative(network, gateway, log)
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

// Helper functions to check for specific error conditions
func isRouteExistsError(err error) bool {
	if routeErr, ok := err.(*entities.RouteOperationError); ok {
		if routeErr.ErrorType == entities.RouteErrSystemCall && routeErr.Cause != nil {
			// Check for "file exists" error
			causeStr := fmt.Sprintf("%v", routeErr.Cause)
			return strings.Contains(causeStr, "file exists") ||
				   strings.Contains(causeStr, "EEXIST")
		}
	}
	// Also check the raw error message
	errStr := fmt.Sprintf("%v", err)
	return strings.Contains(errStr, "file exists") || strings.Contains(errStr, "EEXIST")
}

func isRouteNotFoundError(err error) bool {
	// Always check the complete error message first
	errStr := fmt.Sprintf("%v", err)
	if strings.Contains(errStr, "no such file or directory") ||
	   strings.Contains(errStr, "ENOENT") {
		return true
	}
	
	// NOTE: "no such process" (ESRCH) is NOT treated as "route not found"
	// on macOS because it often indicates a different type of system error
	// rather than the route simply not existing
	
	// Also check structured RouteError
	if routeErr, ok := err.(*entities.RouteOperationError); ok {
		if routeErr.ErrorType == entities.RouteErrSystemCall && routeErr.Cause != nil {
			causeStr := fmt.Sprintf("%v", routeErr.Cause)
			return strings.Contains(causeStr, "no such file or directory") ||
				   strings.Contains(causeStr, "ENOENT")
			// NOTE: Removed "no such process" and "ESRCH" from here too
		}
	}
	
	return false
}