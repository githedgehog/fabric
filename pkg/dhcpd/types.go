package dhcpd

import (
	"net"
	"sync"
	"time"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1alpha2"
)

// type rangeRecord struct {
// 	StartIP net.IP
// 	EndIP   net.IP
// 	//count     int
// 	Subnet    string
// 	Gateway   net.IP
// 	CIDRBlock net.IPNet
// 	VRF       string
// 	CircuitID string
// 	records   []*allocationRecord
// }

// type allocationRecord struct {
// 	IP         net.IP
// 	MacAddress string
// 	Hostname   string
// 	Expiry     time.Time
// }

// type persistentBackend struct {
// 	subnets map[string]*rangeRecord // This is temporary and we should be using a kubernetes backend
// }

var leaseTime = time.Duration(3600 * time.Second)
var pendingDiscoverTimeout = time.Duration(5000 * time.Millisecond)

// type allocations struct {
// 	pool IPv4Allocator
// 	// Offers that have been made but we have not seen a request for. ip->mac address. This is temporary
// 	// while we wait for dhcprequest. Sync to kubernetes backend and destroy this state.
// 	ipReservations *ipallocations
// 	sync.RWMutex
// }

type reservationState uint32

const (
	unassigned reservationState = iota
	pending    reservationState = 1
	committed  reservationState = 2
)

type ipreservation struct {
	address    net.IPNet
	MacAddress string
	expiry     time.Time
	Hostname   string
	state      reservationState
}

type ipallocations struct {
	allocation map[string]*ipreservation
}

type updateBackend struct {
	IP         string
	MacAddress string
	Expiry     time.Time
	Hostname   string
	Vrf        string
	circuitID  string
}
type pluginState struct {
	dhcpSubnets *DHCPSubnets
	svcHdl      *Service
}

type DHCPSubnets struct {
	subnets map[string]*ManagedSubnet
	sync.RWMutex
}

type ManagedSubnet struct {
	dhcpSubnet  *dhcpapi.DHCPSubnet
	pool        IPv4Allocator
	allocations *ipallocations
	sync.RWMutex
}
