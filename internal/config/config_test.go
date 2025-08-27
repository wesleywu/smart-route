package config

import (
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig("info", false, false, "configs/chnroute.txt", "configs/chndns.txt")
	
	if cfg.LogLevel != "info" {
		t.Errorf("Expected log level 'info', got '%s'", cfg.LogLevel)
	}
	
	if cfg.MonitorInterval != 2*time.Second {
		t.Errorf("Expected monitor interval 2s, got %v", cfg.MonitorInterval)
	}
	
	if cfg.ConcurrencyLimit != 50 {
		t.Errorf("Expected concurrency limit 50, got %d", cfg.ConcurrencyLimit)
	}
	
	if cfg.ChnRouteFile != "configs/chnroute.txt" {
		t.Errorf("Expected chn route file 'configs/chnroute.txt', got '%s'", cfg.ChnRouteFile)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		expectError bool
	}{
		{
			name:        "valid config",
			cfg:         NewConfig("info", false, false, "configs/chnroute.txt", "configs/chndns.txt"),
			expectError: false,
		},
		{
			name: "invalid log level",
			cfg: &Config{
				LogLevel:         "invalid",
				MonitorInterval:  2 * time.Second,
				RetryAttempts:    3,
				RouteTimeout:     30 * time.Second,
				ConcurrencyLimit: 50,
				BatchSize:        100,
			},
			expectError: true,
		},
		{
			name: "invalid monitor interval",
			cfg: &Config{
				LogLevel:         "info",
				MonitorInterval:  0,
				RetryAttempts:    3,
				RouteTimeout:     30 * time.Second,
				ConcurrencyLimit: 50,
				BatchSize:        100,
			},
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.expectError {
				t.Errorf("Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestLoadConfigBackwardCompatibility(t *testing.T) {
	// Test that LoadConfig still works for backward compatibility
	cfg, err := LoadConfig("any-path")
	if err != nil {
		t.Errorf("Expected no error for backward compatibility, got: %v", err)
	}
	
	if cfg.LogLevel != "info" {
		t.Errorf("Expected default log level, got: %s", cfg.LogLevel)
	}
	
	if cfg == nil {
		t.Error("Expected config, got nil")
	}
}