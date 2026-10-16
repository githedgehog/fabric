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
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

// the prefix limit is reported also when the switch returns no prefix counters
func TestBGPNeighborPrefixLimitState(t *testing.T) {
	type (
		instance   = oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance
		protocol   = oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol
		neighbor   = oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor
		afiSafi    = oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi
		ipv4       = oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_Ipv4Unicast
		limit      = oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_Ipv4Unicast_PrefixLimit
		limitState = oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_Ipv4Unicast_PrefixLimit_State
	)

	client := newGNMIMock()
	client.root = &oc.Device{
		SonicVrf: &oc.SonicVrf_SonicVrf{VRF: &oc.SonicVrf_SonicVrf_VRF{
			VRF_LIST: map[string]*oc.SonicVrf_SonicVrf_VRF_VRF_LIST{"VrfEext": {VrfName: pointer.To("VrfEext")}},
		}},
		NetworkInstances: &oc.OpenconfigNetworkInstance_NetworkInstances{NetworkInstance: map[string]*instance{
			"VrfEext": {
				Name: pointer.To("VrfEext"),
				Protocols: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{
					Protocol: map[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*protocol{
						{Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, Name: "bgp"}: {
							Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
							Name:       pointer.To("bgp"),
							Bgp: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp{
								Neighbors: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors{
									Neighbor: map[string]*neighbor{
										"100.1.10.6": {
											NeighborAddress: pointer.To("100.1.10.6"),
											State: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State{
												SessionState: oc.OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState_IDLE,
											},
											AfiSafis: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis{
												AfiSafi: map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*afiSafi{ //nolint:exhaustive
													oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST: {
														AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
														Ipv4Unicast: &ipv4{PrefixLimit: &limit{State: &limitState{
															MaxPrefixes: pointer.To[uint32](3), WarningThresholdPct: pointer.To[uint8](90),
														}}},
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
			},
		}},
	}

	p := &BroadcomProcessor{client: client}
	swState := &agentapi.SwitchState{BGPNeighbors: map[string]map[string]agentapi.SwitchStateBGPNeighbor{}}
	require.NoError(t, p.updateBGPNeighborMetrics(t.Context(), switchstate.NewRegistry(), swState, &agentapi.Agent{}, map[string]string{}))

	require.Equal(t, map[string]agentapi.SwitchStateBGPNeighborPrefixes{
		"IPV4_UNICAST": {Limit: 3, LimitWarnPct: 90},
	}, swState.BGPNeighbors["VrfEext"]["100.1.10.6"].Prefixes)
}
