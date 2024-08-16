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
	"encoding/binary"
	"net"
	"sync"

	"github.com/bits-and-blooms/bitset"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/pkg/errors"
)

// This is a range of IP Addresses that is managed as a unit.
type ipv4range struct {
	Start   uint32
	End     uint32
	gateway uint32
	Mask    net.IPMask
	Count   uint32
	bitmap  *bitset.BitSet
	Options []dhcpv4.Option
	sync.RWMutex
}

func newIPv4Range(start, end, gateway net.IP, count uint32, prefixLen uint32) (*ipv4range, error) {
	if start.To4() == nil || end.To4() == nil {
		return nil, errors.Errorf("invalid IPv4 addresses given to create the range: [%s,%s]", start, end)
	}
	if binary.BigEndian.Uint32(start.To4()) > binary.BigEndian.Uint32(end.To4()) {
		return nil, errors.New("no IPs in the given range to allocate")
	}
	if prefixLen > 32 {
		return nil, errors.New("prefix Length must be less than 32")
	}

	r := &ipv4range{
		Start:   binary.BigEndian.Uint32(start.To4()),
		End:     binary.BigEndian.Uint32(end.To4()),
		Count:   count,
		gateway: binary.BigEndian.Uint32(gateway.To4()),
		Mask:    net.CIDRMask(int(prefixLen), 32),
		bitmap:  bitset.New(uint(binary.BigEndian.Uint32(end.To4()) - binary.BigEndian.Uint32(start.To4()) + 1)),
	}
	// This is a sanity check if we get a gatway ip in the middle of the range
	if binary.BigEndian.Uint32(gateway.To4()) > binary.BigEndian.Uint32(start.To4()) &&
		binary.BigEndian.Uint32(gateway.To4()) < binary.BigEndian.Uint32(end.To4()) {
		// Gatway is in the middle of the range allocate this ip before we move ahead
		offset, _ := r.toOffset(gateway)
		r.bitmap.Set(offset) // Reserve the gateway IP
	}

	if r.End-r.Start+1 != count {
		log.Errorf("Count %d,range %d", count, r.End-r.Start+1)

		return nil, errors.New("count does not match range")
	}

	return r, nil
}

func (r *ipv4range) GatewayIP() net.IP {
	r.RLock()
	defer r.RUnlock()
	val := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(val, r.gateway)

	return val
}

// Allocate allocates the next availabe IP in range
func (r *ipv4range) Allocate() (net.IPNet, error) {
	r.Lock()
	defer r.Unlock()

	return r.allocate()
}

// AllocateIP allocates a specific IP in the range if it is available. else return the next available IP
func (r *ipv4range) AllocateIP(ip net.IPNet) (net.IPNet, error) {
	mask := net.CIDRMask(32, 32)

	if ip.IP.To4() != nil {
		mask = ip.Mask
	}

	r.Lock()
	defer r.Unlock()
	offset, err := r.toOffset(ip.IP.To4())
	if err != nil {
		return net.IPNet{}, err
	}
	// first try to get the exact ip
	if !r.bitmap.Test(offset) {
		r.bitmap.Set(offset) // it's available so set it

		return net.IPNet{
			IP:   r.toIP(uint32(offset)), //nolint:gosec
			Mask: mask,
		}, nil
	}
	// Allocate the next available IP
	return r.allocate()
}

func (r *ipv4range) allocate() (net.IPNet, error) {
	var next uint
	// Then any available address
	avail, ok := r.bitmap.NextClear(0)
	if !ok {
		return net.IPNet{}, errors.New("no IPs in the range to allocate")
	}

	next = avail
	r.bitmap.Set(next)

	return net.IPNet{
		IP:   r.toIP(uint32(next)), //nolint:gosec
		Mask: r.Mask,
	}, nil
}

// Free release the ip address back to the range
func (r *ipv4range) Free(ip net.IPNet) error {
	r.Lock()
	defer r.Unlock()
	if ip.IP == nil {
		return errors.New("Zero ip address")
	}
	offset, err := r.toOffset(ip.IP.To4())
	if err != nil {
		return errors.Wrapf(err, "invalid ip address %s", ip.IP.String())
	}
	if !r.bitmap.Test(offset) {
		return errors.New("ip address is not allocated in this range")
	}

	r.bitmap.Clear(offset) // IP released

	return nil
}

func (r *ipv4range) toIP(offset uint32) net.IP {
	if offset > r.End-r.Start {
		return net.IPv4zero
	}
	ip := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(ip, r.Start+offset)

	return ip
}

func (r *ipv4range) toOffset(ip net.IP) (uint, error) {
	ipaddr := binary.BigEndian.Uint32(ip.To4())
	if ipaddr < r.Start || ipaddr > r.End {
		return 0, errors.New("IP address out of range")
	}

	return uint(ipaddr - r.Start), nil
}
