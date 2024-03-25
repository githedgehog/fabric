// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package dhcpd

import (
	"net"
	"sync"
	"time"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1alpha2"
)

var (
	leaseTime              = 3600 * time.Second
	pendingDiscoverTimeout = 5000 * time.Millisecond
)

type reservationState uint32

const (
	unassigned reservationState = iota
	pending    reservationState = 1
	committed  reservationState = 2
)

type ipreservation struct {
	address    net.IPNet
	macAddress string
	expiry     time.Time
	hostname   string
	state      reservationState
}

type ipallocations struct {
	allocation map[string]*ipreservation
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
