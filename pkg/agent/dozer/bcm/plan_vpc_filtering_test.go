// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

func TestBuildVNIVPCFilteringACL(t *testing.T) {
	newVPC := func() vpcapi.VPCSpec {
		return vpcapi.VPCSpec{
			Subnets: map[string]*vpcapi.VPCSubnet{
				"hbgp-a": {
					Subnet:  "10.0.1.0/24",
					HostBGP: true,
					HostBGPExtraPrefixes: map[string]vpcapi.VPCSubnetHostBGPPrefix{
						"10.100.1.0/24": {},
						"10.100.2.0/24": {},
					},
				},
				"hbgp-b": {
					Subnet:               "10.0.2.0/24",
					HostBGP:              true,
					HostBGPExtraPrefixes: map[string]vpcapi.VPCSubnetHostBGPPrefix{"10.100.3.0/24": {}},
				},
				"vlan-c": {Subnet: "10.0.3.0/24", VLAN: 1003},
			},
		}
	}

	newAgent := func() *agentapi.Agent {
		ag := &agentapi.Agent{}
		ag.Spec.Catalog.SubnetIDs = map[string]uint32{
			"10.0.1.0/24":   101,
			"10.0.2.0/24":   102,
			"10.0.3.0/24":   103,
			"10.100.1.0/24": 201,
			"10.100.2.0/24": 202,
			"10.100.3.0/24": 203,
		}

		return ag
	}

	accept := &dozer.SpecACLEntry{Action: dozer.SpecACLEntryActionAccept}

	for _, tt := range []struct {
		name    string
		subnet  string
		vpc     func(vpc *vpcapi.VPCSpec)
		agent   func(ag *agentapi.Agent)
		want    map[uint32]*dozer.SpecACLEntry
		wantErr bool
	}{
		{
			name:   "nothing to filter",
			subnet: "hbgp-a",
			want:   map[uint32]*dozer.SpecACLEntry{65535: accept},
		},
		{
			name:   "restricted hostBGP subnet drops its own subnet and extra prefixes by destination",
			subnet: "hbgp-a",
			vpc:    func(vpc *vpcapi.VPCSpec) { vpc.DefaultRestricted = true },
			want: map[uint32]*dozer.SpecACLEntry{
				1:     {DestinationAddress: new("10.0.1.0/24"), Action: dozer.SpecACLEntryActionDrop},
				201:   {DestinationAddress: new("10.100.1.0/24"), Action: dozer.SpecACLEntryActionDrop},
				202:   {DestinationAddress: new("10.100.2.0/24"), Action: dozer.SpecACLEntryActionDrop},
				65535: accept,
			},
		},
		{
			name:   "restricted VLAN subnet keeps matching source and destination",
			subnet: "vlan-c",
			vpc:    func(vpc *vpcapi.VPCSpec) { vpc.DefaultRestricted = true },
			want: map[uint32]*dozer.SpecACLEntry{
				1:     {SourceAddress: new("10.0.3.0/24"), DestinationAddress: new("10.0.3.0/24"), Action: dozer.SpecACLEntryActionDrop},
				65535: accept,
			},
		},
		{
			name:   "isolated hostBGP subnet is denied with its extra prefixes",
			subnet: "vlan-c",
			vpc:    func(vpc *vpcapi.VPCSpec) { vpc.Subnets["hbgp-b"].Isolated = new(true) },
			want: map[uint32]*dozer.SpecACLEntry{
				102:   {DestinationAddress: new("10.0.2.0/24"), Action: dozer.SpecACLEntryActionDiscard},
				203:   {DestinationAddress: new("10.100.3.0/24"), Action: dozer.SpecACLEntryActionDiscard},
				65535: accept,
			},
		},
		{
			name:   "permit lifts the deny of the extra prefixes too",
			subnet: "vlan-c",
			vpc: func(vpc *vpcapi.VPCSpec) {
				vpc.Subnets["hbgp-b"].Isolated = new(true)
				vpc.Permit = [][]string{{"vlan-c", "hbgp-b"}}
			},
			want: map[uint32]*dozer.SpecACLEntry{65535: accept},
		},
		{
			name:   "restricted and isolated together",
			subnet: "hbgp-a",
			vpc: func(vpc *vpcapi.VPCSpec) {
				vpc.DefaultRestricted = true
				vpc.DefaultIsolated = true
			},
			want: map[uint32]*dozer.SpecACLEntry{
				1:     {DestinationAddress: new("10.0.1.0/24"), Action: dozer.SpecACLEntryActionDrop},
				102:   {DestinationAddress: new("10.0.2.0/24"), Action: dozer.SpecACLEntryActionDiscard},
				103:   {DestinationAddress: new("10.0.3.0/24"), Action: dozer.SpecACLEntryActionDiscard},
				201:   {DestinationAddress: new("10.100.1.0/24"), Action: dozer.SpecACLEntryActionDrop},
				202:   {DestinationAddress: new("10.100.2.0/24"), Action: dozer.SpecACLEntryActionDrop},
				203:   {DestinationAddress: new("10.100.3.0/24"), Action: dozer.SpecACLEntryActionDiscard},
				65535: accept,
			},
		},
		{
			name:   "extra prefix without a subnet ID is skipped",
			subnet: "vlan-c",
			vpc:    func(vpc *vpcapi.VPCSpec) { vpc.Subnets["hbgp-b"].Isolated = new(true) },
			agent:  func(ag *agentapi.Agent) { delete(ag.Spec.Catalog.SubnetIDs, "10.100.3.0/24") },
			want: map[uint32]*dozer.SpecACLEntry{
				102:   {DestinationAddress: new("10.0.2.0/24"), Action: dozer.SpecACLEntryActionDiscard},
				65535: accept,
			},
		},
		{
			name:    "subnet without a subnet ID",
			subnet:  "vlan-c",
			vpc:     func(vpc *vpcapi.VPCSpec) { vpc.Subnets["hbgp-b"].Isolated = new(true) },
			agent:   func(ag *agentapi.Agent) { delete(ag.Spec.Catalog.SubnetIDs, "10.0.2.0/24") },
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vpc := newVPC()
			if tt.vpc != nil {
				tt.vpc(&vpc)
			}
			ag := newAgent()
			if tt.agent != nil {
				tt.agent(ag)
			}

			acl, err := buildVNIVPCFilteringACL(ag, "vpc-1", vpc, tt.subnet, vpc.Subnets[tt.subnet])
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, acl.Entries)
		})
	}
}
