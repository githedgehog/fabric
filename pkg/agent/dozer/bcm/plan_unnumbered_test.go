// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
)

// A fabric or mesh link with no IPs on either side runs BGP unnumbered: IPv6 is enabled on the link
// and the neighbor is keyed by the interface instead of the peer IP, but the peer ASN stays explicit
// so a miscabled link cannot bring the session up. On TH5 the link lives on the workaround SVI
// (see planMeshConnections), so that is the interface the peering runs over.
func TestPlanUnnumberedFabricMeshLinks(t *testing.T) {
	const (
		self     = "leaf-01"
		peer     = "leaf-02"
		spine    = "spine-01"
		selfPort = "E1/1"
		peerASN  = uint32(65102)
		wVLAN    = uint16(3001)
	)

	newAgent := func(silicon string, conn wiringapi.ConnectionSpec) *agentapi.Agent {
		ag := &agentapi.Agent{}
		ag.Name = self
		ag.Spec.Switch.ProtocolIP = "172.30.11.1/32"
		ag.Spec.Connections = map[string]wiringapi.ConnectionSpec{"conn-1": conn}
		ag.Spec.Switches = map[string]wiringapi.SwitchSpec{
			peer:  {ASN: peerASN, ProtocolIP: "172.30.11.2/32"},
			spine: {ASN: peerASN, ProtocolIP: "172.30.11.3/32"},
		}
		ag.Spec.SwitchProfile = &wiringapi.SwitchProfileSpec{SwitchSilicon: silicon}
		ag.Spec.Catalog.TH5WorkaroundVLANs = map[string]uint16{selfPort: wVLAN}

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

	meshConn := func(ip1, ip2 string) wiringapi.ConnectionSpec {
		return wiringapi.ConnectionSpec{
			Mesh: &wiringapi.ConnMesh{
				Links: []wiringapi.MeshLink{{
					Leaf1: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: self + "/" + selfPort}, IP: ip1},
					Leaf2: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: peer + "/E1/2"}, IP: ip2},
				}},
			},
		}
	}

	// planFabricConnections only runs in spine-leaf mode and needs the underlay route-map inputs
	fabricAgent := func(ip1, ip2 string) *agentapi.Agent {
		ag := newAgent(switchprofile.SiliconVS, wiringapi.ConnectionSpec{
			Fabric: &wiringapi.ConnFabric{
				Links: []wiringapi.FabricLink{{
					Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: spine + "/E1/2"}, IP: ip2},
					Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: self + "/" + selfPort}, IP: ip1},
				}},
			},
		})
		ag.Spec.Switch.Role = wiringapi.SwitchRoleServerLeaf
		ag.Spec.Switch.VTEPIP = "172.30.12.1/32"
		ag.Spec.Config.SpineLeaf = &agentapi.AgentSpecConfigSpineLeaf{}
		ag.Spec.Config.VTEPSubnet = "172.30.12.0/24"
		ag.Spec.Config.ProtocolSubnet = "172.30.11.0/24"

		return ag
	}

	t.Run("mesh unnumbered", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planMeshConnections(newAgent(switchprofile.SiliconVS, meshConn("", "")), spec))

		sub := spec.Interfaces[selfPort].Subinterfaces[0]
		require.NotNil(t, sub.IPv6)
		require.True(t, *sub.IPv6.Enabled)
		require.Empty(t, sub.IPs)

		neigh, ok := spec.VRFs[VRFDefault].BGP.Neighbors[selfPort]
		require.True(t, ok, "neighbor must be keyed by the port")
		require.Equal(t, peerASN, *neigh.RemoteAS)
		require.True(t, *neigh.ExtendedNexthop)
		require.Equal(t, FabricBFDProfile, *neigh.BFDProfile)
	})

	t.Run("mesh numbered", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planMeshConnections(newAgent(switchprofile.SiliconVS, meshConn("172.30.128.0/31", "172.30.128.1/31")), spec))

		sub := spec.Interfaces[selfPort].Subinterfaces[0]
		require.Nil(t, sub.IPv6)
		require.Contains(t, sub.IPs, "172.30.128.0")

		neigh, ok := spec.VRFs[VRFDefault].BGP.Neighbors["172.30.128.1"]
		require.True(t, ok, "neighbor must be keyed by the peer IP")
		require.Nil(t, neigh.ExtendedNexthop)
	})

	t.Run("mesh unnumbered on TH5", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planMeshConnections(newAgent(switchprofile.SiliconBroadcomTH5, meshConn("", "")), spec))

		require.Equal(t, wVLAN, *spec.Interfaces[selfPort].AccessVLAN)
		require.Nil(t, spec.Interfaces[selfPort].Subinterfaces[0].IPv6, "the port itself stays L2 on TH5")

		svi := spec.Interfaces[vlanName(wVLAN)]
		require.NotNil(t, svi.VLANIPv6)
		require.True(t, *svi.VLANIPv6.Enabled)
		require.Empty(t, svi.VLANIPs)

		_, ok := spec.VRFs[VRFDefault].BGP.Neighbors[vlanName(wVLAN)]
		require.True(t, ok, "neighbor must be keyed by the workaround SVI")
	})

	t.Run("mesh numbered on TH5", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planMeshConnections(newAgent(switchprofile.SiliconBroadcomTH5, meshConn("172.30.128.0/31", "172.30.128.1/31")), spec))

		svi := spec.Interfaces[vlanName(wVLAN)]
		require.Nil(t, svi.VLANIPv6)
		require.Contains(t, svi.VLANIPs, "172.30.128.0")

		_, ok := spec.VRFs[VRFDefault].BGP.Neighbors["172.30.128.1"]
		require.True(t, ok)
	})

	t.Run("fabric unnumbered", func(t *testing.T) {
		spec := newSpec()
		require.NoError(t, planFabricConnections(fabricAgent("", ""), spec))

		sub := spec.Interfaces[selfPort].Subinterfaces[0]
		require.NotNil(t, sub.IPv6)
		require.Empty(t, sub.IPs)

		neigh, ok := spec.VRFs[VRFDefault].BGP.Neighbors[selfPort]
		require.True(t, ok, "neighbor must be keyed by the port")
		require.Equal(t, peerASN, *neigh.RemoteAS)
		require.True(t, *neigh.ExtendedNexthop)
	})

	t.Run("half numbered links are rejected", func(t *testing.T) {
		require.Error(t, planMeshConnections(newAgent(switchprofile.SiliconVS, meshConn("172.30.128.0/31", "")), newSpec()))
		require.Error(t, planMeshConnections(newAgent(switchprofile.SiliconVS, meshConn("", "172.30.128.1/31")), newSpec()))

		require.Error(t, planFabricConnections(fabricAgent("172.30.128.0/31", ""), newSpec()))
	})
}
