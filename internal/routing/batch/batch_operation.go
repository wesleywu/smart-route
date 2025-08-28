package batch

import (
	"fmt"
	"net"
	"sync"

	"github.com/wesleywu/smart-route/internal/logger"
	"github.com/wesleywu/smart-route/internal/routing/entities"
)

// OperationFunc is a function that performs an operation on a route
type OperationFunc func(*net.IPNet, net.IP, *logger.Logger) error

// Process performs a batch operation on a list of routes with a concurrency limit
func Process(routes []*entities.Route, operationFunc OperationFunc, concurrencyLimit int, log *logger.Logger) error {
	semaphore := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup
	errChan := make(chan error, len(routes))

	for _, route := range routes {
		wg.Add(1)
		go func(r *entities.Route) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			err := operationFunc(&r.Destination, r.Gateway, log)

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

	if len(errors) > 0 {
		return fmt.Errorf("batch operation failed: %d errors", len(errors))
	}

	return nil
}
