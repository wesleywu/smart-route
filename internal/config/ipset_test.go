package config

import (
	"net"
	"os"
	"testing"
)

func TestIPSet(t *testing.T) {
	ipset := NewIPSet()
	
	// Test empty set
	if ipset.Size() != 0 {
		t.Errorf("Expected empty set, got size %d", ipset.Size())
	}
	
	// Add a network
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	ipset.Add(*network)
	
	if ipset.Size() != 1 {
		t.Errorf("Expected size 1, got %d", ipset.Size())
	}
	
	// Test Contains
	testIP := net.ParseIP("192.168.1.100")
	if !ipset.Contains(testIP) {
		t.Error("IP should be contained in network")
	}
	
	testIP = net.ParseIP("10.0.0.1")
	if ipset.Contains(testIP) {
		t.Error("IP should not be contained in network")
	}
	
	// Test Clear
	ipset.Clear()
	if ipset.Size() != 0 {
		t.Errorf("Expected empty set after clear, got size %d", ipset.Size())
	}
}

func TestLoadChnRoutes(t *testing.T) {
	// Create a test file
	testFile := "/tmp/test-routes.txt"
	content := `# Test Chinese routes
192.168.1.0/24
10.0.0.0/8

# Another network
172.16.0.0/12
`
	
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)
	
	// Load routes
	ipset, err := LoadChnRoutes(testFile)
	if err != nil {
		t.Fatalf("Failed to load routes: %v", err)
	}
	
	// Should have 3 networks (comments and empty lines ignored)
	if ipset.Size() != 3 {
		t.Errorf("Expected 3 networks, got %d", ipset.Size())
	}
	
	// Test specific IPs
	testCases := []struct {
		ip       string
		expected bool
	}{
		{"192.168.1.100", true},
		{"10.5.5.5", true},
		{"172.16.100.1", true},
		{"8.8.8.8", false},
	}
	
	for _, tc := range testCases {
		ip := net.ParseIP(tc.ip)
		if ipset.Contains(ip) != tc.expected {
			t.Errorf("IP %s: expected %v, got %v", tc.ip, tc.expected, ipset.Contains(ip))
		}
	}
}

func TestLoadChnRoutesInvalidFile(t *testing.T) {
	_, err := LoadChnRoutes("non-existent-file.txt")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestLoadChnRoutesInvalidCIDR(t *testing.T) {
	// Create a test file with invalid CIDR
	testFile := "/tmp/test-invalid-routes.txt"
	content := `192.168.1.0/24
invalid-cidr
10.0.0.0/8
`
	
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)
	
	// Should fail due to invalid CIDR
	_, err = LoadChnRoutes(testFile)
	if err == nil {
		t.Error("Expected error for invalid CIDR")
	}
}