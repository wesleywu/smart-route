package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// GatewayState represents the state of the gateway
type GatewayState struct {
	PreviousGateway net.IP    `json:"previous_gateway"`
	PreviousIface   string    `json:"previous_interface"`
	LastUpdate      time.Time `json:"last_update"`
	RouteCount      int       `json:"route_count"`
}

// DefaultStateFile is the default file to store the gateway state
const DefaultStateFile = "/tmp/smartroute_gateway_state.json"

// LoadGatewayState loads the gateway state from a file
func LoadGatewayState(stateFile string) (*GatewayState, error) {
	if stateFile == "" {
		stateFile = DefaultStateFile
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// First run, return empty state
			return &GatewayState{}, nil
		}
		return nil, fmt.Errorf("failed to read gateway state: %w", err)
	}

	var state GatewayState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse gateway state: %w", err)
	}

	return &state, nil
}

// Save saves the gateway state to a file
func (gs *GatewayState) Save(stateFile string) error {
	if stateFile == "" {
		stateFile = DefaultStateFile
	}

	// Ensure directory exists
	dir := filepath.Dir(stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(gs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal gateway state: %w", err)
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write gateway state: %w", err)
	}

	return nil
}

// Update updates the gateway state
func (gs *GatewayState) Update(gateway net.IP, iface string, routeCount int) {
	gs.PreviousGateway = make(net.IP, len(gateway))
	copy(gs.PreviousGateway, gateway)
	gs.PreviousIface = iface
	gs.LastUpdate = time.Now()
	gs.RouteCount = routeCount
}

// HasPreviousState checks if the gateway state has a previous state
func (gs *GatewayState) HasPreviousState() bool {
	return gs.PreviousGateway != nil && !gs.LastUpdate.IsZero()
}

// IsGatewayChanged checks if the gateway has changed
func (gs *GatewayState) IsGatewayChanged(currentGateway net.IP, currentIface string) bool {
	if !gs.HasPreviousState() {
		return false
	}
	
	return !gs.PreviousGateway.Equal(currentGateway) || gs.PreviousIface != currentIface
}

// GetPreviousGateway returns the previous gateway
func (gs *GatewayState) GetPreviousGateway() (net.IP, string) {
	if !gs.HasPreviousState() {
		return nil, ""
	}
	
	gateway := make(net.IP, len(gs.PreviousGateway))
	copy(gateway, gs.PreviousGateway)
	
	return gateway, gs.PreviousIface
}

// Clear clears the gateway state
func (gs *GatewayState) Clear() {
	gs.PreviousGateway = nil
	gs.PreviousIface = ""
	gs.LastUpdate = time.Time{}
	gs.RouteCount = 0
}