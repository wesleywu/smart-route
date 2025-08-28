package utils

import (
	"testing"
)

// Test parsing of BSD netstat simplified format destinations
func TestParseDestination_SimplifiedNetworkFormats(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		expectedIP    string
		expectedMask  string
		expectedCIDR  string
		shouldSucceed bool
	}{
		// Real examples from your system
		{
			name:          "203.57.66 simplified format",
			input:         "203.57.66",
			expectedIP:    "203.57.66.0",
			expectedMask:  "ffffff00",
			expectedCIDR:  "203.57.66.0/24",
			shouldSucceed: true,
		},
		{
			name:          "203.57.69 simplified format",
			input:         "203.57.69",
			expectedIP:    "203.57.69.0",
			expectedMask:  "ffffff00",
			expectedCIDR:  "203.57.69.0/24",
			shouldSucceed: true,
		},
		{
			name:          "203.57.73 simplified format",
			input:         "203.57.73",
			expectedIP:    "203.57.73.0",
			expectedMask:  "ffffff00",
			expectedCIDR:  "203.57.73.0/24",
			shouldSucceed: true,
		},
		{
			name:          "203.57.90 simplified format",
			input:         "203.57.90",
			expectedIP:    "203.57.90.0",
			expectedMask:  "ffffff00",
			expectedCIDR:  "203.57.90.0/24",
			shouldSucceed: true,
		},
		{
			name:          "203.57.101 simplified format",
			input:         "203.57.101",
			expectedIP:    "203.57.101.0",
			expectedMask:  "ffffff00",
			expectedCIDR:  "203.57.101.0/24",
			shouldSucceed: true,
		},
		// Additional test cases for different formats
		{
			name:          "Two octet network (10.0)",
			input:         "10.0",
			expectedIP:    "10.0.0.0",
			expectedMask:  "ffff0000",
			expectedCIDR:  "10.0.0.0/16",
			shouldSucceed: true,
		},
		{
			name:          "Complete IP address",
			input:         "192.168.1.100",
			expectedIP:    "192.168.1.100",
			expectedMask:  "ffffffff",
			expectedCIDR:  "192.168.1.100/32",
			shouldSucceed: true,
		},
		{
			name:          "Default route",
			input:         "default",
			expectedIP:    "0.0.0.0",
			expectedMask:  "00000000",
			expectedCIDR:  "0.0.0.0/0",
			shouldSucceed: true,
		},
		{
			name:          "Complete CIDR notation",
			input:         "192.168.1.0/24",
			expectedIP:    "192.168.1.0",
			expectedMask:  "ffffff00",
			expectedCIDR:  "192.168.1.0/24",
			shouldSucceed: true,
		},
		// Edge cases
		{
			name:          "Single number (invalid)",
			input:         "203",
			expectedCIDR:  "",
			shouldSucceed: false,
		},
		{
			name:          "Empty string (invalid)",
			input:         "",
			expectedCIDR:  "",
			shouldSucceed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseDestination(tc.input)

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
			_, _ = ParseDestination(input)
		}
	}
}
