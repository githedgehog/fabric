// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandler4Drops(t *testing.T) {
	mac, err := net.ParseMAC("0c:20:12:fe:00:01")
	require.NoError(t, err)

	for _, tt := range []struct {
		name    string
		giaddr  string
		subnets map[string]*dhcpapi.DHCPSubnet
	}{
		{
			name:   "unknown giaddr",
			giaddr: "192.168.99.1",
		},
		{
			name:   "no subnet",
			giaddr: "0.0.0.0",
		},
		{
			name:   "management subnet for a non-onie device",
			giaddr: "0.0.0.0",
			subnets: map[string]*dhcpapi.DHCPSubnet{
				subnetKey("", ""): {ObjectMeta: kmetav1.ObjectMeta{Name: dhcpapi.ManagementSubnet}},
			},
		},
		{
			name:   "handling error",
			giaddr: "0.0.0.0",
			subnets: map[string]*dhcpapi.DHCPSubnet{
				subnetKey("", ""): {
					ObjectMeta: kmetav1.ObjectMeta{Name: "broken"},
					Spec: dhcpapi.DHCPSubnetSpec{
						CIDRBlock: "10.0.0.0/24",
						StartIP:   "not-an-ip",
						EndIP:     "10.0.0.250",
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			subnets := map[string]*subnet{}
			for key, obj := range tt.subnets {
				subnets[key] = newSubnet(obj)
			}
			s := &Server{
				subnets:        subnets,
				relayAllowlist: map[string]struct{}{"10.0.0.1": {}},
				controlVIP:     net.ParseIP("172.30.0.1"),
			}

			req, err := dhcpv4.NewDiscovery(mac)
			require.NoError(t, err)
			req.GatewayIPAddr = net.ParseIP(tt.giaddr)
			resp, err := dhcpv4.NewReplyFromRequest(req)
			require.NoError(t, err)

			handler, err := s.setupDHCP4Plugin(t.Context())()
			require.NoError(t, err)

			got, stop := handler(req, resp)
			assert.Nil(t, got, "request should be dropped instead of replying")
			assert.True(t, stop)
		})
	}
}
