//go:build windows

package daemon

import "fmt"

// WindowsService is a placeholder for Windows service
type WindowsService struct{}

// NewWindowsService creates a new WindowsService
func NewWindowsService(execPath, configPath string) *WindowsService {
	return &WindowsService{}
}

// Install is not implemented for Windows
func (s *WindowsService) Install() error {
	return fmt.Errorf("automatic installation not supported on Windows - use sc.exe manually")
}

// Uninstall is not implemented for Windows
func (s *WindowsService) Uninstall() error {
	return fmt.Errorf("automatic uninstallation not supported on Windows - use sc.exe manually")
}

// Start is not implemented for Windows
func (s *WindowsService) Start() error {
	return fmt.Errorf("use sc.exe start smartroute")
}

// Stop is not implemented for Windows
func (s *WindowsService) Stop() error {
	return fmt.Errorf("use sc.exe stop smartroute")
}

// Status is not implemented for Windows
func (s *WindowsService) Status() (string, error) {
	return "unknown", fmt.Errorf("use sc.exe query smartroute")
}

// IsInstalled is not implemented for Windows
func (s *WindowsService) IsInstalled() bool {
	return false
}

// NewPlatformService creates a platform-specific service
func NewPlatformService(execPath, configPath string) PlatformService {
	return NewWindowsService(execPath, configPath)
}
