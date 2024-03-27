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

package apiabbr_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/client/apiabbr"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

func TestParseVPCSubnet(t *testing.T) {
	tests := []struct {
		in             string
		expectedName   string
		expectedSubnet *vpcapi.VPCSubnet
		expectedErr    bool
	}{
		{
			in:             "default",
			expectedName:   "default",
			expectedSubnet: &vpcapi.VPCSubnet{},
			expectedErr:    false,
		},
		{
			in:           "default=10.42.0.0/24",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "1042",
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,vlan=2222",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "2222",
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet:   "10.42.0.0/24",
				VLAN:     "1042",
				Isolated: pointer.To(true),
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,isolated",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet:   "10.42.0.0/24",
				VLAN:     "1042",
				Isolated: pointer.To(true),
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i=1",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet:   "10.42.0.0/24",
				VLAN:     "1042",
				Isolated: pointer.To(true),
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i=t",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet:   "10.42.0.0/24",
				VLAN:     "1042",
				Isolated: pointer.To(true),
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i=true",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet:   "10.42.0.0/24",
				VLAN:     "1042",
				Isolated: pointer.To(true),
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i=y",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet:   "10.42.0.0/24",
				VLAN:     "1042",
				Isolated: pointer.To(true),
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i=yes",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet:   "10.42.0.0/24",
				VLAN:     "1042",
				Isolated: pointer.To(true),
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i=f",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "1042",
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i=false",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "1042",
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i=noooope",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "1042",
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,r",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet:     "10.42.0.0/24",
				VLAN:       "1042",
				Restricted: pointer.To(true),
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,i,r",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet:     "10.42.0.0/24",
				VLAN:       "1042",
				Isolated:   pointer.To(true),
				Restricted: pointer.To(true),
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,dhcp",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "1042",
				DHCP: vpcapi.VPCDHCP{
					Enable: true,
				},
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,dhcp,dhcp-end=10.42.0.99",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "1042",
				DHCP: vpcapi.VPCDHCP{
					Enable: true,
					Range: &vpcapi.VPCDHCPRange{
						End: "10.42.0.99",
					},
				},
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,dhcp,dhcp-start=10.42.0.10,dhcp-end=10.42.0.99",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "1042",
				DHCP: vpcapi.VPCDHCP{
					Enable: true,
					Range: &vpcapi.VPCDHCPRange{
						Start: "10.42.0.10",
						End:   "10.42.0.99",
					},
				},
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,dhcp-relay=10.42.100.100",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "1042",
				DHCP: vpcapi.VPCDHCP{
					Relay: "10.42.100.100",
				},
			},
		},
		{
			in:           "default=10.42.0.0/24,vlan=1042,dhcp-pxe-url=10.42.100.100",
			expectedName: "default",
			expectedSubnet: &vpcapi.VPCSubnet{
				Subnet: "10.42.0.0/24",
				VLAN:   "1042",
				DHCP: vpcapi.VPCDHCP{
					PXEURL: "10.42.100.100",
				},
			},
		},
		{
			in:          "default=10.42.0.0/24=vlan=1042",
			expectedErr: true,
		},
		{
			in:          "default=10.42.0.0/24,magic=please",
			expectedErr: true,
		},
		{
			in:          "",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			actualName, actualSubnet, err := apiabbr.ParseVPCSubnet(tt.in)

			if tt.expectedErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedName, actualName)
			require.Equal(t, tt.expectedSubnet, actualSubnet)
		})
	}
}

func TestParseVPCPermits(t *testing.T) {
	tests := []struct {
		in              string
		expectedPermits []string
		expectedErr     bool
	}{
		{
			in:              "subnet-1,subnet-2",
			expectedPermits: []string{"subnet-1", "subnet-2"},
			expectedErr:     false,
		},
		{
			in:              "subnet-1,subnet-2,subnet-3",
			expectedPermits: []string{"subnet-1", "subnet-2", "subnet-3"},
			expectedErr:     false,
		},
		{
			in:          "",
			expectedErr: true,
		},
		{
			in:          "subnet-1",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			actualPermits, err := apiabbr.ParseVPCPermits(tt.in)

			if tt.expectedErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedPermits, actualPermits)
		})
	}
}

func TestParseVPCPeeringPermits(t *testing.T) {
	tests := []struct {
		in             string
		expectedPermit [2]vpcapi.VPCPeer
		expectedErr    bool
	}{
		{
			in: "~",
			expectedPermit: [2]vpcapi.VPCPeer{
				{}, {},
			},
		},
		{
			in: "subnet-1~",
			expectedPermit: [2]vpcapi.VPCPeer{
				{
					Subnets: []string{"subnet-1"},
				}, {},
			},
		},
		{
			in: "~subnet-1",
			expectedPermit: [2]vpcapi.VPCPeer{
				{}, {
					Subnets: []string{"subnet-1"},
				},
			},
		},
		{
			in: "subnet-1~subnet-1",
			expectedPermit: [2]vpcapi.VPCPeer{
				{
					Subnets: []string{"subnet-1"},
				}, {
					Subnets: []string{"subnet-1"},
				},
			},
		},
		{
			in: "subnet-1,subnet-2~subnet-1,subnet-2,subnet-42",
			expectedPermit: [2]vpcapi.VPCPeer{
				{
					Subnets: []string{"subnet-1", "subnet-2"},
				}, {
					Subnets: []string{"subnet-1", "subnet-2", "subnet-42"},
				},
			},
		},
		{
			in:          "",
			expectedErr: true,
		},
		{
			in:          "subnet-1~subnet-a~subnet-noooope",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			actualPermit, err := apiabbr.ParseVPCPeeringPermits("vpc-1", "vpc-2", tt.in)

			if tt.expectedErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Len(t, actualPermit, 2)
			require.Equal(t, tt.expectedPermit[0], actualPermit["vpc-1"])
			require.Equal(t, tt.expectedPermit[1], actualPermit["vpc-2"])
		})
	}
}
