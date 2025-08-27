package config

import (
	"os"
	"testing"
	"time"
)

func TestNewDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig()
	
	if cfg.LogLevel != "info" {
		t.Errorf("Expected log level 'info', got '%s'", cfg.LogLevel)
	}
	
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
			cfg:         NewDefaultConfig(),
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

func TestLoadConfig(t *testing.T) {
	// Test loading non-existent file (should return default config)
	cfg, err := LoadConfig("non-existent.json")
	if err != nil {
		t.Errorf("Expected no error for non-existent file, got: %v", err)
	}
	
	if cfg.LogLevel != "info" {
		t.Errorf("Expected default log level, got: %s", cfg.LogLevel)
	}
	
	// Test loading empty path (should return default config)
	cfg, err = LoadConfig("")
	if err != nil {
		t.Errorf("Expected no error for empty path, got: %v", err)
	}
	
	if cfg == nil {
		t.Error("Expected config, got nil")
	}
}

func TestConfigSave(t *testing.T) {
	cfg := NewDefaultConfig()
	tempFile := "/tmp/test-config.json"
	
	defer os.Remove(tempFile)
	
	err := cfg.Save(tempFile)
	if err != nil {
		t.Errorf("Failed to save config: %v", err)
	}
	
	// Verify file exists
	if _, err := os.Stat(tempFile); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
	
	// Load and verify
	loadedCfg, err := LoadConfig(tempFile)
	if err != nil {
		t.Errorf("Failed to load saved config: %v", err)
	}
	
	if loadedCfg.LogLevel != cfg.LogLevel {
		t.Errorf("Config mismatch after save/load")
	}
}