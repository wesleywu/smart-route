package utils

import "testing"

func TestGetPhysicalGatewayBSD(t *testing.T) {
	gateway, iface, err := GetPhysicalGatewayBSD()
	if err != nil {
		t.Fatalf("failed to get physical gateway: %v", err)
	}
	t.Logf("gateway: %v, iface: %v", gateway, iface)
}