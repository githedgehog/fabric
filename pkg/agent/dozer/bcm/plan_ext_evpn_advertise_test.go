// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
)

// An external VRF advertises over EVPN only routes outside its ipns carrying baseVPCCommunity, so
// every way a route enters it from the external must set that community, and the import of VPC
// subnets must not.
func TestPlanExternalEVPNAdvertise(t *testing.T) {
	const (
		ext     = "ext-01"
		extComm = "50000:0"
	)

	staticAttach := vpcapi.ExternalAttachmentSpec{
		External:   ext,
		Connection: "leaf-01--external",
		Static:     &vpcapi.ExternalAttachmentStatic{VLAN: 10, IP: "100.1.10.2/24", RemoteIP: "100.1.10.1"},
	}
	bgpAttach := vpcapi.ExternalAttachmentSpec{
		External:   ext,
		Connection: "leaf-01--external",
		Switch:     vpcapi.ExternalAttachmentSwitch{VLAN: 20, IP: "100.1.20.2/24"},
		Neighbor:   vpcapi.ExternalAttachmentNeighbor{ASN: 64102, IP: "100.1.20.1"},
	}

	for _, tt := range []struct {
		name    string
		attachs map[string]vpcapi.ExternalAttachmentSpec
	}{
		{name: "static", attachs: map[string]vpcapi.ExternalAttachmentSpec{"static": staticAttach}},
		{name: "bgp", attachs: map[string]vpcapi.ExternalAttachmentSpec{"bgp": bgpAttach}},
		{name: "mixed", attachs: map[string]vpcapi.ExternalAttachmentSpec{"static": staticAttach, "bgp": bgpAttach}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ag := &agentapi.Agent{}
			ag.Name = "leaf-01"
			ag.Spec.Switch.ASN = 65101
			ag.Spec.Switch.ProtocolIP = "172.30.8.2/32"
			ag.Spec.Switch.Topology.Domains = []string{"default"}
			ag.Spec.Config.Domains = map[string]wiringapi.FabricDomainSpec{"default": {SpineASN: 65100}}
			ag.Spec.Config.SpineLeaf = &agentapi.AgentSpecConfigSpineLeaf{}
			ag.Spec.Config.BaseVPCCommunity = extComm
			ag.Spec.Connections = map[string]wiringapi.ConnectionSpec{
				"leaf-01--external": {External: &wiringapi.ConnExternal{
					Link: wiringapi.ConnExternalLink{Switch: wiringapi.BasePortName{Port: "leaf-01/E1/5"}},
				}},
			}
			ag.Spec.IPv4Namespaces = map[string]vpcapi.IPv4NamespaceSpec{"default": {Subnets: []string{"10.0.0.0/16"}}}
			external := vpcapi.ExternalSpec{IPv4Namespace: "default"}
			if _, ok := tt.attachs["static"]; ok {
				external.Static = &vpcapi.ExternalStaticSpec{Prefixes: []string{"0.0.0.0/0"}}
			}
			ag.Spec.Externals = map[string]vpcapi.ExternalSpec{ext: external}
			ag.Spec.ExternalAttachments = tt.attachs
			ag.Spec.Catalog.IRBVLANs = map[string]uint16{librarian.ReqForExt(ext): 3001}
			ag.Spec.Catalog.VPCVNIs = map[string]uint32{librarian.ReqForExt(ext): 300}

			spec := &dozer.Spec{
				Interfaces:     map[string]*dozer.SpecInterface{},
				RouteMaps:      map[string]*dozer.SpecRouteMap{RouteMapL2VPNNeighbors: {Statements: map[string]*dozer.SpecRouteMapStatement{}}},
				PrefixLists:    map[string]*dozer.SpecPrefixList{},
				CommunityLists: map[string]*dozer.SpecCommunityList{},
				AsPathLists:    map[string]*dozer.SpecAsPathList{},
				ACLs:           map[string]*dozer.SpecACL{},
				VRFs:           map[string]*dozer.SpecVRF{},
				VRFVNIMap:      map[string]*dozer.SpecVRFVNIEntry{},
				VXLANTunnelMap: map[string]*dozer.SpecVXLANTunnelMap{},
			}
			require.NoError(t, planExternals(ag, spec))

			require.Equal(t, []string{extComm}, spec.CommunityLists[BGPCommListAllExternals].Members)
			require.Equal(t, &dozer.SpecRouteMap{
				Statements: map[string]*dozer.SpecRouteMapStatement{
					"10": {
						Conditions: dozer.SpecRouteMapConditions{MatchPrefixList: new("ipns-subnets--default")},
						Result:     dozer.SpecRouteMapResultReject,
					},
					"20": {
						Conditions: dozer.SpecRouteMapConditions{MatchCommunityList: new(BGPCommListAllExternals)},
						Result:     dozer.SpecRouteMapResultAccept,
					},
				},
			}, spec.RouteMaps["ext-evpn-advertise--default"])

			vrf := spec.VRFs[extVrfName(ext)]
			require.NotNil(t, vrf)
			require.Equal(t, []string{"ext-evpn-advertise--default"}, vrf.BGP.L2VPNEVPN.AdvertiseIPv4UnicastRouteMaps)

			requireAcceptsTagged := func(routeMap string) {
				t.Helper()
				require.Contains(t, spec.RouteMaps, routeMap)
				for seq, stmt := range spec.RouteMaps[routeMap].Statements {
					if stmt.Result == dozer.SpecRouteMapResultAccept {
						require.Contains(t, stmt.SetCommunities, extComm, "route-map %s statement %s", routeMap, seq)
					}
				}
			}

			if _, ok := tt.attachs["static"]; ok {
				policies := vrf.TableConnections[string(dozer.SpecVRFBGPTableConnectionStatic)].ImportPolicies
				require.NotEmpty(t, policies)
				for _, routeMap := range policies {
					requireAcceptsTagged(routeMap)
				}
			}
			if _, ok := tt.attachs["bgp"]; ok {
				policies := vrf.BGP.Neighbors["100.1.20.1"].IPv4UnicastImportPolicies
				require.NotEmpty(t, policies)
				for _, routeMap := range policies {
					requireAcceptsTagged(routeMap)
				}
			}

			for seq, stmt := range spec.RouteMaps[*vrf.BGP.IPv4Unicast.ImportPolicy].Statements {
				require.NotContains(t, stmt.SetCommunities, extComm, "import vrf route-map statement %s", seq)
			}
		})
	}
}
