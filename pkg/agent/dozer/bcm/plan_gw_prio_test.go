// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

func TestPlanGatewayPriorities(t *testing.T) {
	newAgent := func(comms map[string]string) *agentapi.Agent {
		ag := &agentapi.Agent{}
		ag.Name = "leaf-01"
		ag.Spec.Switch.Role = wiringapi.SwitchRoleServerLeaf
		ag.Spec.Switch.ProtocolIP = "10.0.1.1/32"
		ag.Spec.Switch.VTEPIP = "10.0.2.1/32"
		ag.Spec.Config.SpineLeaf = &agentapi.AgentSpecConfigSpineLeaf{}
		ag.Spec.Config.VTEPSubnet = "10.0.2.0/24"
		ag.Spec.Config.ProtocolSubnet = "10.0.1.0/24"
		ag.Spec.Config.GatewayCommunities = comms

		return ag
	}

	newSpec := func() *dozer.Spec {
		return &dozer.Spec{
			RouteMaps:      map[string]*dozer.SpecRouteMap{},
			PrefixLists:    map[string]*dozer.SpecPrefixList{},
			AsPathLists:    map[string]*dozer.SpecAsPathList{},
			CommunityLists: map[string]*dozer.SpecCommunityList{},
			VRFs: map[string]*dozer.SpecVRF{
				VRFDefault: {BGP: &dozer.SpecVRFBGP{Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{}}},
			},
		}
	}

	t.Run("sorted as numbers", func(t *testing.T) {
		comms := map[string]string{}
		for prio := range 11 {
			comms[fmt.Sprintf("%d", prio)] = fmt.Sprintf("50001:%d", prio)
		}
		spec := newSpec()
		require.NoError(t, planFabricConnections(newAgent(comms), spec))

		stmts := spec.RouteMaps[RouteMapL2VPNNeighbors].Statements
		for prio := range 11 {
			stmt := stmts[fmt.Sprintf("%d", prio+1)]
			require.NotNil(t, stmt)
			require.Equal(t, fmt.Sprintf("gw-prio-%d", prio), *stmt.Conditions.MatchCommunityList)
			require.Equal(t, uint32(GwPrioPreferenceBase+11-prio), *stmt.SetLocalPreference)
			require.Equal(t, []string{fmt.Sprintf("50001:%d", prio)}, spec.CommunityLists[*stmt.Conditions.MatchCommunityList].Members)
		}
	})

	t.Run("priority not a number", func(t *testing.T) {
		require.Error(t, planFabricConnections(newAgent(map[string]string{"high": "50001:0"}), newSpec()))
	})
}
