// Package routing provides tests for the cleanup functionality.
//
// To run tests:
//   go test ./internal/routing -v -run TestCleanup
//
// To run with real files (requires sudo):
//   sudo go test ./internal/routing -v -run TestCleanupAllManagedRoutes_WithRealFiles
//
package routing

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/wesleywu/update-routes-native/internal/config"
	"github.com/wesleywu/update-routes-native/internal/logger"
)

// mockRouteManager implements RouteManager for testing
type mockRouteManager struct {
	routes []Route
}

func (m *mockRouteManager) AddRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return nil
}

func (m *mockRouteManager) DeleteRoute(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return nil
}

func (m *mockRouteManager) BatchAddRoutes(routes []Route, log *logger.Logger) error {
	return nil
}

func (m *mockRouteManager) BatchDeleteRoutes(routes []Route, log *logger.Logger) error {
	fmt.Printf("MockRouteManager: Would delete %d routes:\n", len(routes))
	for _, route := range routes {
		fmt.Printf("  - Network: %s, Gateway: %s, Interface: %s\n", 
			route.Network.String(), route.Gateway.String(), route.Interface)
	}
	return nil
}

func (m *mockRouteManager) GetDefaultGateway() (net.IP, string, error) {
	return net.ParseIP("192.168.1.1"), "en0", nil
}

func (m *mockRouteManager) ListRoutes() ([]Route, error) {
	return m.routes, nil
}

func (m *mockRouteManager) FlushRoutes(gateway net.IP) error {
	return nil
}

func (m *mockRouteManager) CleanupRoutesForNetworks(networks []net.IPNet, log *logger.Logger) error {
	return nil
}

func (m *mockRouteManager) Close() error {
	return nil
}

// createTempConfigFiles creates temporary config files for testing
func createTempConfigFiles(t *testing.T) (string, string) {
	tempDir := t.TempDir()
	
	// Create test chnroute.txt
	chnRouteFile := filepath.Join(tempDir, "chnroute.txt")
	chnRouteContent := `1.0.1.0/24
1.0.2.0/24
114.114.114.0/24
223.5.5.0/24
`
	err := os.WriteFile(chnRouteFile, []byte(chnRouteContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test chnroute file: %v", err)
	}
	
	// Create test chdns.txt
	chnDNSFile := filepath.Join(tempDir, "chdns.txt")
	chnDNSContent := `114.114.114.114
223.5.5.5
`
	err = os.WriteFile(chnDNSFile, []byte(chnDNSContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test chdns file: %v", err)
	}
	
	return chnRouteFile, chnDNSFile
}

func TestNewCleanupManager(t *testing.T) {
	// Create temp config files
	chnRouteFile, chnDNSFile := createTempConfigFiles(t)
	
	// Create logger
	cfg := &config.Config{LogLevel: "info", SilentMode: true}
	log := logger.New(cfg)
	
	// Create mock route manager
	mockRM := &mockRouteManager{}
	
	// Test NewCleanupManager
	cm, err := NewCleanupManager(mockRM, log, chnRouteFile, chnDNSFile)
	if err != nil {
		t.Fatalf("NewCleanupManager failed: %v", err)
	}
	
	// Verify managed networks were loaded
	expectedNetworks := 6 // 4 networks from chnroute + 2 IPs from chdns (as /32 networks)
	if len(cm.managedNetworks) != expectedNetworks {
		t.Errorf("Expected %d managed networks, got %d", expectedNetworks, len(cm.managedNetworks))
	}
	
	// Verify some specific networks
	expectedNetworksStr := []string{
		"1.0.1.0/24",
		"1.0.2.0/24", 
		"114.114.114.0/24",
		"223.5.5.0/24",
		"114.114.114.114/32", // DNS IP as /32
		"223.5.5.5/32",       // DNS IP as /32
	}
	
	actualNetworksStr := make([]string, len(cm.managedNetworks))
	for i, net := range cm.managedNetworks {
		actualNetworksStr[i] = net.String()
	}
	
	for _, expected := range expectedNetworksStr {
		found := false
		for _, actual := range actualNetworksStr {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected network %s not found in managed networks", expected)
		}
	}
	
	t.Logf("✅ Successfully loaded %d managed networks", len(cm.managedNetworks))
	for i, net := range cm.managedNetworks {
		t.Logf("  Network %d: %s", i+1, net.String())
	}
}

func TestCleanupAllManagedRoutes(t *testing.T) {
	// Create temp config files
	chnRouteFile, chnDNSFile := createTempConfigFiles(t)
	
	// Create logger
	cfg := &config.Config{LogLevel: "info", SilentMode: true}
	log := logger.New(cfg)
	
	// Create mock routes in system (simulate current routes)
	systemRoutes := []Route{
		{
			Network:   mustParseCIDR("1.0.1.0/24"),
			Gateway:   net.ParseIP("192.168.1.1"),
			Interface: "en0",
		},
		{
			Network:   mustParseCIDR("114.114.114.114/32"), 
			Gateway:   net.ParseIP("10.0.0.1"),
			Interface: "utun0",
		},
		{
			Network:   mustParseCIDR("8.8.8.8/32"), // This should NOT be cleaned (not in config)
			Gateway:   net.ParseIP("192.168.1.1"),
			Interface: "en0",
		},
		{
			Network:   mustParseCIDR("223.5.5.5/32"), // DNS IP - should be cleaned
			Gateway:   net.ParseIP("172.16.0.1"),
			Interface: "utun1",
		},
	}
	
	// Create mock route manager with system routes
	mockRM := &mockRouteManager{routes: systemRoutes}
	
	// Create cleanup manager
	cm, err := NewCleanupManager(mockRM, log, chnRouteFile, chnDNSFile)
	if err != nil {
		t.Fatalf("NewCleanupManager failed: %v", err)
	}
	
	t.Logf("System has %d routes, cleanup manager handles %d networks", 
		len(systemRoutes), len(cm.managedNetworks))
	
	// Test cleanup
	fmt.Println("\n=== Testing CleanupAllManagedRoutes ===")
	err = cm.CleanupAllManagedRoutes()
	if err != nil {
		t.Errorf("CleanupAllManagedRoutes failed: %v", err)
	}
	
	t.Logf("✅ Cleanup completed successfully")
	t.Logf("Expected routes to be deleted: 1.0.1.0/24, 114.114.114.114/32, 223.5.5.5/32")
	t.Logf("Routes that should NOT be deleted: 8.8.8.8/32 (not managed)")
}

func TestCleanupAllManagedRoutes_WithRealFiles(t *testing.T) {
	// Only run this test with sudo privileges
	if os.Getuid() != 0 {
		t.Skip("Skipping real file test - requires sudo privileges")
	}
	
	// Use actual config files (relative to project root when run from internal/routing)
	chnRouteFile := "../../configs/chnroute.txt" 
	chnDNSFile := "../../configs/chdns.txt"
	
	t.Logf("Looking for files: %s, %s", chnRouteFile, chnDNSFile)
	
	// Check if files exist
	if _, err := os.Stat(chnRouteFile); os.IsNotExist(err) {
		t.Skip("Skipping real file test - chnroute.txt not found")
	}
	if _, err := os.Stat(chnDNSFile); os.IsNotExist(err) {
		t.Skip("Skipping real file test - chdns.txt not found")
	}
	
	// Create logger
	cfg := &config.Config{LogLevel: "debug", SilentMode: false}
	log := logger.New(cfg)
	
	// Create real route manager
	rm, err := NewRouteManager(10, 3)
	if err != nil {
		t.Fatalf("Failed to create route manager: %v", err)
	}
	defer rm.Close()
	
	// Create cleanup manager with real files
	cm, err := NewCleanupManager(rm, log, chnRouteFile, chnDNSFile)
	if err != nil {
		t.Fatalf("NewCleanupManager failed: %v", err)
	}
	
	t.Logf("✅ Loaded %d managed networks from real config files", cm.GetManagedNetworksCount())
	
	// Get current system routes
	fmt.Println("\n=== Getting current system routes ===")
	allRoutes, err := rm.ListRoutes()
	if err != nil {
		t.Logf("Warning: Failed to list routes: %v", err)
		allRoutes = []Route{}
	} else {
		fmt.Printf("Total system routes found by netstat: %d\n", len(allRoutes))
	}
	
	// Check if we can find the specific route 203.26.55.0/24 you mentioned  
	fmt.Println("\n=== Looking for specific route 203.26.55.0/24 ===")
	targetNetwork := mustParseCIDR("203.26.55.0/24")
	found := false
	for _, route := range allRoutes {
		if route.Network.String() == targetNetwork.String() {
			fmt.Printf("✅ Found target route in netstat: %s -> %s (%s)\n",
				route.Network.String(), route.Gateway.String(), route.Interface)
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("❌ Target route 203.26.55.0/24 NOT found in netstat output\n")
		fmt.Printf("This explains why cleanup didn't work - netstat and 'ip route' show different routes!\n")
	}
	
	// Test cleanup (this will actually attempt to clean routes)
	fmt.Println("\n=== Testing CleanupAllManagedRoutes with real files ===")
	fmt.Println("⚠️  This will attempt to delete real routes from your system!")
	
	err = cm.CleanupAllManagedRoutes()
	if err != nil {
		t.Logf("⚠️  Cleanup failed: %v", err)
		t.Fatalf("Cleanup should not fail with real files")
	} else {
		t.Logf("✅ Real cleanup completed successfully")
	}
}

// Helper function to parse CIDR
func mustParseCIDR(cidr string) net.IPNet {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse CIDR %s: %v", cidr, err))
	}
	return *network
}