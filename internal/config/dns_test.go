package config

import (
	"net"
	"os"
	"testing"
)

func TestDNSServers(t *testing.T) {
	dns := NewDNSServers()
	
	// Test empty set
	if dns.Size() != 0 {
		t.Errorf("Expected empty set, got size %d", dns.Size())
	}
	
	// Add an IP
	testIP := net.ParseIP("8.8.8.8")
	dns.Add(testIP)
	
	if dns.Size() != 1 {
		t.Errorf("Expected size 1, got %d", dns.Size())
	}
	
	// Test Contains
	if !dns.Contains(testIP) {
		t.Error("IP should be contained in DNS list")
	}
	
	otherIP := net.ParseIP("1.1.1.1")
	if dns.Contains(otherIP) {
		t.Error("IP should not be contained in DNS list")
	}
	
	// Test Clear
	dns.Clear()
	if dns.Size() != 0 {
		t.Errorf("Expected empty set after clear, got size %d", dns.Size())
	}
}

func TestLoadChnDNS(t *testing.T) {
	// Create a test file
	testFile := "/tmp/test-dns.txt"
	content := `# Test Chinese DNS servers
223.5.5.5
114.114.114.114

# Alibaba DNS
223.6.6.6
`
	
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)
	
	// Load DNS servers
	dns, err := LoadChnDNS(testFile)
	if err != nil {
		t.Fatalf("Failed to load DNS servers: %v", err)
	}
	
	// Should have 3 servers (comments and empty lines ignored)
	if dns.Size() != 3 {
		t.Errorf("Expected 3 DNS servers, got %d", dns.Size())
	}
	
	// Test specific IPs
	testCases := []struct {
		ip       string
		expected bool
	}{
		{"223.5.5.5", true},
		{"114.114.114.114", true},
		{"223.6.6.6", true},
		{"8.8.8.8", false},
	}
	
	for _, tc := range testCases {
		ip := net.ParseIP(tc.ip)
		if dns.Contains(ip) != tc.expected {
			t.Errorf("IP %s: expected %v, got %v", tc.ip, tc.expected, dns.Contains(ip))
		}
	}
}

func TestLoadChnDNSInvalidFile(t *testing.T) {
	_, err := LoadChnDNS("non-existent-file.txt")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestLoadChnDNSInvalidIP(t *testing.T) {
	// Create a test file with invalid IP
	testFile := "/tmp/test-invalid-dns.txt"
	content := `223.5.5.5
invalid-ip
114.114.114.114
`
	
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)
	
	// Should fail due to invalid IP
	_, err = LoadChnDNS(testFile)
	if err == nil {
		t.Error("Expected error for invalid IP")
	}
}

func TestGetIPs(t *testing.T) {
	dns := NewDNSServers()
	
	// Add some IPs
	ip1 := net.ParseIP("223.5.5.5")
	ip2 := net.ParseIP("114.114.114.114")
	dns.Add(ip1)
	dns.Add(ip2)
	
	ips := dns.GetIPs()
	if len(ips) != 2 {
		t.Errorf("Expected 2 IPs, got %d", len(ips))
	}
	
	// Verify IPs are copied (not references)
	if &ips[0] == &dns.IPs[0] {
		t.Error("GetIPs should return copies, not references")
	}
}