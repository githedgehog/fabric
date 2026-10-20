// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerIdentifier(t *testing.T) {
	vip := net.ParseIP("172.30.0.1").To4()
	s := &Server{controlVIP: vip}

	got, err := s.serverIdentifier(net.ParseIP("172.30.8.1"))
	require.NoError(t, err)
	assert.Equal(t, vip, got)
}

func TestNAKResponse(t *testing.T) {
	vip := net.ParseIP("172.30.0.1").To4()

	for _, tt := range []struct {
		name      string
		giaddr    string
		broadcast bool
	}{
		{"unrelayed", "0.0.0.0", false},
		{"relayed is broadcast by the relay", "172.30.8.1", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := request(t, 0, "10.0.0.100", "10.0.0.100")
			request.GatewayIPAddr = net.ParseIP(tt.giaddr)
			resp, err := dhcpv4.NewReplyFromRequest(request)
			require.NoError(t, err)

			nakResponse(request, resp, vip)

			assert.Equal(t, dhcpv4.MessageTypeNak, resp.MessageType())
			assert.Equal(t, vip, resp.ServerIdentifier())
			assert.True(t, resp.YourIPAddr.IsUnspecified())
			assert.True(t, resp.ClientIPAddr.IsUnspecified())
			assert.Equal(t, tt.broadcast, resp.IsBroadcast())
			assert.Nil(t, resp.Options.Get(dhcpv4.OptionIPAddressLeaseTime))
		})
	}
}

func TestUpdateResponseServerIdentifier(t *testing.T) {
	sn := testLeaseSubnet()
	vip := net.ParseIP("172.30.0.1").To4()

	request := req(0)
	resp, err := dhcpv4.NewReplyFromRequest(request)
	require.NoError(t, err)

	_, ipNet, err := net.ParseCIDR(sn.Spec.CIDRBlock)
	require.NoError(t, err)
	ipNet.IP = net.ParseIP("10.0.0.100").To4()

	require.NoError(t, updateResponse(request, resp, sn, ipNet, vip))
	assert.Equal(t, vip, resp.ServerIdentifier())
	assert.Equal(t, ipNet.IP, resp.YourIPAddr)
}
