// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"net"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandler4NAK(t *testing.T) {
	obj := testLeaseSubnet()
	obj.Status.Allocated = map[string]dhcpapi.DHCPAllocated{mac(9): {IP: "10.0.0.100"}}
	vip := net.ParseIP("172.30.0.1").To4()
	s := &Server{
		Config:     *testCfg,
		subnets:    map[string]*subnet{subnetKey("", ""): newSubnet(obj)},
		controlVIP: vip,
	}

	req := request(t, 0, "10.0.0.100", "")
	resp, err := dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)

	handler, err := s.setupDHCP4Plugin(t.Context())()
	require.NoError(t, err)

	got, _ := handler(req, resp)
	require.NotNil(t, got)
	assert.Equal(t, dhcpv4.MessageTypeNak, got.MessageType())
	assert.True(t, got.YourIPAddr.IsUnspecified())
	assert.Equal(t, vip, got.ServerIdentifier())
	assert.Len(t, s.subnets[subnetKey("", "")].obj.Load().Status.Allocated, 1, "nothing is recorded on NAK")
}

func TestHandler4Timeout(t *testing.T) {
	cfg := *testCfg
	cfg.RequestTimeout.Duration = time.Nanosecond
	s := &Server{
		Config:     cfg,
		subnets:    map[string]*subnet{subnetKey("", ""): newSubnet(testLeaseSubnet())},
		controlVIP: net.ParseIP("172.30.0.1").To4(),
	}

	handler, err := s.setupDHCP4Plugin(t.Context())()
	require.NoError(t, err)

	req := request(t, 0, "", "")
	resp, err := dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)

	got, stop := handler(req, resp)
	assert.Nil(t, got, "request should be dropped instead of replying")
	assert.True(t, stop)
	assert.EqualValues(t, 1, s.budget.expired.Load())
}

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
				Config:         *testCfg,
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
