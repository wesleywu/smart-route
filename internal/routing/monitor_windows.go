//go:build windows

package routing

import (
	"fmt"
)

// createRouteSocket is not supported on Windows
func (nm *NetworkMonitor) createRouteSocket() error {
	return fmt.Errorf("route socket not supported on Windows")
}

// closeRouteSocket is a no-op on Windows
func (nm *NetworkMonitor) closeRouteSocket() {
	// No-op on Windows
}

// readRouteSocket is not supported on Windows
func (nm *NetworkMonitor) readRouteSocket(buffer []byte) (int, error) {
	return 0, fmt.Errorf("route socket not supported on Windows")
}

// isSocketError always returns false on Windows
func (nm *NetworkMonitor) isSocketError(err error) bool {
	return false
}

// startPlatformMonitoring starts platform-specific monitoring for Windows
func (nm *NetworkMonitor) startPlatformMonitoring() {
	nm.logger.Debug("Platform not supported for route socket, enabling polling", "platform", "windows")
	nm.pollEnabled = true
}
