//go:build darwin || freebsd

package routing

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"

	"github.com/wesleywu/update-routes-native/internal/logger"
	"golang.org/x/sys/unix"
)

// BSD route message types
const (
	RTM_ADD      = 0x1
	RTM_DELETE   = 0x2
	RTM_CHANGE   = 0x3
	RTM_GET      = 0x4
	RTM_LOSING   = 0x5
	RTM_REDIRECT = 0x6
	RTM_MISS     = 0x7
	RTM_LOCK     = 0x8
	RTM_OLDADD   = 0x9
	RTM_OLDDEL   = 0xa
	RTM_RESOLVE  = 0xb
	RTM_NEWADDR  = 0xc
	RTM_DELADDR  = 0xd
	RTM_IFINFO   = 0xe
)

// Route flags
const (
	RTF_UP        = 0x1
	RTF_GATEWAY   = 0x2
	RTF_HOST      = 0x4
	RTF_REJECT    = 0x8
	RTF_DYNAMIC   = 0x10
	RTF_MODIFIED  = 0x20
	RTF_DONE      = 0x40
	RTF_DELCLONE  = 0x80
	RTF_CLONING   = 0x100
	RTF_XRESOLVE  = 0x200
	RTF_LLINFO    = 0x400
	RTF_STATIC    = 0x800
	RTF_BLACKHOLE = 0x1000
	RTF_PROTO2    = 0x4000
	RTF_PROTO1    = 0x8000
	RTF_PRCLONING = 0x10000
	RTF_WASCLONED = 0x20000
	RTF_PROTO3    = 0x40000
	RTF_PINNED    = 0x100000
	RTF_LOCAL     = 0x200000
	RTF_BROADCAST = 0x400000
	RTF_MULTICAST = 0x800000
	RTF_IFSCOPE   = 0x1000000
	RTF_CONDEMNED = 0x2000000
	RTF_IFREF     = 0x4000000
	RTF_PROXY     = 0x8000000
	RTF_ROUTER    = 0x10000000
)

// Socket address types
const (
	RTA_DST     = 0x1
	RTA_GATEWAY = 0x2
	RTA_NETMASK = 0x4
	RTA_GENMASK = 0x8
	RTA_IFP     = 0x10
	RTA_IFA     = 0x20
	RTA_AUTHOR  = 0x40
	RTA_BRD     = 0x80
)

// Route message header
type rtMsghdr struct {
	msglen  uint16
	version uint8
	msgtype uint8
	hdrlen  uint16
	index   uint16
	flags   int32
	addrs   int32
	pid     int32
	seq     int32
	errno   int32
	use     int32
	inits   uint32
	rmx     rtMetrics
}

type rtMetrics struct {
	locks    uint32
	mtu      uint32
	hopcount uint32
	expire   int32
	recvpipe uint32
	sendpipe uint32
	ssthresh uint32
	rtt      uint32
	rttvar   uint32
	pksent   uint32
	weight   uint32
	filler   [3]uint32
}

// Socket address structures
type sockaddrInet struct {
	len    uint8
	family uint8
	port   uint16
	addr   [4]byte
	zero   [8]int8
}

func (rm *BSDRouteManager) addRouteNative(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.sendRouteMessage(RTM_ADD, network, gateway, log)
}

func (rm *BSDRouteManager) deleteRouteNative(network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	return rm.sendRouteMessage(RTM_DELETE, network, gateway, log)
}

func (rm *BSDRouteManager) sendRouteMessage(msgType uint8, network *net.IPNet, gateway net.IP, log *logger.Logger) error {
	// For deletion, ensure we use the canonical network address (network IP masked with netmask)
	networkAddr := network.IP.Mask(network.Mask)

	// Convert network and gateway to sockaddr structures
	dst := ipToSockaddr(networkAddr)
	gw := ipToSockaddr(gateway)
	mask := maskToSockaddr(network.Mask)

	// Calculate message size
	msgSize := int(unsafe.Sizeof(rtMsghdr{})) +
		int(dst.len) + int(gw.len) + int(mask.len)

	// Align to 4-byte boundary
	msgSize = (msgSize + 3) &^ 3

	// Create message buffer
	buf := make([]byte, msgSize)

	// Fill in route message header
	hdr := (*rtMsghdr)(unsafe.Pointer(&buf[0]))
	hdr.msglen = uint16(msgSize)
	hdr.version = unix.RTM_VERSION
	hdr.msgtype = msgType
	hdr.hdrlen = uint16(unsafe.Sizeof(rtMsghdr{}))
	hdr.index = 0

	// Set appropriate flags based on operation type
	if msgType == RTM_ADD {
		hdr.flags = RTF_UP | RTF_GATEWAY | RTF_STATIC
	} else if msgType == RTM_DELETE {
		// For deletion, match the existing route flags exactly
		hdr.flags = RTF_GATEWAY | RTF_STATIC
	}

	hdr.addrs = RTA_DST | RTA_GATEWAY | RTA_NETMASK
	hdr.pid = int32(syscall.Getpid())
	
	// Use atomic increment for sequence number to avoid conflicts
	// This must be done within the mutex-protected section
	rm.seqNum++
	hdr.seq = rm.seqNum

	// Add socket addresses
	offset := int(unsafe.Sizeof(rtMsghdr{}))

	// Destination
	copy(buf[offset:], (*[16]byte)(unsafe.Pointer(dst))[:dst.len])
	offset += roundUp(int(dst.len))

	// Gateway
	copy(buf[offset:], (*[16]byte)(unsafe.Pointer(gw))[:gw.len])
	offset += roundUp(int(gw.len))

	// Netmask
	copy(buf[offset:], (*[16]byte)(unsafe.Pointer(mask))[:mask.len])

	// Send message
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	_, err := unix.Write(rm.socket, buf)
	if err != nil {
		operation := "add"
		if msgType == RTM_DELETE {
			operation = "delete"
		}
		log.Error("Failed to send route message", "error", err, "operation", operation, "network", network.String(), "gateway", gateway.String())
		return &RouteError{
			Type:    ErrSystemCall,
			Network: *network,
			Gateway: gateway,
			Cause:   fmt.Errorf("failed to send route message: %w", err),
		}
	}
	return nil
}

func ipToSockaddr(ip net.IP) *sockaddrInet {
	sa := &sockaddrInet{
		len:    16,
		family: unix.AF_INET,
		port:   0,
	}

	if ip4 := ip.To4(); ip4 != nil {
		copy(sa.addr[:], ip4)
	}

	return sa
}

func maskToSockaddr(mask net.IPMask) *sockaddrInet {
	sa := &sockaddrInet{
		len:    16,
		family: unix.AF_INET,
		port:   0,
	}

	if len(mask) == 4 {
		copy(sa.addr[:], mask)
	} else if len(mask) == 16 {
		// IPv6 mask, convert to IPv4 if possible
		copy(sa.addr[:], mask[12:])
	}

	return sa
}

func roundUp(size int) int {
	return (size + 3) &^ 3
}
