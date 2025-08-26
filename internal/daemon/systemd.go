//go:build linux

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	// SystemdServiceTemplate is the template for the systemd service file
	SystemdServiceTemplate = `[Unit]
Description=Smart Route Manager
After=network.target
Wants=network.target

[Service]
Type=simple
ExecStart=%s daemon --config %s
Restart=always
RestartSec=5
User=root
Group=root
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target`
	// SystemdServicePath is the path to the systemd service file
	SystemdServicePath = "/etc/systemd/system/smartroute.service"
)

// SystemdService is a systemd service for Linux
type SystemdService struct {
	execPath   string
	configPath string
}

// NewSystemdService creates a new SystemdService
func NewSystemdService(execPath, configPath string) *SystemdService {
	return &SystemdService{
		execPath:   execPath,
		configPath: configPath,
	}
}

// Install installs the systemd service
func (s *SystemdService) Install() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("root privileges required to install systemd service")
	}

	serviceContent := fmt.Sprintf(SystemdServiceTemplate, s.execPath, s.configPath)
	
	if err := os.WriteFile(SystemdServicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	cmd := exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	cmd = exec.Command("systemctl", "enable", "smartroute")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	return nil
}

// Uninstall uninstalls the systemd service
func (s *SystemdService) Uninstall() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("root privileges required to uninstall systemd service")
	}

	cmd := exec.Command("systemctl", "disable", "smartroute")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: failed to disable service: %v\n", err)
	}

	cmd = exec.Command("systemctl", "stop", "smartroute")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: failed to stop service: %v\n", err)
	}

	if err := os.Remove(SystemdServicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	cmd = exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	return nil
}

// Start starts the systemd service
func (s *SystemdService) Start() error {
	cmd := exec.Command("systemctl", "start", "smartroute")
	return cmd.Run()
}

// Stop stops the systemd service
func (s *SystemdService) Stop() error {
	cmd := exec.Command("systemctl", "stop", "smartroute")
	return cmd.Run()
}

// Status returns the status of the systemd service
func (s *SystemdService) Status() (string, error) {
	cmd := exec.Command("systemctl", "is-active", "smartroute")
	output, err := cmd.Output()
	if err != nil {
		return "stopped", nil
	}

	status := string(output)
	status = status[:len(status)-1] // Remove newline
	return status, nil
}

// IsInstalled checks if the systemd service is installed
func (s *SystemdService) IsInstalled() bool {
	_, err := os.Stat(SystemdServicePath)
	return err == nil
}

// InstallBinary installs the binary
func InstallBinary(sourcePath, targetDir string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("root privileges required to install binary")
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	targetPath := filepath.Join(targetDir, "smartroute")
	
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer source.Close()

	target, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer target.Close()

	if _, err := target.ReadFrom(source); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	if err := os.Chmod(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	return nil
}