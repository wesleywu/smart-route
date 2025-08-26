//go:build darwin

package daemon

// NewPlatformService creates a platform-specific service
func NewPlatformService(execPath, configPath string) PlatformService {
	return NewLaunchdService(execPath, configPath)
}
