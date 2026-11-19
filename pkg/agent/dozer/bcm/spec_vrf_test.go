// Copyright 2026 Hedgehog
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

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

// The switch refuses a neighbor replace that swaps the restart timer for warning-only or back, so the prefix limit
// has to be deleted first. Both directions crash-looped the agent on a VLAB.
func TestSpecVRFBGPPrefixLimitModeSwitch(t *testing.T) {
	restart := &dozer.SpecVRFBGPNeighbor{IPv4MaxPrefixes: new(uint32(3)), IPv4MaxPrefixesRestart: new(uint16(5))}
	restartHigher := &dozer.SpecVRFBGPNeighbor{IPv4MaxPrefixes: new(uint32(10)), IPv4MaxPrefixesRestart: new(uint16(5))}
	warnOnly := &dozer.SpecVRFBGPNeighbor{IPv4MaxPrefixes: new(uint32(3)), IPv4MaxPrefixesWarnOnly: new(true)}
	noLimit := &dozer.SpecVRFBGPNeighbor{}

	deletePath := "/network-instances/network-instance[name=VrfEext]/protocols/protocol[identifier=BGP][name=bgp]/bgp" +
		"/neighbors/neighbor[neighbor-address=1.2.3.4]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/ipv4-unicast/prefix-limit"

	for _, tt := range []struct {
		name            string
		actual, desired *dozer.SpecVRFBGPNeighbor
		wantDelete      bool
	}{
		{name: "restart to warning-only", actual: restart, desired: warnOnly, wantDelete: true},
		{name: "warning-only to restart", actual: warnOnly, desired: restart, wantDelete: true},
		{name: "restart with another max", actual: restart, desired: restartHigher},
		{name: "added", actual: noLimit, desired: warnOnly},
		{name: "removed", actual: warnOnly, desired: noLimit},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actual := &dozer.SpecVRFBGP{Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{"1.2.3.4": tt.actual}}
			desired := &dozer.SpecVRFBGP{Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{"1.2.3.4": tt.desired}}

			actions := &ActionQueue{}
			require.NoError(t, specVRFBGPEnforcer.Handle("/network-instances/network-instance[name=VrfEext]", "VrfEext", actual, desired, actions))

			deletes := []string{}
			for _, a := range actions.actions {
				if act := a.(*Action); act.Type == ActionTypeDelete {
					deletes = append(deletes, act.Path)
				}
			}
			if tt.wantDelete {
				require.Equal(t, []string{deletePath}, deletes)
			} else {
				require.Empty(t, deletes)
			}
		})
	}

	require.Less(t, ActionWeightVRFBGPNeighborDelete, ActionWeightVRFBGPNeighborUpdate)
}

// SONiC returns AS numbers as strings once the BGP instance uses a dotted notation
func TestUnmarshalOCVRFsDottedAS(t *testing.T) {
	ocVal := &oc.OpenconfigNetworkInstance_NetworkInstances{
		NetworkInstance: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance{
			VRFDefault: {
				Name:   new(VRFDefault),
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Config{Name: new(VRFDefault)},
				Protocols: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{
					Protocol: map[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol{
						{Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, Name: "bgp"}: {
							Bgp: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp{
								Global: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global{
									Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_Config{
										As:         oc.UnionString("64086.59904"),
										AsNotation: oc.OpenconfigBgp_AsNotationType_ASDOT,
									},
								},
								Neighbors: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors{
									Neighbor: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor{
										"1.2.3.4": {
											NeighborAddress: new("1.2.3.4"),
											Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config{
												NeighborAddress: new("1.2.3.4"),
												PeerAs:          oc.UnionString("65100"),
												LocalAs:         oc.UnionString("1.0"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	vrfs, err := unmarshalOCVRFs(ocVal)
	require.NoError(t, err)
	bgp := vrfs[VRFDefault].BGP
	require.NotNil(t, bgp)
	require.Equal(t, new(uint32(4200000000)), bgp.AS)
	require.Equal(t, new(true), bgp.ASDot)
	require.Equal(t, new(uint32(65100)), bgp.Neighbors["1.2.3.4"].RemoteAS)
	require.Equal(t, new(uint32(65536)), bgp.Neighbors["1.2.3.4"].LocalAS)
}

func TestUnmarshalOCVRFsIPv4AllowOwnAS(t *testing.T) {
	neighbor := func(addr string, enabled bool) *oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor{
			NeighborAddress: new(addr),
			Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config{
				NeighborAddress: new(addr),
			},
			AfiSafis: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis{
				AfiSafi: map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi{ //nolint:exhaustive,nolintlint
					oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST: {
						AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
						AllowOwnAs: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_AllowOwnAs{
							Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_AllowOwnAs_Config{
								Enabled: new(enabled),
								AsCount: new(uint8(1)),
							},
						},
					},
				},
			},
		}
	}

	ocVal := &oc.OpenconfigNetworkInstance_NetworkInstances{
		NetworkInstance: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance{
			VRFDefault: {
				Name:   new(VRFDefault),
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Config{Name: new(VRFDefault)},
				Protocols: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{
					Protocol: map[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol{
						{Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, Name: "bgp"}: {
							Bgp: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp{
								Global: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global{
									Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_Config{
										As: oc.UnionUint32(65100),
									},
								},
								Neighbors: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors{
									Neighbor: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor{
										"1.2.3.4": neighbor("1.2.3.4", true),
										"1.2.3.5": neighbor("1.2.3.5", false),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	vrfs, err := unmarshalOCVRFs(ocVal)
	require.NoError(t, err)
	neighbors := vrfs[VRFDefault].BGP.Neighbors
	require.Equal(t, new(uint8(1)), neighbors["1.2.3.4"].IPv4AllowOwnAS)
	// the switch may keep the count around after disabling it
	require.Nil(t, neighbors["1.2.3.5"].IPv4AllowOwnAS)
}
