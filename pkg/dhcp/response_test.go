// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/boot/server"
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

const pxeTFTPURL = "tftp://10.0.0.5/pxe/boot.efi"

func respondSpec() dhcpapi.DHCPSubnetSpec {
	return dhcpapi.DHCPSubnetSpec{
		CIDRBlock:        "10.0.0.0/24",
		Gateway:          "10.0.0.1",
		StartIP:          "10.0.0.100",
		EndIP:            "10.0.0.199",
		LeaseTimeSeconds: 3600,
		DNSServers:       []string{"10.0.0.2", "1.1.1.1"},
		TimeServers:      []string{"10.0.0.3"},
		InterfaceMTU:     9036,
		ZTPBaseURL:       "http://172.30.0.1:32000",
	}
}

type respondCase struct {
	name string
	spec func(spec *dhcpapi.DHCPSubnetSpec)
	req  []dhcpv4.Modifier
	want func(t *testing.T, resp *dhcpv4.DHCPv4)
}

func respondCases() []respondCase {
	vip := net.ParseIP("172.30.0.1").To4()
	relayed := dhcpv4.WithOption(dhcpv4.OptRelayAgentInfo(
		dhcpv4.Option{Code: dhcpv4.AgentCircuitIDSubOption, Value: dhcpv4.String("vlan1000")},
	))
	wants := dhcpv4.WithRequestedOptions
	route := func(dst, gw string) *dhcpv4.Route {
		prefix := netip.MustParsePrefix(dst)

		return &dhcpv4.Route{Dest: prefixIPNet(prefix), Router: net.ParseIP(gw).To4()}
	}

	return []respondCase{
		{
			name: "l2 with dns, ntp and mtu",
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, "10.0.0.100", resp.YourIPAddr.String())
				assert.Equal(t, time.Hour, resp.IPAddressLeaseTime(0))
				assert.Equal(t, []net.IP{net.ParseIP("10.0.0.1").To4()}, resp.Router())
				assert.Equal(t, vip, resp.ServerIdentifier())
				assert.Equal(t, net.CIDRMask(24, 32), resp.SubnetMask())
				assert.Equal(t, []net.IP{net.ParseIP("10.0.0.2").To4(), net.ParseIP("1.1.1.1").To4()}, resp.DNS())
				assert.Equal(t, []net.IP{net.ParseIP("10.0.0.3").To4()}, resp.NTPServers())
				assert.Equal(t, []byte{0x23, 0x4c}, resp.Options.Get(dhcpv4.OptionInterfaceMTU))
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionClasslessStaticRoute))
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionURL))
			},
		},
		{
			name: "no dns, ntp, mtu and default lease",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) {
				spec.DNSServers, spec.TimeServers, spec.InterfaceMTU, spec.LeaseTimeSeconds = nil, nil, 0, 0
			},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, 10*time.Minute, resp.IPAddressLeaseTime(0))
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionDomainNameServer))
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionNTPServers))
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionInterfaceMTU))
			},
		},
		{
			name: "l3 first request gets a short lease and /32",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) { spec.L3Mode = true },
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, 10*time.Second, resp.IPAddressLeaseTime(0))
				assert.Equal(t, net.CIDRMask(32, 32), resp.SubnetMask())
			},
		},
		{
			name: "l3 renewal gets the configured lease",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) { spec.L3Mode = true },
			req:  []dhcpv4.Modifier{dhcpv4.WithClientIP(net.ParseIP("10.0.0.100"))},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, time.Hour, resp.IPAddressLeaseTime(0))
			},
		},
		{
			name: "advertised routes requested replace the router",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) {
				spec.AdvertisedRoutes = []dhcpapi.DHCPRoute{{Destination: "192.168.0.0/16", Gateway: "10.0.0.4"}}
			},
			req: []dhcpv4.Modifier{wants(dhcpv4.OptionClasslessStaticRoute)},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Nil(t, resp.Router())
				assert.Equal(t, []*dhcpv4.Route{route("0.0.0.0/0", "10.0.0.1"), route("192.168.0.0/16", "10.0.0.4")},
					resp.ClasslessStaticRoute())
			},
		},
		{
			name: "advertised routes not requested",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) {
				spec.AdvertisedRoutes = []dhcpapi.DHCPRoute{{Destination: "192.168.0.0/16", Gateway: "10.0.0.4"}}
			},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, []net.IP{net.ParseIP("10.0.0.1").To4()}, resp.Router())
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionClasslessStaticRoute))
			},
		},
		{
			name: "routes requested without advertised routes keep the router",
			req:  []dhcpv4.Modifier{wants(dhcpv4.OptionClasslessStaticRoute)},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, []net.IP{net.ParseIP("10.0.0.1").To4()}, resp.Router())
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionClasslessStaticRoute))
			},
		},
		{
			name: "l3 without default route advertises the subnet",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) { spec.L3Mode, spec.DisableDefaultRoute = true, true },
			req:  []dhcpv4.Modifier{wants(dhcpv4.OptionClasslessStaticRoute)},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Nil(t, resp.Router())
				assert.Equal(t, []*dhcpv4.Route{route("10.0.0.0/24", "10.0.0.1")}, resp.ClasslessStaticRoute())
			},
		},
		{
			name: "l2 without default route",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) { spec.DisableDefaultRoute = true },
			req:  []dhcpv4.Modifier{wants(dhcpv4.OptionClasslessStaticRoute)},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Nil(t, resp.Router())
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionClasslessStaticRoute))
			},
		},
		{
			name: "onie",
			req:  []dhcpv4.Modifier{dhcpv4.WithOption(dhcpv4.OptClassIdentifier(onieClassIDPrefix + "x86_64-accton"))},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, []byte("http://172.30.0.1:32000"+nosinstall.OnieURLSuffix), resp.Options.Get(dhcpv4.OptionURL))
			},
		},
		{
			name: "cumulus",
			req:  []dhcpv4.Modifier{dhcpv4.WithOption(dhcpv4.OptClassIdentifier(cumulusClassID))},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, []byte("http://172.30.0.1:32000"+server.CumulusZTPURLSuffix), resp.Options.Get(dhcpv4.GenericOptionCode(239)))
			},
		},
		{
			name: "pxe over http",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) { spec.PXEURL = "http://10.0.0.5:8080/boot.efi" },
			req: []dhcpv4.Modifier{relayed, wants(dhcpv4.OptionTFTPServerName, dhcpv4.OptionBootfileName),
				dhcpv4.WithOption(dhcpv4.OptClassIdentifier("HTTPClient"))},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, "10.0.0.5:8080", resp.TFTPServerName())
				assert.Equal(t, "http://10.0.0.5:8080/boot.efi", resp.BootFileName)
				assert.Equal(t, "HTTPClient", resp.ClassIdentifier())
				assert.Equal(t, vip, resp.ServerIdentifier())
			},
		},
		{
			name: "pxe over tftp",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) { spec.PXEURL = pxeTFTPURL },
			req:  []dhcpv4.Modifier{relayed, wants(dhcpv4.OptionTFTPServerName, dhcpv4.OptionBootfileName)},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Equal(t, "10.0.0.5", resp.TFTPServerName())
				assert.Equal(t, "pxe/boot.efi", resp.BootFileNameOption())
				assert.Equal(t, net.ParseIP("10.0.0.5").To4(), resp.ServerIdentifier())
			},
		},
		{
			name: "pxe not requested",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) { spec.PXEURL = pxeTFTPURL },
			req:  []dhcpv4.Modifier{relayed},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionTFTPServerName))
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionBootfileName))
			},
		},
		{
			name: "pxe isn't sent to unrelayed clients",
			spec: func(spec *dhcpapi.DHCPSubnetSpec) { spec.PXEURL = pxeTFTPURL },
			req:  []dhcpv4.Modifier{wants(dhcpv4.OptionTFTPServerName, dhcpv4.OptionBootfileName)},
			want: func(t *testing.T, resp *dhcpv4.DHCPv4) {
				t.Helper()
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionTFTPServerName))
				assert.Nil(t, resp.Options.Get(dhcpv4.OptionBootfileName))
			},
		},
	}
}

func respondRequest(t *testing.T, tc respondCase) (*dhcpapi.DHCPSubnet, *dhcpv4.DHCPv4, *dhcpv4.DHCPv4) {
	t.Helper()

	obj := &dhcpapi.DHCPSubnet{Spec: respondSpec()}
	if tc.spec != nil {
		tc.spec(&obj.Spec)
	}

	hwAddr, err := net.ParseMAC(mac(0))
	require.NoError(t, err)
	// without a parameter request list all options count as requested
	mods := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(hwAddr),
		dhcpv4.WithRequestedOptions(dhcpv4.OptionSubnetMask, dhcpv4.OptionRouter, dhcpv4.OptionDomainNameServer),
	}
	req, err := dhcpv4.New(append(mods, tc.req...)...)
	require.NoError(t, err)
	resp, err := dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)

	return obj, req, resp
}

func TestRespond(t *testing.T) {
	vip := net.ParseIP("172.30.0.1").To4()

	for _, tc := range respondCases() {
		t.Run(tc.name, func(t *testing.T) {
			obj, req, resp := respondRequest(t, tc)

			sp := newSubnetSpec(obj)
			require.NoError(t, sp.err)
			sp.respond(req, resp, netip.MustParseAddr("10.0.0.100"), vip)

			tc.want(t, resp)
		})
	}
}
