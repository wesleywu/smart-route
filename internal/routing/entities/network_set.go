package entities

import (
	"net"

	"github.com/cespare/xxhash/v2"
)

// NetworkSet is a custom set implementation for IPNet objects using hash-based map
type NetworkSet struct {
	ipNets map[uint64]*net.IPNet // maps network hash to network pointer
}

// NewNetworkSet creates a new NetworkSet
func NewNetworkSet() *NetworkSet {
	return &NetworkSet{
		ipNets: make(map[uint64]*net.IPNet),
	}
}

// Add adds a network to the set
func (rs *NetworkSet) Add(ipNet *net.IPNet) bool {
	if ipNet == nil {
		return false
	}
	hash := hashIPNet(*ipNet)

	// Check if network already exists
	if _, exists := rs.ipNets[hash]; exists {
		return false
	}

	rs.ipNets[hash] = ipNet
	return true
}

// ContainsNetwork checks if the set contains a network
func (rs *NetworkSet) ContainsNetwork(ipNet net.IPNet) bool {
	hash := hashIPNet(ipNet)
	_, exists := rs.ipNets[hash]
	return exists
}

// Size returns the number of networks in the set
func (rs *NetworkSet) Size() int {
	return len(rs.ipNets)
}

// ToRoutes converts the set to a slice of routes with the given gateway
func (rs *NetworkSet) ToRoutes(gateway net.IP) []*Route {
	routes := make([]*Route, 0, len(rs.ipNets))
	for _, ipNet := range rs.ipNets {
		routes = append(routes, &Route{
			Destination: *ipNet,
			Gateway:     gateway,
		})
	}
	return routes
}

// Hash returns the hash code for this Route, only based on Destination network
func hashIPNet(ip net.IPNet) uint64 {
	// Create hasher instance
	h := xxhash.New()

	// Hash the destination IP (convert to 4-byte representation for IPv4)
	ip4 := ip.IP.To4()
	if ip4 != nil {
		// IPv4: use the 4-byte representation
		_, _ = h.Write(ip4)
	} else {
		// IPv6: use the full 16-byte representation
		_, _ = h.Write(ip.IP.To16())
	}

	// Hash the network mask information
	maskBytes, maskBits := ip.Mask.Size()
	_, _ = h.Write([]byte{byte(maskBytes), byte(maskBits)})

	return h.Sum64()
}