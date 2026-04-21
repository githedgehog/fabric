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

package iputil

import (
	"net"
	"net/netip"

	cidrlib "github.com/apparentlymart/go-cidr/cidr"
	"github.com/pkg/errors"
)

type ParsedCIDR struct {
	IP             net.IP
	Subnet         net.IPNet
	Gateway        net.IP
	DHCPRangeStart net.IP
	DHCPRangeEnd   net.IP
}

func ParseCIDR(cidr string) (*ParsedCIDR, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse cidr %s", cidr)
	}

	_, last, err := Range(cidr)
	if err != nil {
		return nil, err
	}

	return &ParsedCIDR{
		IP:             ip,
		Subnet:         *ipNet,
		Gateway:        cidrlib.Inc(ipNet.IP),
		DHCPRangeStart: cidrlib.Inc(cidrlib.Inc(ipNet.IP)),
		DHCPRangeEnd:   net.ParseIP(last),
	}, nil
}

func Range(cidr string) (string, string, error) {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to parse cidr %s", cidr)
	}

	first, last := cidrlib.AddressRange(subnet)

	return first.String(), last.String(), nil
}

func VerifyNoOverlap(subnets []*net.IPNet) error {
	anyNetCIDR := "0.0.0.0/0"
	_, anyNet, err := net.ParseCIDR(anyNetCIDR)
	if err != nil {
		return errors.Wrapf(err, "failed to parse cidr %s", anyNetCIDR)
	}

	return errors.Wrapf(cidrlib.VerifyNoOverlap(subnets, anyNet), "failed to verify no overlap subnets")
}

func LastIP(ipNet *net.IPNet) *net.IPNet {
	last := make(net.IP, len(ipNet.IP))
	for i := range last {
		last[i] = ipNet.IP[i] | (^ipNet.Mask[i])
	}

	return &net.IPNet{IP: last, Mask: ipNet.Mask}
}

// IsSubset reports whether inner is contained in outer with a prefix
// length at least as specific as outer's. A check like net.IPNet.Contains on
// the network address alone does not suffice: a wider inner prefix can land
// inside a narrower outer one and still span addresses outside outer.
func IsSubset(inner, outer netip.Prefix) bool {
	inner = inner.Masked()
	outer = outer.Masked()

	return inner.Addr().Is4() == outer.Addr().Is4() &&
		outer.Contains(inner.Addr()) &&
		inner.Bits() >= outer.Bits()
}

// LastIPNetip returns the last address of a prefix as a netip.Addr.
func LastIPNetip(prefix netip.Prefix) netip.Addr {
	addr := prefix.Masked().Addr()
	bits := prefix.Bits()

	if addr.Is4() {
		b := addr.As4()
		for i := range b {
			// shift = how many of this byte's bits are still network bits.
			// shift<=0: whole byte is host bits → set all to 1.
			// shift<8:  byte straddles the boundary → fill only the host portion.
			// shift>=8: whole byte is network bits → already correct, do nothing.
			if shift := bits - i*8; shift <= 0 {
				b[i] = 0xff
			} else if shift < 8 {
				b[i] |= byte(0xff >> shift) // mask of (8-shift) low bits
			}
		}

		return netip.AddrFrom4(b)
	}

	b := addr.As16()
	for i := range b {
		if shift := bits - i*8; shift <= 0 {
			b[i] = 0xff
		} else if shift < 8 {
			b[i] |= byte(0xff >> shift)
		}
	}

	result := netip.AddrFrom16(b)
	if zone := addr.Zone(); zone != "" {
		result = result.WithZone(zone)
	}

	return result
}

func VerifyNoOverlapNetip(subnets []netip.Prefix) error {
	for i, first := range subnets {
		for _, second := range subnets[i+1:] {
			if first.Overlaps(second) {
				return errors.Errorf("subnets %s and %s overlap", first, second)
			}
		}
	}

	return nil
}
