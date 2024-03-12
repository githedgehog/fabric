package dhcpd

import (
	"encoding/binary"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.githedgehog.com/fabric/api/dhcp/v1alpha2"
)

func Test_handleDiscover4(t *testing.T) {
	type args struct {
		req  *dhcpv4.DHCPv4
		resp *dhcpv4.DHCPv4
	}
	tests := []struct {
		name          string
		args          args
		wantErr       bool
		expectedState func() bool
	}{
		{
			name: "Test a DHCP request with no relayAgentInfo",
			args: args{req: func() *dhcpv4.DHCPv4 {
				if pluginHdl == nil {
					pluginHdl = &pluginState{
						dhcpSubnets: &DHCPSubnets{
							subnets: map[string]*ManagedSubnet{},
						},
						//svcHdl: svc,
					}
				}
				pool, _ := NewIPv4Range(
					net.ParseIP("10.10.1.10"),
					net.ParseIP("10.10.1.240"),
					net.ParseIP("10.10.1.1"),
					binary.BigEndian.Uint32(net.ParseIP("10.10.1.240").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
					uint32(24),
				)
				pluginHdl.dhcpSubnets.subnets["VrfV12"+"Vlan2000"] = &ManagedSubnet{
					dhcpSubnet: &v1alpha2.DHCPSubnet{
						Spec: v1alpha2.DHCPSubnetSpec{},
					},
					pool: pool,
					allocations: &ipallocations{
						allocation: make(map[string]*ipreservation),
					},
				}
				hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
				req, _ := dhcpv4.NewDiscovery(hardwareAddress)

				return req
			}(),
				resp: func() *dhcpv4.DHCPv4 {
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '2',
					}))
					resp, _ := dhcpv4.NewReplyFromRequest(req)
					return resp
				}()},
			wantErr: false,
			expectedState: func() bool {

				return true
			},
		},
		{
			name: "Test a DHCP request with relayAgentInfo and subnet available",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					if pluginHdl == nil {
						pluginHdl = &pluginState{
							dhcpSubnets: &DHCPSubnets{
								subnets: map[string]*ManagedSubnet{},
							},
							//svcHdl: svc,
						}
					}
					pool, _ := NewIPv4Range(
						net.ParseIP("10.10.1.10"),
						net.ParseIP("10.10.1.240"),
						net.ParseIP("10.10.1.1"),
						binary.BigEndian.Uint32(net.ParseIP("10.10.1.240").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
						uint32(24),
					)
					pluginHdl.dhcpSubnets.subnets["VrfV13"+"Vlan2000"] = &ManagedSubnet{
						dhcpSubnet: &v1alpha2.DHCPSubnet{
							Spec: v1alpha2.DHCPSubnetSpec{},
						},
						pool: pool,
						allocations: &ipallocations{
							allocation: make(map[string]*ipreservation),
						},
					}
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '3',
					}))

					return req
				}(),
				resp: func() *dhcpv4.DHCPv4 {
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '3',
					}))
					resp, _ := dhcpv4.NewReplyFromRequest(req)
					return resp
				}(),
			},
			wantErr: false,
			expectedState: func() bool {
				if val, ok := pluginHdl.dhcpSubnets.subnets["VrfV13"+"Vlan2000"]; ok {
					if len(val.allocations.allocation) == 1 {
						return true
					}
				}
				return false
			},
		},
		{
			name: "Subnet is not populated for vrf+vlan",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					if pluginHdl == nil {
						pluginHdl = &pluginState{
							dhcpSubnets: &DHCPSubnets{
								subnets: map[string]*ManagedSubnet{},
							},
							//svcHdl: svc,
						}
					}
					// pool, _ := NewIPv4Range(
					// 	net.ParseIP("10.10.1.10"),
					// 	net.ParseIP("10.10.1.240"),
					// 	net.ParseIP("10.10.1.1"),
					// 	binary.BigEndian.Uint32(net.ParseIP("10.10.1.240").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
					// 	uint32(24),
					// )
					// pluginHdl.dhcpSubnets.subnets["VrfV14"+"Vlan2000"] = &ManagedSubnet{
					// 	dhcpSubnet: &v1alpha2.DHCPSubnet{
					// 		Spec: v1alpha2.DHCPSubnetSpec{},
					// 	},
					// 	pool: pool,
					// 	allocations: &ipallocations{
					// 		allocation: make(map[string]*ipreservation),
					// 	},
					// }
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '3',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '4',
					}))

					return req
				}(),
				resp: func() *dhcpv4.DHCPv4 {
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '4',
					}))
					resp, _ := dhcpv4.NewReplyFromRequest(req)
					return resp
				}(),
			},
			wantErr: true,
			expectedState: func() bool {
				return true
			},
		},
		{
			name: "Test a DHCP request with relayAgentInfo and subnet available and there is an existing reservation",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					if pluginHdl == nil {
						pluginHdl = &pluginState{
							dhcpSubnets: &DHCPSubnets{
								subnets: map[string]*ManagedSubnet{},
							},
							//svcHdl: svc,
						}
					}
					pool, _ := NewIPv4Range(
						net.ParseIP("10.10.1.10"),
						net.ParseIP("10.10.1.240"),
						net.ParseIP("10.10.1.1"),
						binary.BigEndian.Uint32(net.ParseIP("10.10.1.240").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
						uint32(24),
					)
					pluginHdl.dhcpSubnets.subnets["VrfV15"+"Vlan2000"] = &ManagedSubnet{
						dhcpSubnet: &v1alpha2.DHCPSubnet{
							Spec: v1alpha2.DHCPSubnetSpec{},
						},
						pool: pool,
						allocations: &ipallocations{
							allocation: map[string]*ipreservation{
								"00:00:00:00:00:01": &ipreservation{
									address:    net.IPNet{IP: net.ParseIP("10.10.1.10"), Mask: net.CIDRMask(24, 32)},
									MacAddress: "00:00:00:00:00:01",
									expiry:     time.Now().Add(time.Hour * 1),
									Hostname:   "testhost",
									state:      committed,
								},
							},
						},
					}
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '5',
					}))

					return req
				}(),
				resp: func() *dhcpv4.DHCPv4 {
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '5',
					}))
					resp, _ := dhcpv4.NewReplyFromRequest(req)
					return resp
				}(),
			},
			wantErr: false,
			expectedState: func() bool {
				if val, ok := pluginHdl.dhcpSubnets.subnets["VrfV15"+"Vlan2000"]; ok {
					if len(val.allocations.allocation) == 1 {
						return true
					}
				}
				return false
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := handleDiscover4(tt.args.req, tt.args.resp); (err != nil) != tt.wantErr || !tt.expectedState() {
				t.Errorf("handleDiscover4() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_handleRequest4(t *testing.T) {
	type args struct {
		req  *dhcpv4.DHCPv4
		resp *dhcpv4.DHCPv4
	}
	tests := []struct {
		name          string
		args          args
		wantErr       bool
		expectedState func() bool
	}{
		{
			name: "Test a DHCP request with relayAgentInfo and subnet available and there is an existing reservation",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					if pluginHdl == nil {
						pluginHdl = &pluginState{
							dhcpSubnets: &DHCPSubnets{
								subnets: map[string]*ManagedSubnet{},
							},
							//svcHdl: svc,
						}
					}
					pool, _ := NewIPv4Range(
						net.ParseIP("10.10.1.10"),
						net.ParseIP("10.10.1.240"),
						net.ParseIP("10.10.1.1"),
						binary.BigEndian.Uint32(net.ParseIP("10.10.1.240").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
						uint32(24),
					)
					pluginHdl.dhcpSubnets.subnets["VrfV16"+"Vlan2000"] = &ManagedSubnet{
						dhcpSubnet: &v1alpha2.DHCPSubnet{
							Spec: v1alpha2.DHCPSubnetSpec{},
							Status: v1alpha2.DHCPSubnetStatus{
								Allocated: map[string]v1alpha2.DHCPAllocated{},
							},
						},
						pool: pool,
						allocations: &ipallocations{
							allocation: map[string]*ipreservation{
								"00:00:00:00:00:01": &ipreservation{
									address:    net.IPNet{IP: net.ParseIP("10.10.1.10"), Mask: net.CIDRMask(24, 32)},
									MacAddress: "00:00:00:00:00:01",
									expiry:     time.Now().Add(time.Hour * 1),
									Hostname:   "testhost",
									state:      committed,
								},
							},
						},
					}
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
					req, _ := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
						dhcpv4.WithClientIP(net.ParseIP("10.10.1.10")),
						dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
							1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
							151, 7, ' ', 'V', 'r', 'f', 'V', '1', '6',
						}),
						dhcpv4.WithHwAddr(hardwareAddress),
					)

					return req
				}(),
				resp: func() *dhcpv4.DHCPv4 {
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:01")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '5',
					}))
					resp, _ := dhcpv4.NewReplyFromRequest(req)
					return resp
				}(),
			},
			wantErr: false,
			expectedState: func() bool {
				if val, ok := pluginHdl.dhcpSubnets.subnets["VrfV15"+"Vlan2000"]; ok {
					res, ok := val.allocations.allocation["00:00:00:00:00:01"]
					if !ok {
						return false
					}
					if res.state != committed {
						return false
					}

					return true
				}
				return false
			},
		},
		{
			name: "Test a request where we havent seen the mac address before",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					if pluginHdl == nil {
						pluginHdl = &pluginState{
							dhcpSubnets: &DHCPSubnets{
								subnets: map[string]*ManagedSubnet{},
							},
							//svcHdl: svc,
						}
					}
					pool, _ := NewIPv4Range(
						net.ParseIP("10.10.1.10"),
						net.ParseIP("10.10.1.240"),
						net.ParseIP("10.10.1.1"),
						binary.BigEndian.Uint32(net.ParseIP("10.10.1.240").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
						uint32(24),
					)
					pluginHdl.dhcpSubnets.subnets["VrfV16"+"Vlan2000"] = &ManagedSubnet{
						dhcpSubnet: &v1alpha2.DHCPSubnet{
							Spec: v1alpha2.DHCPSubnetSpec{},
							Status: v1alpha2.DHCPSubnetStatus{
								Allocated: map[string]v1alpha2.DHCPAllocated{},
							},
						},
						pool: pool,
						allocations: &ipallocations{
							allocation: map[string]*ipreservation{
								"00:00:00:00:00:01": &ipreservation{
									address:    net.IPNet{IP: net.ParseIP("10.10.1.10"), Mask: net.CIDRMask(24, 32)},
									MacAddress: "00:00:00:00:00:01",
									expiry:     time.Now().Add(time.Hour * 1),
									Hostname:   "testhost",
									state:      committed,
								},
							},
						},
					}
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:02")
					req, _ := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
						dhcpv4.WithClientIP(net.ParseIP("10.10.1.10")),
						dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
							1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
							151, 7, ' ', 'V', 'r', 'f', 'V', '1', '6',
						}),
						dhcpv4.WithHwAddr(hardwareAddress),
					)

					return req
				}(),
				resp: func() *dhcpv4.DHCPv4 {
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:02")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '6',
					}))
					resp, _ := dhcpv4.NewReplyFromRequest(req)
					return resp
				}(),
			},
			wantErr: false,
			expectedState: func() bool {
				if val, ok := pluginHdl.dhcpSubnets.subnets["VrfV16"+"Vlan2000"]; ok {
					res, ok := val.allocations.allocation["00:00:00:00:00:02"]
					if !ok {
						return false
					}
					if res.state != committed {
						return false
					}

					return true
				}
				return false
			},
		},
		{
			name: "Test Request where subnet is unknown",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					if pluginHdl == nil {
						pluginHdl = &pluginState{
							dhcpSubnets: &DHCPSubnets{
								subnets: map[string]*ManagedSubnet{},
							},
							//svcHdl: svc,
						}
					}
					pool, _ := NewIPv4Range(
						net.ParseIP("10.10.1.10"),
						net.ParseIP("10.10.1.240"),
						net.ParseIP("10.10.1.1"),
						binary.BigEndian.Uint32(net.ParseIP("10.10.1.240").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
						uint32(24),
					)
					pluginHdl.dhcpSubnets.subnets["VrfV16"+"Vlan2000"] = &ManagedSubnet{
						dhcpSubnet: &v1alpha2.DHCPSubnet{
							Spec: v1alpha2.DHCPSubnetSpec{},
							Status: v1alpha2.DHCPSubnetStatus{
								Allocated: map[string]v1alpha2.DHCPAllocated{},
							},
						},
						pool: pool,
						allocations: &ipallocations{
							allocation: map[string]*ipreservation{
								"00:00:00:00:00:01": &ipreservation{
									address:    net.IPNet{IP: net.ParseIP("10.10.1.10"), Mask: net.CIDRMask(24, 32)},
									MacAddress: "00:00:00:00:00:01",
									expiry:     time.Now().Add(time.Hour * 1),
									Hostname:   "testhost",
									state:      committed,
								},
							},
						},
					}
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:02")
					req, _ := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
						dhcpv4.WithClientIP(net.ParseIP("10.10.1.10")),
						dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
							1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
							151, 7, ' ', 'V', 'r', 'f', 'V', '1', '7',
						}),
						dhcpv4.WithHwAddr(hardwareAddress),
					)

					return req
				}(),
				resp: func() *dhcpv4.DHCPv4 {
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:02")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '7',
					}))
					resp, _ := dhcpv4.NewReplyFromRequest(req)
					return resp
				}(),
			},
			wantErr: true,
			expectedState: func() bool {
				return true
			},
		},
		{
			name: "test request for an exhausted pool",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					if pluginHdl == nil {
						pluginHdl = &pluginState{
							dhcpSubnets: &DHCPSubnets{
								subnets: map[string]*ManagedSubnet{},
							},
							//svcHdl: svc,
						}
					}
					pool, _ := NewIPv4Range(
						net.ParseIP("10.10.1.10"),
						net.ParseIP("10.10.1.12"),
						net.ParseIP("10.10.1.1"),
						binary.BigEndian.Uint32(net.ParseIP("10.10.1.12").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
						uint32(24),
					)
					pool.Allocate()
					pool.Allocate()
					pool.Allocate()
					pluginHdl.dhcpSubnets.subnets["VrfV18"+"Vlan2000"] = &ManagedSubnet{
						dhcpSubnet: &v1alpha2.DHCPSubnet{
							Spec: v1alpha2.DHCPSubnetSpec{},
							Status: v1alpha2.DHCPSubnetStatus{
								Allocated: map[string]v1alpha2.DHCPAllocated{},
							},
						},
						pool: pool,
						allocations: &ipallocations{
							allocation: map[string]*ipreservation{
								"00:00:00:00:00:01": &ipreservation{
									address:    net.IPNet{IP: net.ParseIP("10.10.1.10"), Mask: net.CIDRMask(24, 32)},
									MacAddress: "00:00:00:00:00:01",
									expiry:     time.Now().Add(time.Hour * 1),
									Hostname:   "testhost",
									state:      committed,
								},
							},
						},
					}

					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:03")
					req, _ := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
						dhcpv4.WithClientIP(net.ParseIP("10.10.1.10")),
						dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
							1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
							151, 7, ' ', 'V', 'r', 'f', 'V', '1', '8',
						}),
						dhcpv4.WithHwAddr(hardwareAddress),
					)

					return req
				}(),
				resp: func() *dhcpv4.DHCPv4 {
					hardwareAddress, _ := net.ParseMAC("00:00:00:00:00:03")
					req, _ := dhcpv4.NewDiscovery(hardwareAddress, dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
						1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
						151, 7, ' ', 'V', 'r', 'f', 'V', '1', '8',
					}))
					resp, _ := dhcpv4.NewReplyFromRequest(req)
					return resp
				}(),
			},
			wantErr: true,
			expectedState: func() bool {
				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := handleRequest4(tt.args.req, tt.args.resp); (err != nil) != tt.wantErr || !tt.expectedState() {
				t.Errorf("handleRequest4() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_handleDecline4(t *testing.T) {
	type args struct {
		req  *dhcpv4.DHCPv4
		resp *dhcpv4.DHCPv4
	}
	tests := []struct {
		name          string
		args          args
		wantErr       bool
		expectedState func() bool
	}{
		{
			name: "Test where decline has no agent info on it",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					decline, _ := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeDecline))

					return decline
				}(),
				resp: func() *dhcpv4.DHCPv4 {
					return &dhcpv4.DHCPv4{}
				}(),
			},
			wantErr: false,
			expectedState: func() bool {
				return true
			},
		},

		{
			name: "Test Decline with valid agent info with invalid vrf Vlan",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					hwAddr, _ := net.ParseMAC("00:00:00:00:00:04")
					decline, _ := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeDecline),
						dhcpv4.WithHwAddr(hwAddr),
						dhcpv4.WithClientIP(net.ParseIP("10.10.1.10")),
						dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
							1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
							151, 7, ' ', 'V', 'r', 'f', 'V', '1', '9',
						}))
					if pluginHdl == nil {
						pluginHdl = &pluginState{
							dhcpSubnets: &DHCPSubnets{
								subnets: map[string]*ManagedSubnet{},
							},
							//svcHdl: svc,
						}
					}
					pool, _ := NewIPv4Range(
						net.ParseIP("10.10.1.10"),
						net.ParseIP("10.10.1.12"),
						net.ParseIP("10.10.1.1"),
						binary.BigEndian.Uint32(net.ParseIP("10.10.1.12").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
						uint32(24),
					)
					pluginHdl.dhcpSubnets.subnets["VrfV19"+"Vlan2000"] = &ManagedSubnet{
						dhcpSubnet: &v1alpha2.DHCPSubnet{
							Spec: v1alpha2.DHCPSubnetSpec{},
							Status: v1alpha2.DHCPSubnetStatus{
								Allocated: map[string]v1alpha2.DHCPAllocated{},
							},
						},
						pool: pool,
						allocations: &ipallocations{
							allocation: map[string]*ipreservation{
								"00:00:00:00:00:04": &ipreservation{
									address:    net.IPNet{IP: net.ParseIP("10.10.1.10"), Mask: net.CIDRMask(24, 32)},
									MacAddress: "00:00:00:00:00:04",
									expiry:     time.Now().Add(time.Hour * 1),
									Hostname:   "testhost",
									state:      committed,
								},
							},
						},
					}

					return decline

				}(),
				resp: &dhcpv4.DHCPv4{},
			},
			wantErr: false,
			expectedState: func() bool {
				if val, ok := pluginHdl.dhcpSubnets.subnets["VrfV19"+"Vlan2000"]; ok {
					if _, ok1 := val.allocations.allocation["00:00:00:00:00:04"]; ok1 {
						return false
					}

					if val.pool.(*ipv4range).bitmap.Test(0) {
						return false
					}
				}
				return true
			},
		},
		{
			name: "Test Decline where agent info is valid but subnet is not defined",
			args: args{
				req: func() *dhcpv4.DHCPv4 {
					hwAddr, _ := net.ParseMAC("00:00:00:00:00:04")
					decline, _ := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeDecline),
						dhcpv4.WithHwAddr(hwAddr),
						dhcpv4.WithClientIP(net.ParseIP("10.10.1.10")),
						dhcpv4.WithGeneric(dhcpv4.OptionRelayAgentInformation, []byte{
							1, 8, 'V', 'l', 'a', 'n', '2', '0', '0', '0',
							151, 7, ' ', 'V', 'r', 'f', 'V', '2', '1',
						}))
					if pluginHdl == nil {
						pluginHdl = &pluginState{
							dhcpSubnets: &DHCPSubnets{
								subnets: map[string]*ManagedSubnet{},
							},
							//svcHdl: svc,
						}
					}
					pool, _ := NewIPv4Range(
						net.ParseIP("10.10.1.10"),
						net.ParseIP("10.10.1.12"),
						net.ParseIP("10.10.1.1"),
						binary.BigEndian.Uint32(net.ParseIP("10.10.1.12").To4())-binary.BigEndian.Uint32(net.ParseIP("10.10.1.10").To4())+1,
						uint32(24),
					)
					pluginHdl.dhcpSubnets.subnets["VrfV20"+"Vlan2000"] = &ManagedSubnet{
						dhcpSubnet: &v1alpha2.DHCPSubnet{
							Spec: v1alpha2.DHCPSubnetSpec{},
							Status: v1alpha2.DHCPSubnetStatus{
								Allocated: map[string]v1alpha2.DHCPAllocated{},
							},
						},
						pool: pool,
						allocations: &ipallocations{
							allocation: map[string]*ipreservation{
								"00:00:00:00:00:04": &ipreservation{
									address:    net.IPNet{IP: net.ParseIP("10.10.1.10"), Mask: net.CIDRMask(24, 32)},
									MacAddress: "00:00:00:00:00:04",
									expiry:     time.Now().Add(time.Hour * 1),
									Hostname:   "testhost",
									state:      committed,
								},
							},
						},
					}

					return decline

				}(),
				resp: &dhcpv4.DHCPv4{},
			},
			wantErr: true,
			expectedState: func() bool {
				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := handleDecline4(tt.args.req, tt.args.resp); (err != nil) != tt.wantErr || !tt.expectedState() {
				t.Errorf("handleDecline4() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_handleRelease4(t *testing.T) {
	type args struct {
		req  *dhcpv4.DHCPv4
		resp *dhcpv4.DHCPv4
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := handleRelease4(tt.args.req, tt.args.resp); (err != nil) != tt.wantErr {
				t.Errorf("handleRelease4() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_getSubnetInfo(t *testing.T) {
	type args struct {
		vrfName   string
		circuitID string
	}
	tests := []struct {
		name    string
		args    args
		want    *ManagedSubnet
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getSubnetInfo(tt.args.vrfName, tt.args.circuitID)
			if (err != nil) != tt.wantErr {
				t.Errorf("getSubnetInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getSubnetInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_handleExpiredLeases(t *testing.T) {
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handleExpiredLeases()
		})
	}
}

func Test_addPxeInfo(t *testing.T) {
	type args struct {
		req    *dhcpv4.DHCPv4
		resp   *dhcpv4.DHCPv4
		subnet *ManagedSubnet
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addPxeInfo(tt.args.req, tt.args.resp, tt.args.subnet)
		})
	}
}
