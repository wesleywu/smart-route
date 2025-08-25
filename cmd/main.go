package main

import (
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/wesleywu/update-routes-native/internal/config"
	"github.com/wesleywu/update-routes-native/internal/daemon"
	"github.com/wesleywu/update-routes-native/internal/logger"
	"github.com/wesleywu/update-routes-native/internal/network"
	"github.com/wesleywu/update-routes-native/internal/routing"
)

var (
	version = "1.0.0"
	
	configFile string
	silentMode bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "smartroute",
		Short: "Smart Route Manager for VPN split tunneling",
		Long:  `A high-performance route management tool for Chinese IP addresses and DNS servers split tunneling.`,
		Run:   runOnce,
	}

	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as daemon service",
		Long:  `Run the smart route manager as a background daemon service with network monitoring.`,
		Run:   runDaemon,
	}

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install as system service",
		Long:  `Install the smart route manager as a system service (launchd on macOS, systemd on Linux).`,
		Run:   installService,
	}

	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall system service",
		Long:  `Uninstall the smart route manager system service.`,
		Run:   uninstallService,
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show service status",
		Long:  `Show the current status of the smart route manager service.`,
		Run:   showStatus,
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  `Show version, build information and system details.`,
		Run:   showVersion,
	}

	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test configuration and connectivity",
		Long:  `Test configuration files, network connectivity and routing capabilities.`,
		Run:   testConfiguration,
	}

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Configuration file path")
	rootCmd.PersistentFlags().BoolVarP(&silentMode, "silent", "s", false, "Silent mode (no output)")

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(testCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runOnce(cmd *cobra.Command, args []string) {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if silentMode {
		cfg.SilentMode = true
	}

	log := logger.New(cfg)
	log.Info("Starting one-time route setup", "version", version)

	_, err = daemon.NewServiceManager(cfg, log)
	if err != nil {
		log.Error("Failed to create service manager", "error", err)
		os.Exit(1)
	}

	gateway, iface, err := network.GetDefaultGateway()
	if err != nil {
		log.Error("Failed to get default gateway", "error", err)
		os.Exit(1)
	}

	log.Info("Default gateway detected", "gateway", gateway.String(), "interface", iface)

	rm, err := routing.NewRouteManager(cfg.ConcurrencyLimit, cfg.RetryAttempts)
	if err != nil {
		log.Error("Failed to create route manager", "error", err)
		os.Exit(1)
	}
	defer rm.Close()

	chnRoutes, err := config.LoadChnRoutes(cfg.ChnRouteFile)
	if err != nil {
		log.Error("Failed to load Chinese routes", "error", err)
		os.Exit(1)
	}

	chnDNS, err := config.LoadChnDNS(cfg.ChnDNSFile)
	if err != nil {
		log.Error("Failed to load Chinese DNS", "error", err)
		os.Exit(1)
	}

	log.Info("Configuration loaded", "chn_routes", chnRoutes.Size(), "chn_dns", chnDNS.Size())

	// Load gateway state to check for changes
	log.Info("Checking gateway state...")
	gatewayState, err := config.LoadGatewayState("")
	if err != nil {
		log.Warn("Failed to load gateway state", "error", err)
		gatewayState = &config.GatewayState{}
	}

	// Check if gateway has changed
	if gatewayState.IsGatewayChanged(gateway, iface) {
		prevGateway, prevIface := gatewayState.GetPreviousGateway()
		log.Info("Gateway change detected",
			"previous_gateway", prevGateway.String(),
			"previous_interface", prevIface,
			"current_gateway", gateway.String(),
			"current_interface", iface)

		// Clean up routes for previous gateway
		log.Info("Cleaning up routes for previous gateway", "gateway", prevGateway.String())
		if err := cleanupRoutesForGateway(rm, chnRoutes, chnDNS, prevGateway, log); err != nil {
			log.Warn("Failed to cleanup routes for previous gateway", "error", err)
		}
	} else if gatewayState.HasPreviousState() {
		log.Info("Gateway unchanged", "gateway", gateway.String(), "interface", iface)
		// Still clean up any existing routes to ensure consistency
		log.Info("Cleaning up existing routes for consistency...")
		if err := cleanupRoutesForGateway(rm, chnRoutes, chnDNS, gateway, log); err != nil {
			log.Debug("No existing routes found to cleanup", "error", err)
		}
	} else {
		log.Info("First time setup", "gateway", gateway.String(), "interface", iface)
	}

	var routes []routing.Route
	for _, network := range chnRoutes.GetNetworks() {
		routes = append(routes, routing.Route{
			Network: &network,
			Gateway: gateway,
		})
	}

	for _, ip := range chnDNS.GetIPs() {
		var network *net.IPNet
		if ip.To4() != nil {
			network = &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
		} else {
			network = &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
		}
		routes = append(routes, routing.Route{
			Network: network,
			Gateway: gateway,
		})
	}

	log.Info("Setting up routes", "gateway", gateway.String(), "total", len(routes))
	if err := rm.BatchAddRoutes(routes); err != nil {
		log.Error("Failed to setup routes", "error", err)
		os.Exit(1)
	}

	// Update gateway state after successful setup
	gatewayState.Update(gateway, iface, len(routes))
	if err := gatewayState.Save(""); err != nil {
		log.Warn("Failed to save gateway state", "error", err)
	} else {
		log.Info("Gateway state saved", "gateway", gateway.String(), "routes", len(routes))
	}

	log.Info("Route setup completed successfully")
}

func runDaemon(cmd *cobra.Command, args []string) {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if silentMode {
		cfg.SilentMode = true
	}
	cfg.DaemonMode = true

	log := logger.New(cfg)

	sm, err := daemon.NewServiceManager(cfg, log)
	if err != nil {
		log.Error("Failed to create service manager", "error", err)
		os.Exit(1)
	}

	if err := sm.Start(); err != nil {
		log.Error("Failed to start service", "error", err)
		os.Exit(1)
	}

	if err := sm.Wait(); err != nil {
		log.Error("Service error", "error", err)
		os.Exit(1)
	}
}

func installService(cmd *cobra.Command, args []string) {
	if os.Getuid() != 0 {
		fmt.Fprintf(os.Stderr, "Error: Root privileges required for installation\n")
		os.Exit(1)
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
		os.Exit(1)
	}

	configPath := "/etc/smartroute/config.json"
	if configFile != "" {
		configPath = configFile
	}

	switch runtime.GOOS {
	case "darwin":
		service := daemon.NewLaunchdService(execPath, configPath)
		if err := service.Install(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to install service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Service installed successfully (launchd)")
	case "linux":
		// For now, use a basic installation approach for Linux
		fmt.Println("Linux systemd service installation not fully implemented")
		fmt.Printf("Please manually create systemd service using the template in scripts/service/smartroute.service\n")
		fmt.Printf("sudo cp scripts/service/smartroute.service /etc/systemd/system/\n")
		fmt.Printf("sudo systemctl enable smartroute\n")
		fmt.Printf("sudo systemctl start smartroute\n")
	default:
		fmt.Fprintf(os.Stderr, "Service installation not supported on %s\n", runtime.GOOS)
		os.Exit(1)
	}
}

func uninstallService(cmd *cobra.Command, args []string) {
	if os.Getuid() != 0 {
		fmt.Fprintf(os.Stderr, "Error: Root privileges required for uninstallation\n")
		os.Exit(1)
	}

	switch runtime.GOOS {
	case "darwin":
		service := daemon.NewLaunchdService("", "")
		if err := service.Uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to uninstall service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Service uninstalled successfully")
	case "linux":
		fmt.Println("Linux systemd service uninstallation:")
		fmt.Printf("sudo systemctl stop smartroute\n")
		fmt.Printf("sudo systemctl disable smartroute\n")
		fmt.Printf("sudo rm -f /etc/systemd/system/smartroute.service\n")
		fmt.Printf("sudo systemctl daemon-reload\n")
	default:
		fmt.Fprintf(os.Stderr, "Service uninstallation not supported on %s\n", runtime.GOOS)
		os.Exit(1)
	}
}

func showStatus(cmd *cobra.Command, args []string) {
	switch runtime.GOOS {
	case "darwin":
		service := daemon.NewLaunchdService("", "")
		status, err := service.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get service status: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Service status: %s\n", status)
		fmt.Printf("Service installed: %t\n", service.IsInstalled())
	case "linux":
		fmt.Println("Linux systemd service status:")
		fmt.Printf("Check status with: sudo systemctl status smartroute\n")
		fmt.Printf("Check if enabled: sudo systemctl is-enabled smartroute\n")
		fmt.Printf("Check if active: sudo systemctl is-active smartroute\n")
	default:
		fmt.Fprintf(os.Stderr, "Service status not supported on %s\n", runtime.GOOS)
		os.Exit(1)
	}
}

func showVersion(cmd *cobra.Command, args []string) {
	fmt.Printf("Smart Route Manager v%s\n", version)
	fmt.Printf("Runtime: %s\n", runtime.Version())
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	
	gateway, iface, err := network.GetDefaultGateway()
	if err == nil {
		fmt.Printf("Current Gateway: %s (%s)\n", gateway.String(), iface)
	}
}

func testConfiguration(cmd *cobra.Command, args []string) {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Configuration loaded successfully")

	chnRoutes, err := config.LoadChnRoutes(cfg.ChnRouteFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load Chinese routes: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Chinese routes loaded: %d networks\n", chnRoutes.Size())

	chnDNS, err := config.LoadChnDNS(cfg.ChnDNSFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load Chinese DNS: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Chinese DNS loaded: %d servers\n", chnDNS.Size())

	gateway, iface, err := network.GetDefaultGateway()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to get default gateway: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Default gateway: %s (%s)\n", gateway.String(), iface)

	if os.Getuid() != 0 {
		fmt.Println("⚠️  Root privileges required for route operations")
	} else {
		fmt.Println("✅ Root privileges available")
	}

	fmt.Println("✅ All tests passed")
}

func cleanupRoutesForGateway(rm routing.RouteManager, chnRoutes *config.IPSet, chnDNS *config.DNSServers, gateway net.IP, log *logger.Logger) error {
	if gateway == nil {
		return fmt.Errorf("gateway cannot be nil")
	}
	
	log.Info("Cleaning routes for specific gateway", "gateway", gateway.String())
	
	// Build routes to delete for this specific gateway
	var routesToDelete []routing.Route
	
	// Add Chinese network routes
	for _, network := range chnRoutes.GetNetworks() {
		routesToDelete = append(routesToDelete, routing.Route{
			Network: &network,
			Gateway: gateway,
		})
	}
	
	// Add Chinese DNS routes  
	for _, ip := range chnDNS.GetIPs() {
		var network *net.IPNet
		if ip.To4() != nil {
			network = &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
		} else {
			network = &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
		}
		routesToDelete = append(routesToDelete, routing.Route{
			Network: network,
			Gateway: gateway,
		})
	}
	
	// Try to delete these routes
	if len(routesToDelete) > 0 {
		log.Info("Attempting to delete routes", "gateway", gateway.String(), "count", len(routesToDelete))
		err := rm.BatchDeleteRoutes(routesToDelete)
		if err != nil {
			return fmt.Errorf("failed to delete routes for gateway %s: %w", gateway.String(), err)
		}
		log.Info("Successfully cleaned routes", "gateway", gateway.String(), "count", len(routesToDelete))
	}
	
	return nil
}

