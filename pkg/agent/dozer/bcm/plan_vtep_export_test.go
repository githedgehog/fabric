// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

// A leaf advertises only its own VTEP to the spines, unless it is attached to a gateway: the
// gateway VTEP has to reach the rest of the fabric and cannot be matched explicitly, so such a
// leaf falls back to advertising the whole VTEP subnet.
func TestPlanFabricVTEPExportPolicy(t *testing.T) {
	const (
		self  = "leaf-01"
		spine = "spine-01"
	)

	newAgent := func(role wiringapi.SwitchRole, gateway bool) *agentapi.Agent {
		ag := &agentapi.Agent{}
		ag.Name = self
		ag.Spec.Switch.Role = role
		ag.Spec.Switch.ProtocolIP = "172.30.11.1/32"
		ag.Spec.Switch.VTEPIP = "172.30.12.1/32"
		ag.Spec.Config.SpineLeaf = &agentapi.AgentSpecConfigSpineLeaf{}
		ag.Spec.Config.VTEPSubnet = "172.30.12.0/24"
		ag.Spec.Config.ProtocolSubnet = "172.30.11.0/24"
		ag.Spec.Switches = map[string]wiringapi.SwitchSpec{
			spine: {ASN: 65100, ProtocolIP: "172.30.11.2/32"},
		}
		ag.Spec.Connections = map[string]wiringapi.ConnectionSpec{
			"fabric-1": {Fabric: &wiringapi.ConnFabric{
				Links: []wiringapi.FabricLink{{
					Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: spine + "/E1/1"}, IP: "172.30.128.1/31"},
					Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: self + "/E1/1"}, IP: "172.30.128.0/31"},
				}},
			}},
		}
		if gateway {
			ag.Spec.Connections["gw-1"] = wiringapi.ConnectionSpec{Gateway: &wiringapi.ConnGateway{
				Links: []wiringapi.GatewayLink{{
					Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: self + "/E1/2"}, IP: "172.30.129.0/31"},
					Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.BasePortName{Port: "gw-01/enp2s1"}, IP: "172.30.129.1/31"},
				}},
			}}
		}

		return ag
	}

	newSpec := func() *dozer.Spec {
		return &dozer.Spec{
			Interfaces:     map[string]*dozer.SpecInterface{},
			RouteMaps:      map[string]*dozer.SpecRouteMap{},
			PrefixLists:    map[string]*dozer.SpecPrefixList{},
			CommunityLists: map[string]*dozer.SpecCommunityList{},
			VRFs: map[string]*dozer.SpecVRF{
				VRFDefault: {BGP: &dozer.SpecVRFBGP{Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{}}},
			},
		}
	}

	// the loopback session with the spine is the one carrying the VTEP routes
	exportPolicies := func(spec *dozer.Spec) []string {
		neigh, ok := spec.VRFs[VRFDefault].BGP.Neighbors["172.30.11.2"]
		require.True(t, ok, "loopback neighbor with the spine must exist")

		return neigh.IPv4UnicastExportPolicies
	}

	t.Run("leaf without gateway", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planFabricConnections(newAgent(wiringapi.SwitchRoleServerLeaf, false), spec))

		require.Equal(t, []string{RouteMapLoopbackVTEP}, exportPolicies(spec))
		require.Equal(t, "172.30.12.1/32", spec.PrefixLists[PrefixListVTEPPrefix].Prefixes[10].Prefix.Prefix)
	})

	t.Run("leaf with gateway", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planFabricConnections(newAgent(wiringapi.SwitchRoleServerLeaf, true), spec))

		require.Equal(t, []string{RouteMapLoopbackAllVTEPs}, exportPolicies(spec))
		require.NotContains(t, spec.RouteMaps, RouteMapLoopbackVTEP)
	})

	t.Run("spine", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planFabricConnections(newAgent(wiringapi.SwitchRoleSpine, false), spec))

		require.Equal(t, []string{RouteMapLoopbackAllVTEPs}, exportPolicies(spec))
		require.NotContains(t, spec.RouteMaps, RouteMapLoopbackVTEP)
	})
}
