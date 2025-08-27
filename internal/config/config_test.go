package config

import (
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()
	
	if cfg.MonitorInterval != 2*time.Second {
		t.Errorf("Expected monitor interval 2s, got %v", cfg.MonitorInterval)
	}
	
	if cfg.ConcurrencyLimit != 50 {
		t.Errorf("Expected concurrency limit 50, got %d", cfg.ConcurrencyLimit)
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
			cfg:         NewConfig(),
			expectError: false,
		},
		{
			name: "invalid monitor interval",
			cfg: &Config{
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
	
	if cfg == nil {
		t.Error("Expected config, got nil")
	}
}