//go:build linux

package daemon

import "fmt"

// LinuxService is a placeholder for Linux systemd service
type LinuxService struct{}

// NewLinuxService creates a new LinuxService
func NewLinuxService(execPath, configPath string) *LinuxService {
	return &LinuxService{}
}

// Install is not implemented for Linux
func (s *LinuxService) Install() error {
	return fmt.Errorf("automatic installation not supported on Linux - use systemd manually")
}

// Uninstall is not implemented for Linux
func (s *LinuxService) Uninstall() error {
	return fmt.Errorf("automatic uninstallation not supported on Linux - use systemctl manually")
}

// Start is not implemented for Linux
func (s *LinuxService) Start() error {
	return fmt.Errorf("use systemctl start smartroute")
}

// Stop is not implemented for Linux
func (s *LinuxService) Stop() error {
	return fmt.Errorf("use systemctl stop smartroute")
}

// Status is not implemented for Linux
func (s *LinuxService) Status() (string, error) {
	return "unknown", fmt.Errorf("use systemctl status smartroute")
}

// IsInstalled is not implemented for Linux
func (s *LinuxService) IsInstalled() bool {
	return false
}

// NewPlatformService creates a platform-specific service
func NewPlatformService(execPath, configPath string) PlatformService {
	return NewLinuxService(execPath, configPath)
}
