package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	// 基本配置
	LogLevel   string `json:"log_level"`
	SilentMode bool   `json:"silent_mode"`
	DaemonMode bool   `json:"daemon_mode"`

	// 文件路径
	ChnRouteFile string `json:"chn_route_file"`
	ChnDNSFile   string `json:"chn_dns_file"`

	// 网络配置
	MonitorInterval time.Duration `json:"monitor_interval"`
	RetryAttempts   int           `json:"retry_attempts"`
	RouteTimeout    time.Duration `json:"route_timeout"`

	// 性能配置
	ConcurrencyLimit int `json:"concurrency_limit"`
	BatchSize        int `json:"batch_size"`
}

func NewDefaultConfig() *Config {
	return &Config{
		LogLevel:         "info",
		SilentMode:       false,
		DaemonMode:       false,
		ChnRouteFile:     "configs/chnroute.txt",
		ChnDNSFile:       "configs/chdns.txt",
		MonitorInterval:  5 * time.Second,
		RetryAttempts:    3,
		RouteTimeout:     30 * time.Second,
		ConcurrencyLimit: 50,
		BatchSize:        100,
	}
}

func LoadConfig(path string) (*Config, error) {
	config := NewDefaultConfig()

	if path == "" {
		return config, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
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

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}