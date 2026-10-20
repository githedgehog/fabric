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
