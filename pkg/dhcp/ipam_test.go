// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"fmt"
	"net"
	"net/netip"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
)

func req(id int) *dhcpv4.DHCPv4 {
	mac, err := net.ParseMAC(fmt.Sprintf("00:00:5e:00:53:%02d", id))
	if err != nil {
		panic(err)
	}

	req, err := dhcpv4.NewDiscovery(mac)
	if err != nil {
		panic(err)
	}

	return req
}

func TestAllocate(t *testing.T) {
	for _, tt := range []struct {
		name     string
		subnet   *dhcpapi.DHCPSubnet
		expected []netip.Addr
		error    bool
	}{
		{
			name: "allocate IP",
			subnet: &dhcpapi.DHCPSubnet{
				Spec: dhcpapi.DHCPSubnetSpec{
					LeaseTimeSeconds: 60,
					Subnet:           "default",
					CIDRBlock:        "10.0.0.0/24",
					Gateway:          "10.0.0.1",
					StartIP:          "10.0.0.100",
					EndIP:            "10.0.0.199",
					VRF:              "test",
					CircuitID:        "test",
				},
				Status: dhcpapi.DHCPSubnetStatus{
					Allocated: map[string]dhcpapi.DHCPAllocated{},
				},
			},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.100"),
				netip.MustParseAddr("10.0.0.101"),
				netip.MustParseAddr("10.0.0.102"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for idx, expectedIP := range tt.expected {
				allocatedIP, err := allocate(tt.subnet, req(idx))
				assert.NoError(t, err)
				assert.Equal(t, expectedIP.String(), allocatedIP.String())
			}
		})
	}
}
