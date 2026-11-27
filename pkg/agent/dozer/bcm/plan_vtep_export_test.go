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

// A leaf advertises its own VTEP and the gateway VTEPs to the spines; the latter also when learned from another
// spine, so gateways can reach each other through a leaf when nothing else is left (#1782).
func TestPlanFabricVTEPPolicies(t *testing.T) {
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
		ag.Spec.Switch.Topology.Domains = []string{"default"}
		ag.Spec.Config.Domains = map[string]wiringapi.FabricDomainSpec{
			"default": {SpineASN: 65100, GatewayASN: 65200},
			"plane-b": {SpineASN: 65010, GatewayASN: 65011},
		}
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
			AsPathLists:    map[string]*dozer.SpecAsPathList{},
			CommunityLists: map[string]*dozer.SpecCommunityList{},
			VRFs: map[string]*dozer.SpecVRF{
				VRFDefault: {BGP: &dozer.SpecVRFBGP{Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{}}},
			},
		}
	}

	// the loopback session with the spine is the one carrying the VTEP routes
	loopbackNeighbor := func(spec *dozer.Spec) *dozer.SpecVRFBGPNeighbor {
		neigh, ok := spec.VRFs[VRFDefault].BGP.Neighbors["172.30.11.2"]
		require.True(t, ok, "loopback neighbor with the spine must exist")

		return neigh
	}

	gatewayVTEPs := &dozer.SpecRouteMapStatement{
		Conditions: dozer.SpecRouteMapConditions{
			MatchAsPathList: new(AsPathListGatewayOriginAny),
			MatchPrefixList: new(PrefixListAllVTEPPrefixes),
		},
		Result: dozer.SpecRouteMapResultAccept,
	}
	rejectTransit := &dozer.SpecRouteMapStatement{
		Conditions: dozer.SpecRouteMapConditions{MatchAsPathList: new(AsPathListLeafTransit)},
		Result:     dozer.SpecRouteMapResultReject,
	}
	acceptRest := &dozer.SpecRouteMapStatement{Result: dozer.SpecRouteMapResultAccept}

	t.Run("leaf without gateway", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planFabricConnections(newAgent(wiringapi.SwitchRoleServerLeaf, false), spec))

		neigh := loopbackNeighbor(spec)
		require.Equal(t, []string{RouteMapLoopbackVTEP}, neigh.IPv4UnicastExportPolicies)
		require.Equal(t, []string{RouteMapLoopbackImport}, neigh.IPv4UnicastImportPolicies)
		require.Nil(t, neigh.IPv4AllowOwnAS)
		require.Equal(t, "172.30.12.1/32", spec.PrefixLists[PrefixListVTEPPrefix].Prefixes[10].Prefix.Prefix)

		require.Equal(t, gatewayVTEPs, spec.RouteMaps[RouteMapLoopbackVTEP].Statements["20"])
		require.Equal(t, []string{"_65200$"}, spec.AsPathLists[AsPathListGatewayOriginAny].Members)

		// it must never use a path that went through another leaf, not even towards a gateway
		require.Equal(t, map[string]*dozer.SpecRouteMapStatement{
			"20":    rejectTransit,
			"65535": acceptRest,
		}, spec.RouteMaps[RouteMapLoopbackImport].Statements)
		require.Equal(t, []string{"_65100_.*_65100_"}, spec.AsPathLists[AsPathListLeafTransit].Members)
	})

	t.Run("leaf with gateway", func(t *testing.T) {
		spec := newSpec()
		ag := newAgent(wiringapi.SwitchRoleServerLeaf, true)
		require.NoError(t, planFabricConnections(ag, spec))
		require.NoError(t, planGatewayConnections(ag, spec))

		require.Equal(t, gatewayVTEPs, spec.RouteMaps[RouteMapLoopbackVTEP].Statements["20"])

		// it takes the gateway VTEPs that went through another leaf, to forward its gateways' traffic to them
		require.Equal(t, map[string]*dozer.SpecRouteMapStatement{
			"10":    gatewayVTEPs,
			"20":    rejectTransit,
			"65535": acceptRest,
		}, spec.RouteMaps[RouteMapLoopbackImport].Statements)

		gw := spec.VRFs[VRFDefault].BGP.Neighbors["172.30.129.1"]
		require.NotNil(t, gw)
		require.Equal(t, []string{RouteMapGatewayImport}, gw.IPv4UnicastImportPolicies)
		require.Equal(t, []string{RouteMapGatewayExport}, gw.IPv4UnicastExportPolicies)

		// anything not originated by the gateway is dropped, so it is never a path between switches
		require.Equal(t, map[string]*dozer.SpecRouteMapStatement{
			"10": {
				Conditions: dozer.SpecRouteMapConditions{MatchAsPathList: new(AsPathListGatewayOrigin)},
				Result:     dozer.SpecRouteMapResultAccept,
			},
		}, spec.RouteMaps[RouteMapGatewayImport].Statements)
		require.Equal(t, []string{"^65200$"}, spec.AsPathLists[AsPathListGatewayOrigin].Members)
		require.Equal(t, map[string]*dozer.SpecRouteMapStatement{
			"10": {
				Conditions: dozer.SpecRouteMapConditions{MatchPrefixList: new(PrefixListAllVTEPPrefixes)},
				Result:     dozer.SpecRouteMapResultAccept,
			},
		}, spec.RouteMaps[RouteMapGatewayExport].Statements)
	})

	t.Run("leaf in two domains", func(t *testing.T) {
		spec := newSpec()
		ag := newAgent(wiringapi.SwitchRoleServerLeaf, false)
		ag.Spec.Switch.Topology.Domains = []string{"default", "plane-b"}
		require.NoError(t, planFabricConnections(ag, spec))

		// it would pass one domain's gateway VTEPs on to the other domain's spines
		require.NotContains(t, spec.RouteMaps[RouteMapLoopbackVTEP].Statements, "20")
		require.Equal(t, []string{"_65010_.*_65010_", "_65100_.*_65100_"}, spec.AsPathLists[AsPathListLeafTransit].Members)
		require.Equal(t, []string{RouteMapLoopbackImport}, loopbackNeighbor(spec).IPv4UnicastImportPolicies)
	})

	t.Run("mesh leaf", func(t *testing.T) {
		spec := newSpec()
		ag := newAgent(wiringapi.SwitchRoleServerLeaf, true)
		delete(ag.Spec.Connections, "fabric-1")
		require.NoError(t, planFabricConnections(ag, spec))

		require.NotContains(t, spec.RouteMaps, RouteMapLoopbackImport)
		require.NotContains(t, spec.RouteMaps[RouteMapLoopbackVTEP].Statements, "20")
		require.NotContains(t, spec.AsPathLists, AsPathListLeafTransit)
	})

	t.Run("spine", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planFabricConnections(newAgent(wiringapi.SwitchRoleSpine, false), spec))

		neigh := loopbackNeighbor(spec)
		require.Equal(t, []string{RouteMapLoopbackAllVTEPs}, neigh.IPv4UnicastExportPolicies)
		require.NotContains(t, spec.RouteMaps, RouteMapLoopbackVTEP)

		// its own ASN once is a path through a leaf, which is only accepted for the gateway VTEPs
		require.Equal(t, new(uint8(1)), neigh.IPv4AllowOwnAS)
		require.Nil(t, neigh.L2VPNEVPNAllowOwnAS)
		require.Equal(t, []string{RouteMapLoopbackImport}, neigh.IPv4UnicastImportPolicies)
		require.Equal(t, map[string]*dozer.SpecRouteMapStatement{
			"10":    gatewayVTEPs,
			"20":    rejectTransit,
			"65535": acceptRest,
		}, spec.RouteMaps[RouteMapLoopbackImport].Statements)
		require.Equal(t, []string{"_65100_"}, spec.AsPathLists[AsPathListLeafTransit].Members)
	})
}
