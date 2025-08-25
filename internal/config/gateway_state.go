package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

type GatewayState struct {
	PreviousGateway net.IP    `json:"previous_gateway"`
	PreviousIface   string    `json:"previous_interface"`
	LastUpdate      time.Time `json:"last_update"`
	RouteCount      int       `json:"route_count"`
}

const DefaultStateFile = "/tmp/smartroute_gateway_state.json"

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

func (gs *GatewayState) Update(gateway net.IP, iface string, routeCount int) {
	gs.PreviousGateway = make(net.IP, len(gateway))
	copy(gs.PreviousGateway, gateway)
	gs.PreviousIface = iface
	gs.LastUpdate = time.Now()
	gs.RouteCount = routeCount
}

func (gs *GatewayState) HasPreviousState() bool {
	return gs.PreviousGateway != nil && !gs.LastUpdate.IsZero()
}

func (gs *GatewayState) IsGatewayChanged(currentGateway net.IP, currentIface string) bool {
	if !gs.HasPreviousState() {
		return false
	}
	
	return !gs.PreviousGateway.Equal(currentGateway) || gs.PreviousIface != currentIface
}

func (gs *GatewayState) GetPreviousGateway() (net.IP, string) {
	if !gs.HasPreviousState() {
		return nil, ""
	}
	
	gateway := make(net.IP, len(gs.PreviousGateway))
	copy(gateway, gs.PreviousGateway)
	
	return gateway, gs.PreviousIface
}

func (gs *GatewayState) Clear() {
	gs.PreviousGateway = nil
	gs.PreviousIface = ""
	gs.LastUpdate = time.Time{}
	gs.RouteCount = 0
}