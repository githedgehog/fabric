package dhcpd

import "net"

type IPv4Allocator interface {
	AllocateIP(hint net.IPNet) (net.IPNet, error)
	Allocate() (net.IPNet, error)
	Free(net.IPNet) error
	GatewayIP() net.IP
}
