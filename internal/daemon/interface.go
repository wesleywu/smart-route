package daemon

// PlatformService defines the interface for platform-specific services
type PlatformService interface {
	Install() error
	Uninstall() error
	Start() error
	Stop() error
	Status() (string, error)
	IsInstalled() bool
}
