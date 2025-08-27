package config

import (
	"fmt"
	"time"
)

type Config struct {
	// 基本配置 - 由命令行参数设置
	LogLevel   string
	SilentMode bool
	DaemonMode bool

	// 文件路径 - 由命令行参数设置
	ChnRouteFile string
	ChnDNSFile   string

	// 网络配置 - 硬编码默认值
	MonitorInterval time.Duration
	RetryAttempts   int
	RouteTimeout    time.Duration

	// 性能配置 - 硬编码默认值
	ConcurrencyLimit int
	BatchSize        int
}

// NewConfig creates a new config with command line parameters
func NewConfig(logLevel string, silentMode, daemonMode bool, chnRouteFile, chnDNSFile string) *Config {
	return &Config{
		LogLevel:         logLevel,
		SilentMode:       silentMode,
		DaemonMode:       daemonMode,
		ChnRouteFile:     chnRouteFile,
		ChnDNSFile:       chnDNSFile,
		
		// 硬编码的合理默认值
		MonitorInterval:  2 * time.Second,
		RetryAttempts:    3,
		RouteTimeout:     30 * time.Second,
		ConcurrencyLimit: 50,
		BatchSize:        100,
	}
}

// LoadConfig is deprecated - use NewConfig instead
// Kept for backward compatibility, always returns default config
func LoadConfig(path string) (*Config, error) {
	// Return hardcoded config with default file paths
	return NewConfig("info", false, false, "configs/chnroute.txt", "configs/chndns.txt"), nil
}

func (c *Config) Validate() error {
	if c.MonitorInterval < time.Second {
		return fmt.Errorf("monitor_interval must be at least 1 second")
	}

	if c.RetryAttempts < 1 {
		return fmt.Errorf("retry_attempts must be at least 1")
	}

	if c.RouteTimeout < time.Second {
		return fmt.Errorf("route_timeout must be at least 1 second")
	}

	if c.ConcurrencyLimit < 1 {
		return fmt.Errorf("concurrency_limit must be at least 1")
	}

	if c.BatchSize < 1 {
		return fmt.Errorf("batch_size must be at least 1")
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}

	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid log_level: %s", c.LogLevel)
	}

	return nil
}

// Save method removed - config is now managed via command line parameters