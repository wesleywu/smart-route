//go:build darwin

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	LaunchdPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.smartroute.daemon</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>daemon</string>
		<string>--config</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/var/log/smartroute.out.log</string>
	<key>StandardErrorPath</key>
	<string>/var/log/smartroute.err.log</string>
	<key>WorkingDirectory</key>
	<string>/usr/local/bin</string>
</dict>
</plist>`
	
	LaunchdPlistPath = "/Library/LaunchDaemons/com.smartroute.plist"
)

type LaunchdService struct {
	execPath   string
	configPath string
}

func NewLaunchdService(execPath, configPath string) *LaunchdService {
	return &LaunchdService{
		execPath:   execPath,
		configPath: configPath,
	}
}

func (s *LaunchdService) Install() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("root privileges required to install launchd service")
	}

	plistContent := fmt.Sprintf(LaunchdPlistTemplate, s.execPath, s.configPath)
	
	if err := os.WriteFile(LaunchdPlistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}

	cmd := exec.Command("launchctl", "load", LaunchdPlistPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load launchd service: %w", err)
	}

	return nil
}

func (s *LaunchdService) Uninstall() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("root privileges required to uninstall launchd service")
	}

	cmd := exec.Command("launchctl", "unload", LaunchdPlistPath)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: failed to unload service: %v\n", err)
	}

	if err := os.Remove(LaunchdPlistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist file: %w", err)
	}

	return nil
}

func (s *LaunchdService) Start() error {
	cmd := exec.Command("launchctl", "start", "com.smartroute.daemon")
	return cmd.Run()
}

func (s *LaunchdService) Stop() error {
	cmd := exec.Command("launchctl", "stop", "com.smartroute.daemon")
	return cmd.Run()
}

func (s *LaunchdService) Status() (string, error) {
	cmd := exec.Command("launchctl", "list", "com.smartroute.daemon")
	output, err := cmd.Output()
	if err != nil {
		return "stopped", nil
	}

	if len(output) > 0 {
		return "running", nil
	}
	
	return "unknown", nil
}

func (s *LaunchdService) IsInstalled() bool {
	_, err := os.Stat(LaunchdPlistPath)
	return err == nil
}

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