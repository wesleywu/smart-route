package platform

import "net"

// routesMatch checks if two networks are equivalent
func routesMatch(net1, net2 net.IPNet) bool {
	return net1.IP.Equal(net2.IP) && 
		   len(net1.Mask) == len(net2.Mask) &&
		   net1.Mask.String() == net2.Mask.String()
}