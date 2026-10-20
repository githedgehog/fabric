// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
)

func validSpec() dhcpapi.DHCPSubnetSpec {
	return dhcpapi.DHCPSubnetSpec{
		CIDRBlock:        "10.0.1.5/24",
		Gateway:          "10.0.1.1",
		StartIP:          "10.0.1.10",
		EndIP:            "10.0.1.99",
		LeaseTimeSeconds: 3600,
		DNSServers:       []string{"1.1.1.1"},
		TimeServers:      []string{"10.0.1.2"},
		InterfaceMTU:     9036,
		AdvertisedRoutes: []dhcpapi.DHCPRoute{{Destination: "192.168.5.7/16", Gateway: "10.0.1.3"}},
		PXEURL:           "tftp://10.0.1.4/boot.efi",
		ZTPBaseURL:       "http://172.30.0.1:32000",
		Static:           map[string]dhcpapi.DHCPSubnetStatic{"AA:BB:CC:DD:EE:01": {IP: "10.0.1.200"}},
	}
}

func TestDHCPSubnetParse(t *testing.T) {
	spec := validSpec()
	parsed, err := spec.Parse()
	require.NoError(t, err)

	assert.Equal(t, netip.MustParsePrefix("10.0.1.0/24"), parsed.Prefix)
	assert.Equal(t, netip.MustParseAddr("10.0.1.1"), parsed.Gateway)
	assert.Equal(t, netip.MustParseAddr("10.0.1.10"), parsed.Start)
	assert.Equal(t, netip.MustParseAddr("10.0.1.99"), parsed.End)
	assert.Equal(t, []netip.Addr{netip.MustParseAddr("1.1.1.1")}, parsed.DNSServers)
	assert.Equal(t, []netip.Addr{netip.MustParseAddr("10.0.1.2")}, parsed.TimeServers)
	assert.Equal(t, []dhcpapi.DHCPRouteParsed{{
		Destination: netip.MustParsePrefix("192.168.0.0/16"),
		Gateway:     netip.MustParseAddr("10.0.1.3"),
	}}, parsed.AdvertisedRoutes)
	require.NotNil(t, parsed.PXEURL)
	assert.Equal(t, "10.0.1.4", parsed.PXEURL.Host)

	require.NoError(t, spec.Validate())
}

func TestDHCPSubnetParseInvalid(t *testing.T) {
	for _, tt := range []struct {
		name string
		mod  func(spec *dhcpapi.DHCPSubnetSpec)
	}{
		{"bad cidr", func(s *dhcpapi.DHCPSubnetSpec) { s.CIDRBlock = "10.0.1.0" }},
		{"ipv6 cidr", func(s *dhcpapi.DHCPSubnetSpec) { s.CIDRBlock = "fd00::/64" }},
		{"no gateway", func(s *dhcpapi.DHCPSubnetSpec) { s.Gateway = "" }},
		{"gateway outside", func(s *dhcpapi.DHCPSubnetSpec) { s.Gateway = "10.0.2.1" }},
		{"bad start", func(s *dhcpapi.DHCPSubnetSpec) { s.StartIP = "10.0.1" }},
		{"end outside", func(s *dhcpapi.DHCPSubnetSpec) { s.EndIP = "10.0.2.99" }},
		{"start after end", func(s *dhcpapi.DHCPSubnetSpec) { s.StartIP, s.EndIP = s.EndIP, s.StartIP }},
		{"bad dns", func(s *dhcpapi.DHCPSubnetSpec) { s.DNSServers = []string{"dns.local"} }},
		{"ipv6 dns", func(s *dhcpapi.DHCPSubnetSpec) { s.DNSServers = []string{"fd00::1"} }},
		{"bad ntp", func(s *dhcpapi.DHCPSubnetSpec) { s.TimeServers = []string{""} }},
		{"mtu too small", func(s *dhcpapi.DHCPSubnetSpec) { s.InterfaceMTU = 95 }},
		{"mtu too large", func(s *dhcpapi.DHCPSubnetSpec) { s.InterfaceMTU = 9037 }},
		{"bad route destination", func(s *dhcpapi.DHCPSubnetSpec) { s.AdvertisedRoutes[0].Destination = "192.168.0.0" }},
		{"ipv6 route destination", func(s *dhcpapi.DHCPSubnetSpec) { s.AdvertisedRoutes[0].Destination = "fd00::/8" }},
		{"route gateway outside", func(s *dhcpapi.DHCPSubnetSpec) { s.AdvertisedRoutes[0].Gateway = "10.0.2.3" }},
		{"pxe url without scheme", func(s *dhcpapi.DHCPSubnetSpec) { s.PXEURL = "10.0.1.4/boot.efi" }},
		{"bad pxe url", func(s *dhcpapi.DHCPSubnetSpec) { s.PXEURL = "http://[::1" }},
		{"ztp url without host", func(s *dhcpapi.DHCPSubnetSpec) { s.ZTPBaseURL = "http:///onie" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := validSpec()
			tt.mod(&spec)

			_, err := spec.Parse()
			require.Error(t, err)
			require.Error(t, spec.Validate())
		})
	}
}

func TestDHCPSubnetParseStatics(t *testing.T) {
	spec := validSpec()
	spec.Static = map[string]dhcpapi.DHCPSubnetStatic{
		"AA:BB:CC:DD:EE:01": {IP: "10.0.1.200"},
		"aa:bb:cc:dd:ee:02": {IP: "10.0.1.201"},
		"aa:bb:cc:dd:ee:03": {IP: "10.0.1.201"}, // same IP as 02
		"aa:bb:cc:dd:ee:04": {IP: "10.0.1.0"},   // subnet
		"aa:bb:cc:dd:ee:05": {IP: "10.0.1.1"},   // gateway
		"aa:bb:cc:dd:ee:06": {IP: "10.0.1.255"}, // broadcast
		"aa:bb:cc:dd:ee:07": {IP: "10.0.2.7"},   // outside
		"aa:bb:cc:dd:ee:08": {IP: "not-an-ip"},
		"aa:bb:cc:dd:ee:09": {IP: ""},
		"not-a-mac":         {IP: "10.0.1.202"},
		"aa-bb-cc-dd-ee-01": {IP: "10.0.1.203"}, // same MAC as the first one
	}

	parsed, err := spec.Parse()
	require.NoError(t, err)

	statics, errs := parsed.ParseStatics(spec.Static)
	assert.Equal(t, map[string]netip.Addr{
		"aa:bb:cc:dd:ee:01": netip.MustParseAddr("10.0.1.200"),
		"aa:bb:cc:dd:ee:02": netip.MustParseAddr("10.0.1.201"),
	}, statics)
	assert.Len(t, errs, 9)

	require.Error(t, spec.Validate())
}
