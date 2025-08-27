//go:build darwin || linux

package routing

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// createRouteSocket creates a route socket for Unix systems
func (nm *NetworkMonitor) createRouteSocket() error {
	sock, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err != nil {
		return fmt.Errorf("failed to create route socket: %w", err)
	}
	nm.routeSocket = sock
	return nil
}

// closeRouteSocket closes the route socket for Unix systems
func (nm *NetworkMonitor) closeRouteSocket() {
	if nm.routeSocket > 0 {
		unix.Close(nm.routeSocket)
		nm.routeSocket = 0
	}
}

// readRouteSocket reads from the route socket for Unix systems
func (nm *NetworkMonitor) readRouteSocket(buffer []byte) (int, error) {
	return unix.Read(nm.routeSocket, buffer)
}

// isSocketError checks if an error is a socket error for Unix systems
func (nm *NetworkMonitor) isSocketError(err error) bool {
	// Only count serious socket errors, ignore temporary errors
	return err != unix.EAGAIN && 
	       err != unix.EWOULDBLOCK && 
	       err != unix.EINTR &&
	       err != unix.ECONNRESET &&
	       err != unix.EPIPE
}

// startPlatformMonitoring starts platform-specific monitoring for Unix systems
func (nm *NetworkMonitor) startPlatformMonitoring() {
	if err := nm.createRouteSocket(); err != nil {
		fmt.Printf("Failed to create route socket, enabling polling as fallback: %v\n", err)
		nm.pollEnabled = true
	} else {
		fmt.Printf("Route socket monitoring started (real-time events)\n")
		go nm.monitorRouteSocket()
	}
}
