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
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
)

func mac(idx int) string {
	return fmt.Sprintf("00:00:5e:00:53:%02d", idx)
}

func req(id int) *dhcpv4.DHCPv4 {
	mac, err := net.ParseMAC(mac(id))
	if err != nil {
		panic(err)
	}

	req, err := dhcpv4.NewDiscovery(mac)
	if err != nil {
		panic(err)
	}

	return req
}

func request(t *testing.T, id int, requested, ciaddr string) *dhcpv4.DHCPv4 {
	t.Helper()

	hwAddr, err := net.ParseMAC(mac(id))
	require.NoError(t, err)

	mods := []dhcpv4.Modifier{dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest), dhcpv4.WithHwAddr(hwAddr)}
	if requested != "" {
		mods = append(mods, dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP(requested))))
	}
	if ciaddr != "" {
		mods = append(mods, dhcpv4.WithClientIP(net.ParseIP(ciaddr)))
	}

	req, err := dhcpv4.New(mods...)
	require.NoError(t, err)

	return req
}

// allocateIn allocates and records the entry the way a write would
func allocateIn(t *testing.T, subnet *dhcpapi.DHCPSubnet, req *dhcpv4.DHCPv4) (netip.Addr, error) {
	t.Helper()

	sp := newSubnetSpec(subnet)
	require.NoError(t, sp.err)

	ip, _, err := allocate(subnet, sp, nil, req)
	if err != nil {
		return netip.Addr{}, err
	}

	if subnet.Status.Allocated == nil {
		subnet.Status.Allocated = map[string]dhcpapi.DHCPAllocated{}
	}
	subnet.Status.Allocated[req.ClientHWAddr.String()] = dhcpapi.DHCPAllocated{IP: ip.String()}

	return ip, nil
}

func TestAllocateRequest(t *testing.T) {
	newSubnet := func() *dhcpapi.DHCPSubnet {
		return &dhcpapi.DHCPSubnet{
			Spec: dhcpapi.DHCPSubnetSpec{
				LeaseTimeSeconds: 3600,
				CIDRBlock:        "10.0.0.0/24",
				Gateway:          "10.0.0.1",
				StartIP:          "10.0.0.100",
				EndIP:            "10.0.0.199",
				Static: map[string]dhcpapi.DHCPSubnetStatic{
					mac(5): {IP: "10.0.0.222"},
				},
			},
			Status: dhcpapi.DHCPSubnetStatus{
				Allocated: map[string]dhcpapi.DHCPAllocated{
					mac(0): {IP: "10.0.0.100"},
					mac(1): {IP: "10.0.0.101"},
				},
			},
		}
	}

	for _, tt := range []struct {
		name string
		req  func(t *testing.T) *dhcpv4.DHCPv4
		want string // empty means NAK
	}{
		{
			name: "requested ip after offer",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 0, "10.0.0.100", "") },
			want: "10.0.0.100",
		},
		{
			name: "requested ip free in range",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 2, "10.0.0.150", "") },
			want: "10.0.0.150",
		},
		{
			name: "requested ip used by another mac",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 2, "10.0.0.101", "") },
		},
		{
			name: "requested ip outside of the range",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 2, "10.0.0.50", "") },
		},
		{
			name: "requested ip is the gateway",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 2, "10.0.0.1", "") },
		},
		{
			name: "requested ip is another mac's static",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 2, "10.0.0.222", "") },
		},
		{
			name: "renewal of own ip",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 1, "", "10.0.0.101") },
			want: "10.0.0.101",
		},
		{
			name: "renewal of an ip now used by another mac",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 2, "", "10.0.0.101") },
		},
		{
			name: "static mac requests its static ip outside of the range",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 5, "10.0.0.222", "") },
			want: "10.0.0.222",
		},
		{
			name: "static mac renews a dynamic ip",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 5, "", "10.0.0.150") },
		},
		{
			name: "request without requested ip and ciaddr falls back to own ip",
			req:  func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 1, "", "") },
			want: "10.0.0.101",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			subnet := newSubnet()

			got, err := allocateIn(t, subnet, tt.req(t))
			if tt.want == "" {
				require.ErrorIs(t, err, ErrRequestedIPUnavailable)
				assert.Equal(t, newSubnet().Status, subnet.Status, "nothing is recorded on NAK")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestAllocateDiscoverIgnoresUnavailableRequestedIP(t *testing.T) {
	subnet := &dhcpapi.DHCPSubnet{
		Spec: dhcpapi.DHCPSubnetSpec{
			LeaseTimeSeconds: 3600,
			CIDRBlock:        "10.0.0.0/24",
			Gateway:          "10.0.0.1",
			StartIP:          "10.0.0.100",
			EndIP:            "10.0.0.199",
		},
		Status: dhcpapi.DHCPSubnetStatus{
			Allocated: map[string]dhcpapi.DHCPAllocated{mac(0): {IP: "10.0.0.100"}},
		},
	}

	discover := req(1)
	discover.UpdateOption(dhcpv4.OptRequestedIPAddress(net.ParseIP("10.0.0.100")))

	got, err := allocateIn(t, subnet, discover)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.101", got.String())
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
		{
			name: "known IP",
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
					Allocated: map[string]dhcpapi.DHCPAllocated{
						mac(0): {
							IP: "10.0.0.101",
						},
					},
				},
			},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.101"),
				netip.MustParseAddr("10.0.0.100"),
				netip.MustParseAddr("10.0.0.102"),
			},
		},
		{
			name: "static IP",
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
					Static: map[string]dhcpapi.DHCPSubnetStatic{
						mac(0): {
							IP: "10.0.0.101",
						},
					},
				},
				Status: dhcpapi.DHCPSubnetStatus{
					Allocated: map[string]dhcpapi.DHCPAllocated{},
				},
			},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.101"),
				netip.MustParseAddr("10.0.0.100"),
				netip.MustParseAddr("10.0.0.102"),
			},
		},
		{
			name: "static IP out of range",
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
					Static: map[string]dhcpapi.DHCPSubnetStatic{
						mac(0): {
							IP: "10.0.0.222",
						},
					},
				},
				Status: dhcpapi.DHCPSubnetStatus{
					Allocated: map[string]dhcpapi.DHCPAllocated{},
				},
			},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.222"),
				netip.MustParseAddr("10.0.0.100"),
				netip.MustParseAddr("10.0.0.101"),
			},
		},
		{
			name: "static IP already allocated",
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
					Static: map[string]dhcpapi.DHCPSubnetStatic{
						mac(0): {
							IP: "10.0.0.122",
						},
					},
				},
				Status: dhcpapi.DHCPSubnetStatus{
					Allocated: map[string]dhcpapi.DHCPAllocated{
						mac(1): {
							IP: "10.0.0.122",
						},
					},
				},
			},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.100"),
				netip.MustParseAddr("10.0.0.122"),
				netip.MustParseAddr("10.0.0.101"),
			},
		},
		{
			name: "static IP already allocated outside the range",
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
					Static: map[string]dhcpapi.DHCPSubnetStatic{
						mac(0): {
							IP: "10.0.0.222",
						},
					},
				},
				Status: dhcpapi.DHCPSubnetStatus{
					Allocated: map[string]dhcpapi.DHCPAllocated{
						mac(1): {
							IP: "10.0.0.222",
						},
					},
				},
			},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.100"),
				// static IP will be allocated for requested MAC after outside the range allocation expires
				netip.MustParseAddr("10.0.0.101"),
				netip.MustParseAddr("10.0.0.102"),
			},
		},
		{
			name: "static and reserved IPs",
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
					Static: map[string]dhcpapi.DHCPSubnetStatic{
						mac(0): {
							IP: "10.0.0.101",
						},
					},
				},
				Status: dhcpapi.DHCPSubnetStatus{
					Allocated: map[string]dhcpapi.DHCPAllocated{
						mac(1): {
							IP: "10.0.0.102",
						},
					},
				},
			},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.101"),
				netip.MustParseAddr("10.0.0.102"),
				netip.MustParseAddr("10.0.0.100"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for idx, expectedIP := range tt.expected {
				allocatedIP, err := allocateIn(t, tt.subnet, req(idx))
				assert.NoError(t, err)
				assert.Equalf(t, expectedIP.String(), allocatedIP.String(), "ips for mac %s", mac(idx))
			}
		})
	}
}
