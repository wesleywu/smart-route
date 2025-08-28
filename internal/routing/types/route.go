package types

import (
	"net"
)

// Route represents a system route table entry
type Route struct {
	Destination net.IPNet // Destination network
	Gateway     net.IP    // Gateway IP address
	Metric      int       // Route metric/priority
}

// RouteAction represents the type of operation to be performed on a route
type RouteAction int

// Route action constants
const (
	// RouteActionAdd adds a route to the system routing table
	RouteActionAdd RouteAction = iota
	// RouteActionDelete removes a route from the system routing table
	RouteActionDelete
)