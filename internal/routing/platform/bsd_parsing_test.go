//go:build darwin || freebsd

package platform

import (
	"testing"
)

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

	routes, err := parseNetstatOutputBSD(netstatOutput)
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
		foundRoutes[route.Destination.String()] = route.Gateway.String()
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
		t.Logf("  %s -> %s", route.Destination.String(), route.Gateway.String())
	}
}
