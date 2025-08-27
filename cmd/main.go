package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/wesleywu/smart-route/internal/config"
	"github.com/wesleywu/smart-route/internal/daemon"
	"github.com/wesleywu/smart-route/internal/logger"
	"github.com/wesleywu/smart-route/internal/routing"
)

var (
	version = "1.0.0"

	// Command line flags
	silentMode bool
	verboseMode bool  
	routeFile  string
	dnsFile    string
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

	rootCmd.PersistentFlags().BoolVarP(&silentMode, "silent", "s", false, "Silent mode (no output)")
	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Verbose mode (debug level logging)")
	rootCmd.PersistentFlags().StringVar(&routeFile, "route-file", "", "External routes file path (defaults to embedded data)")
	rootCmd.PersistentFlags().StringVar(&dnsFile, "dns-file", "", "External DNS file path (defaults to embedded data)")

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

func runOnce(_ *cobra.Command, _ []string) {
	// Determine log level based on command line flags
	logLevel := "info"
	if verboseMode {
		logLevel = "debug"
	} else if silentMode {
		logLevel = "error"
	}

	cfg := config.NewConfig()

	log := logger.New(logLevel)
	log.Info("Starting one-time route setup", "version", version)

	rm, err := routing.NewPlatformRouteManager(cfg.ConcurrencyLimit, cfg.RetryAttempts)
	if err != nil {
		log.Error("Failed to create route manager", "error", err)
		os.Exit(1)
	}
	defer rm.Close()

	gateway, iface, err := rm.GetPhysicalGateway()
	if err != nil {
		log.Error("Failed to get default gateway", "error", err)
		os.Exit(1)
	}

	log.Info("Default gateway detected", "gateway", gateway.String(), "interface", iface)

	chnRoutes, err := config.LoadChnRoutesWithFallback(routeFile)
	if err != nil {
		log.Error("Failed to load Chinese routes", "error", err)
		os.Exit(1)
	}

	chnDNS, err := config.LoadChnDNSWithFallback(dnsFile)
	if err != nil {
		log.Error("Failed to load Chinese DNS", "error", err)
		os.Exit(1)
	}

	log.Info("Configuration loaded", "chn_routes", chnRoutes.Size(), "chn_dns", chnDNS.Size())

	// Create unified route switch handler
	routeSwitch, err := routing.NewRouteSwitch(rm, chnRoutes, chnDNS, log)
	if err != nil {
		log.Error("Failed to create route switch", "error", err)
		os.Exit(1)
	}

	// One-time mode: Always perform complete route reset
	// This ensures consistent behavior and clean state for every run
	log.Info("One-time mode: performing complete route reset",
		"current_gateway", gateway.String(), "interface", iface)

	// Always use the unified logic: cleanup all managed routes, then setup for current gateway
	if err := routeSwitch.SetupRoutes(gateway); err != nil {
		log.Error("Failed to setup routes", "error", err)
		os.Exit(1)
	}

	log.Info("Route setup completed successfully")
}

func runDaemon(cmd *cobra.Command, args []string) {
	// Determine log level based on command line flags
	logLevel := "info"
	if verboseMode {
		logLevel = "debug"
	} else if silentMode {
		logLevel = "error"
	}

	cfg := config.NewConfig()

	log := logger.New(logLevel)

	sm, err := daemon.NewServiceManager(cfg, log, routeFile, dnsFile)
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

	// No longer using config files - pass empty string
	service := daemon.NewPlatformService(execPath, "")
	if err := service.Install(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to install service: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Service installed successfully (%s)\n", runtime.GOOS)
}

func uninstallService(cmd *cobra.Command, args []string) {
	if os.Getuid() != 0 {
		fmt.Fprintf(os.Stderr, "Error: Root privileges required for uninstallation\n")
		os.Exit(1)
	}

	service := daemon.NewPlatformService("", "")
	if err := service.Uninstall(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to uninstall service: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Service uninstalled successfully")
}

func showStatus(cmd *cobra.Command, args []string) {
	service := daemon.NewPlatformService("", "")
	status, err := service.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get service status: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Service status: %s\n", status)
	fmt.Printf("Service installed: %t\n", service.IsInstalled())
}

func showVersion(cmd *cobra.Command, args []string) {
	fmt.Printf("Smart Route Manager v%s\n", version)
	fmt.Printf("Runtime: %s\n", runtime.Version())
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	// Try to show current gateway information
	rm, err := routing.NewPlatformRouteManager(1, 1) // minimal settings for quick check
	if err == nil {
		defer rm.Close()
		gateway, iface, err := rm.GetPhysicalGateway()
		if err == nil {
			fmt.Printf("Current Gateway: %s (%s)\n", gateway.String(), iface)
		}
	}
}

func testConfiguration(cmd *cobra.Command, args []string) {
	// Determine log level based on command line flags
	logLevel := "info"
	if verboseMode {
		logLevel = "debug"
	} else if silentMode {
		logLevel = "error"
	}

	cfg := config.NewConfig()

	log := logger.New(logLevel)
	log.Debug("Starting configuration test")
	fmt.Println("✅ Configuration loaded successfully")

	chnRoutes, err := config.LoadChnRoutesWithFallback(routeFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load Chinese routes: %v\n", err)
		os.Exit(1)
	}
	log.Debug("Chinese routes loading details", "file", routeFile, "networks", chnRoutes.Size())
	fmt.Printf("✅ Chinese routes loaded: %d networks\n", chnRoutes.Size())

	chnDNS, err := config.LoadChnDNSWithFallback(dnsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load Chinese DNS: %v\n", err)
		os.Exit(1)
	}
	log.Debug("Chinese DNS loading details", "file", dnsFile, "servers", chnDNS.Size())
	fmt.Printf("✅ Chinese DNS loaded: %d servers\n", chnDNS.Size())

	rm, err := routing.NewPlatformRouteManager(cfg.ConcurrencyLimit, cfg.RetryAttempts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to create route manager: %v\n", err)
		os.Exit(1)
	}
	defer rm.Close()

	gateway, iface, err := rm.GetPhysicalGateway()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to get default gateway: %v\n", err)
		os.Exit(1)
	}
	log.Debug("Gateway detection details", "gateway", gateway.String(), "interface", iface)
	fmt.Printf("✅ Default gateway: %s (%s)\n", gateway.String(), iface)

	if os.Getuid() != 0 {
		fmt.Println("⚠️  Root privileges required for route operations")
	} else {
		fmt.Println("✅ Root privileges available")
	}

	fmt.Println("✅ All tests passed")
}
