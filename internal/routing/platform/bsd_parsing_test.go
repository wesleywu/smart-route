//go:build darwin || freebsd

package platform

import (
	"net"
	"testing"
)

// Test parsing of BSD netstat simplified format destinations
func TestParseDestination_SimplifiedNetworkFormats(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedIP     string
		expectedMask   string
		expectedCIDR   string
		shouldSucceed  bool
	}{
		// Real examples from your system
		{
			name:         "203.57.66 simplified format",
			input:        "203.57.66",
			expectedIP:   "203.57.66.0",
			expectedMask: "ffffff00",
			expectedCIDR: "203.57.66.0/24",
			shouldSucceed: true,
		},
		{
			name:         "203.57.69 simplified format",
			input:        "203.57.69",
			expectedIP:   "203.57.69.0",
			expectedMask: "ffffff00",
			expectedCIDR: "203.57.69.0/24",
			shouldSucceed: true,
		},
		{
			name:         "203.57.73 simplified format",
			input:        "203.57.73",
			expectedIP:   "203.57.73.0",
			expectedMask: "ffffff00",
			expectedCIDR: "203.57.73.0/24",
			shouldSucceed: true,
		},
		{
			name:         "203.57.90 simplified format",
			input:        "203.57.90",
			expectedIP:   "203.57.90.0",
			expectedMask: "ffffff00",
			expectedCIDR: "203.57.90.0/24",
			shouldSucceed: true,
		},
		{
			name:         "203.57.101 simplified format",
			input:        "203.57.101",
			expectedIP:   "203.57.101.0",
			expectedMask: "ffffff00",
			expectedCIDR: "203.57.101.0/24",
			shouldSucceed: true,
		},
		// Additional test cases for different formats
		{
			name:         "Two octet network (10.0)",
			input:        "10.0",
			expectedIP:   "10.0.0.0",
			expectedMask: "ffff0000",
			expectedCIDR: "10.0.0.0/16",
			shouldSucceed: true,
		},
		{
			name:         "Complete IP address",
			input:        "192.168.1.100",
			expectedIP:   "192.168.1.100",
			expectedMask: "ffffffff",
			expectedCIDR: "192.168.1.100/32",
			shouldSucceed: true,
		},
		{
			name:         "Default route",
			input:        "default",
			expectedIP:   "0.0.0.0",
			expectedMask: "00000000",
			expectedCIDR: "0.0.0.0/0",
			shouldSucceed: true,
		},
		{
			name:         "Complete CIDR notation",
			input:        "192.168.1.0/24",
			expectedIP:   "192.168.1.0",
			expectedMask: "ffffff00",
			expectedCIDR: "192.168.1.0/24",
			shouldSucceed: true,
		},
		// Edge cases
		{
			name:         "Single number (invalid)",
			input:        "203",
			expectedCIDR: "",
			shouldSucceed: false,
		},
		{
			name:         "Empty string (invalid)",
			input:        "",
			expectedCIDR: "",
			shouldSucceed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseDestination(tc.input)
			
			if tc.shouldSucceed {
				if err != nil {
					t.Errorf("Expected success but got error: %v", err)
					return
				}
				
				if result == nil {
					t.Errorf("Expected result but got nil")
					return
				}
				
				// Check IP
				if result.IP.String() != tc.expectedIP {
					t.Errorf("Expected IP %s, got %s", tc.expectedIP, result.IP.String())
				}
				
				// Check mask
				if result.Mask.String() != tc.expectedMask {
					t.Errorf("Expected mask %s, got %s", tc.expectedMask, result.Mask.String())
				}
				
				// Check CIDR representation
				if result.String() != tc.expectedCIDR {
					t.Errorf("Expected CIDR %s, got %s", tc.expectedCIDR, result.String())
				}
				
				t.Logf("✅ Correctly parsed '%s' -> %s", tc.input, result.String())
			} else {
				if err == nil {
					t.Errorf("Expected error but got success with result: %v", result)
				} else {
					t.Logf("✅ Correctly rejected invalid input '%s': %v", tc.input, err)
				}
			}
		})
	}
}

// Test complete netstat line parsing with simplified formats
func TestParseNetstatOutput_WithSimplifiedFormats(t *testing.T) {
	// Simulate netstat output with simplified network formats like your system shows
	netstatOutput := `Routing tables

Internet:
Destination        Gateway            Flags               Netif Expire
default            192.168.32.1       UGScIg                en0       
203.57.66          192.168.32.1       UGSc                  en0       
203.57.69          192.168.32.1       UGSc                  en0       
203.57.73          192.168.32.1       UGSc                  en0       
203.57.90          192.168.32.1       UGSc                  en0       
203.57.101         192.168.32.1       UGSc                  en0       
203.26.55          192.168.32.1       UGSc                  en0       
10.0               192.168.32.1       UGSc                  en0       
192.168.1.100      192.168.32.1       UGHS                  en0       
`

	routes, err := parseNetstatOutput(netstatOutput)
	if err != nil {
		t.Fatalf("Failed to parse netstat output: %v", err)
	}

	expectedRoutes := map[string]string{
		"0.0.0.0/0":        "192.168.32.1", // default
		"203.57.66.0/24":   "192.168.32.1", // simplified 3-octet
		"203.57.69.0/24":   "192.168.32.1", // simplified 3-octet
		"203.57.73.0/24":   "192.168.32.1", // simplified 3-octet
		"203.57.90.0/24":   "192.168.32.1", // simplified 3-octet
		"203.57.101.0/24":  "192.168.32.1", // simplified 3-octet
		"203.26.55.0/24":   "192.168.32.1", // simplified 3-octet
		"10.0.0.0/16":      "192.168.32.1", // simplified 2-octet
		"192.168.1.100/32": "192.168.32.1", // complete IP as /32
	}

	t.Logf("Parsed %d routes from netstat output", len(routes))

	// Check that all expected routes are present
	foundRoutes := make(map[string]string)
	for _, route := range routes {
		foundRoutes[route.Network.String()] = route.Gateway.String()
	}

	for expectedNetwork, expectedGateway := range expectedRoutes {
		if gateway, found := foundRoutes[expectedNetwork]; found {
			if gateway == expectedGateway {
				t.Logf("✅ Found expected route: %s -> %s", expectedNetwork, gateway)
			} else {
				t.Errorf("❌ Route %s found but with wrong gateway: expected %s, got %s", 
					expectedNetwork, expectedGateway, gateway)
			}
		} else {
			t.Errorf("❌ Expected route not found: %s -> %s", expectedNetwork, expectedGateway)
		}
	}

	// Log all found routes for debugging
	t.Logf("All parsed routes:")
	for _, route := range routes {
		t.Logf("  %s -> %s (%s)", route.Network.String(), route.Gateway.String(), route.Interface)
	}
}

// Benchmark parsing performance with simplified formats
func BenchmarkParseDestination_SimplifiedFormats(b *testing.B) {
	testInputs := []string{
		"203.57.66",
		"203.57.69", 
		"203.57.73",
		"203.57.90",
		"203.57.101",
		"203.26.55",
		"10.0",
		"default",
		"192.168.1.100",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, input := range testInputs {
			_, _ = parseDestination(input)
		}
	}
}

// Test that routesMatch function works correctly with simplified formats
func TestRoutesMatch_WithSimplifiedFormats(t *testing.T) {
	// Test matching between config file networks and parsed netstat networks
	testCases := []struct {
		name           string
		configNetwork  string  // From chnroute.txt
		netstatNetwork string  // From parsed netstat (simplified format)
		shouldMatch    bool
	}{
		{
			name:           "Exact match for 203.57.66.0/24",
			configNetwork:  "203.57.66.0/24",
			netstatNetwork: "203.57.66.0/24", // After parsing "203.57.66"
			shouldMatch:    true,
		},
		{
			name:           "Exact match for 203.26.55.0/24",
			configNetwork:  "203.26.55.0/24",
			netstatNetwork: "203.26.55.0/24", // After parsing "203.26.55"
			shouldMatch:    true,
		},
		{
			name:           "No match for different networks",
			configNetwork:  "203.57.66.0/24",
			netstatNetwork: "203.57.67.0/24",
			shouldMatch:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse both networks
			_, configNet, err := net.ParseCIDR(tc.configNetwork)
			if err != nil {
				t.Fatalf("Failed to parse config network %s: %v", tc.configNetwork, err)
			}
			
			_, netstatNet, err := net.ParseCIDR(tc.netstatNetwork)
			if err != nil {
				t.Fatalf("Failed to parse netstat network %s: %v", tc.netstatNetwork, err)
			}

			// Use the routesMatch function (or similar logic)
			matches := routesMatch(*configNet, *netstatNet)
			
			if matches != tc.shouldMatch {
				t.Errorf("Expected match=%t for %s vs %s, but got %t", 
					tc.shouldMatch, tc.configNetwork, tc.netstatNetwork, matches)
			} else {
				t.Logf("✅ Correct match result for %s vs %s: %t", 
					tc.configNetwork, tc.netstatNetwork, matches)
			}
		})
	}
}
