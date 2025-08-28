package config

import (
	"time"
)

// Config represents the configuration for the smart route manager
type Config struct {
	// 网络配置 - 硬编码默认值
	MonitorInterval time.Duration
	RetryAttempts   int
	RouteTimeout    time.Duration

	// 性能配置 - 硬编码默认值
	ConcurrencyLimit int
	BatchSize        int
}

// NewConfig creates a new config with default values
func NewConfig() *Config {
	return &Config{
		// 硬编码的合理默认值
		MonitorInterval:  2 * time.Second,
		RetryAttempts:    3,
		RouteTimeout:     30 * time.Second,
		ConcurrencyLimit: 50,
		BatchSize:        100,
	}
}
