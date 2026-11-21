// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fmeta "go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kyaml "sigs.k8s.io/yaml"
)

func TestBuildZTPFor(t *testing.T) {
	agent := &agentapi.Agent{
		Name:          "leaf-1",
		ManagedFields: []kmetav1.ManagedFieldsEntry{{Manager: "fabric-ctrl"}},
		Spec: agentapi.AgentSpec{
			Config:        agentapi.AgentSpecConfig{ControlVIP: "172.30.0.1/32"},
			Switch:        wiringapi.SwitchSpec{ProtocolIP: "172.30.8.1/32"},
			SwitchProfile: switchprofile.CumulusVX.Spec.DeepCopy(),
		},
		Status: agentapi.AgentStatus{RunID: "old-run-id"},
	}

	ztp, err := BuildZTPFor(agent.DeepCopy(), []byte("kubeconfig"), "v1.2.3+dev")
	require.NoError(t, err)
	require.Contains(t, ztp.String(), `wget -O agent "http://172.30.0.1:32000/agent?version=v1.2.3%2Bdev"`)
	require.Contains(t, ztp.String(), `nv config apply -y -m 'hh ztp v1.2.3+dev'`)
	require.NotContains(t, ztp.String(), "old-run-id")
	require.NotContains(t, ztp.String(), "managedFields")

	_, err = BuildZTPFor(agent.DeepCopy(), []byte("kubeconfig"), "")
	require.ErrorContains(t, err, "agent version is empty")
}

func TestBuildConfigErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		profile   *wiringapi.SwitchProfile
		breakouts map[string]string
		speeds    map[string]string
		autoNegs  map[string]bool
		fecs      map[string]wiringapi.PortFECMode
		port      string
		subnets   []string
		bundled   []string // ports of server-02 bundled connection
		bundledID uint16   // its port channel ID
		bundledTo []string // subnets it's attached to
		bundled2  []string // ports of server-03 bundled connection with the same port channel ID
		mutate    func(agent *agentapi.Agent)
		err       string
	}{
		{name: "no-profile", port: "E1/1", err: "switch profile is missing"},
		{name: "vx-missing-port", profile: &switchprofile.CumulusVX, port: "E1/65", err: "port E1/65 isn't available"},
		{name: "vx-sub-port", profile: &switchprofile.CumulusVX, port: "E1/1/1", err: "port E1/1/1 isn't available"},
		{
			name:      "sn5610-sub-port-out-of-mode",
			profile:   &switchprofile.CumulusNvidiaSN5610,
			breakouts: map[string]string{"E1/3": "2x400G"},
			port:      "E1/3/3",
			err:       "port E1/3/3 isn't available",
		},
		{
			name:      "sn5610-disabled-port",
			profile:   &switchprofile.CumulusNvidiaSN5610,
			breakouts: map[string]string{"E1/9": "8x100G"},
			port:      "E1/10",
			err:       "port E1/10 isn't available",
		},
		{
			name:    "vx-port-native-vlan-twice",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			subnets: []string{"vpc-01/default", "vpc-02/default"},
			mutate: func(agent *agentapi.Agent) {
				for name, attach := range agent.Spec.VPCAttachments {
					attach.NativeVLAN = true
					agent.Spec.VPCAttachments[name] = attach
				}
			},
			err: "native vlan is already 1001",
		},
		{
			name:    "vx-subnet-vlan-used-twice",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			subnets: []string{"vpc-01/default", "vpc-02/default"},
			mutate:  func(agent *agentapi.Agent) { agent.Spec.VPCs["vpc-02"].Subnets["default"].VLAN = 1001 },
			err:     "vlan 1001 is used by both subnet",
		},
		{
			name:    "vx-attachment-unsupported-conn",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.Connections["leaf-01--mesh--leaf-02"] = wiringapi.ConnectionSpec{Mesh: &wiringapi.ConnMesh{
					Links: []wiringapi.MeshLink{{
						Leaf1: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-01/E1/2"}},
						Leaf2: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-02/E1/2"}},
					}},
				}}
				agent.Spec.VPCAttachments["leaf-01--mesh--vpc-01--default"] = agentapi.VPCAttachmentSpecAnn{
					Connection: "leaf-01--mesh--leaf-02",
					Subnet:     "vpc-01/default",
				}
			},
			err: "conn leaf-01--mesh--leaf-02 type isn't supported",
		},
		{
			name:    "vx-vpc-mode-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withVPC01(func(vpc *vpcapi.VPCSpec) { vpc.Mode = vpcapi.VPCModeL3VNI }),
			err:     "vpc vpc-01: mode l3vni isn't supported",
		},
		{
			name:    "vx-vpc-permit-subnet-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withVPC01(func(vpc *vpcapi.VPCSpec) { vpc.Permit = [][]string{{"default", "other"}} }),
			err:     "vpc vpc-01: permit #0: subnet other not found",
		},
		{
			name:    "vx-vpc-static-route-invalid-prefix",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withStaticRoutes(
				vpcapi.VPCStaticRoute{Prefix: "10.99.0.0/16", NextHops: []string{"10.0.1.10"}},
				vpcapi.VPCStaticRoute{Prefix: "fd00::/64", NextHops: []string{"10.0.1.10"}},
			),
			err: `vpc vpc-01: static route #1: invalid prefix "fd00::/64"`,
		},
		{
			name:    "vx-vpc-static-route-invalid-next-hop",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withStaticRoutes(vpcapi.VPCStaticRoute{Prefix: "10.99.0.0/16", NextHops: []string{"10.0.1.10", "10.0.1"}}),
			err:     `vpc vpc-01: static route #0: invalid next hop "10.0.1"`,
		},
		{
			name:    "vx-vpc-static-route-no-next-hops",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withStaticRoutes(vpcapi.VPCStaticRoute{Prefix: "10.99.0.0/16"}),
			err:     "vpc vpc-01: static route #0: no next hops",
		},
		{
			name:    "vx-vpc-static-route-prefix-twice",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withStaticRoutes(
				vpcapi.VPCStaticRoute{Prefix: "10.99.0.0/16", NextHops: []string{"10.0.1.10"}},
				vpcapi.VPCStaticRoute{Prefix: "10.99.0.0/16", NextHops: []string{"10.0.1.20"}},
			),
			err: "vpc vpc-01: static route #1: prefix 10.99.0.0/16 is used more than once",
		},
		{
			name:    "vx-subnet-host-bgp-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.AttachedVPCs = map[string]bool{"vpc-01": true}
				agent.Spec.VPCs["vpc-01"].Subnets["default"].HostBGP = true
			},
			err: "vpc vpc-01: subnet default: host BGP isn't supported",
		},
		{
			name:    "vx-subnet-dhcp-relay-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.AttachedVPCs = map[string]bool{"vpc-01": true}
				agent.Spec.VPCs["vpc-01"].Subnets["default"].DHCP.Relay = "10.99.0.1"
			},
			err: "vpc vpc-01: subnet default: DHCP relay to other servers isn't supported",
		},
		{
			name:    "vx-subnet-isolated-no-id",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withVPC01(func(vpc *vpcapi.VPCSpec) {
					vpc.DefaultIsolated = true
					vpc.Subnets["other"] = &vpcapi.VPCSubnet{Subnet: "10.0.12.0/24", Gateway: "10.0.12.1", VLAN: 1012}
				})(agent)
				agent.Spec.Catalog.VPCSubnetVNIs["vpc-01"]["other"] = 102
			},
			err: "subnet vpc-01/other: subnet ID 0 isn't between 100 and 65000",
		},
		{
			name:    "vx-vpc-peering-remote-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.VPCPeerings = map[string]vpcapi.VPCPeeringSpec{"vpc-01--vpc-02": {
					Remote: "border",
					Permit: []map[string]vpcapi.VPCPeer{{"vpc-01": {}, "vpc-02": {}}},
				}}
			},
			err: "vpc peering vpc-01--vpc-02: remote peerings aren't supported",
		},
		{
			name:    "vx-vpc-peering-subnet-no-id",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.VPCPeerings = map[string]vpcapi.VPCPeeringSpec{"vpc-01--vpc-02": {
					Permit: []map[string]vpcapi.VPCPeer{{"vpc-01": {}, "vpc-02": {Subnets: []string{"default"}}}},
				}}
				agent.Spec.VPCs["vpc-02"].Subnets["other"] = &vpcapi.VPCSubnet{Subnet: "10.0.12.0/24", Gateway: "10.0.12.1", VLAN: 1012}
				agent.Spec.Catalog.VPCSubnetVNIs["vpc-02"]["other"] = 202
			},
			err: "vpc peering vpc-01--vpc-02: subnet vpc-02/other: subnet ID 0 isn't between 100 and 65000",
		},
		{
			name:    "vx-vpc-peering-vpc-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.VPCPeerings = map[string]vpcapi.VPCPeeringSpec{"vpc-01--vpc-09": {
					Permit: []map[string]vpcapi.VPCPeer{{"vpc-01": {}, "vpc-09": {}}},
				}}
			},
			err: "vpc peering vpc-01--vpc-09: vpc vpc-09 not found",
		},
		{
			name:    "vx-vpc-peering-permit-subnet-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.VPCPeerings = map[string]vpcapi.VPCPeeringSpec{"vpc-01--vpc-02": {
					Permit: []map[string]vpcapi.VPCPeer{{"vpc-01": {}, "vpc-02": {Subnets: []string{"other"}}}},
				}}
			},
			err: "vpc peering vpc-01--vpc-02: permit #0: subnet vpc-02/other not found",
		},
		{
			name:    "vx-vpc-subnet-empty",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.VPCs["vpc-02"].Subnets["other"] = nil },
			err:     "subnet vpc-02/other: empty",
		},
		{
			name:    "vx-gateway-no-domain",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withGateway("", 0, "172.30.128.20/31"),
			err:     "conn leaf-01--gateway--gateway-1: switch with a gateway connection has to be in a single domain",
		},
		{
			name:    "vx-gateway-no-asn",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withGateway("default", 0, "172.30.128.20/31"),
			err:     "conn leaf-01--gateway--gateway-1: no gateway ASN for domain default",
		},
		{
			name:    "vx-gateway-no-ip",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withGateway("default", 65534, ""),
			err:     `conn leaf-01--gateway--gateway-1: parsing switch IP ""`,
		},
		{
			name:    "vx-gateway-community-invalid-prio",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				asLeaf(agent)
				agent.Spec.Config.GatewayCommunities = map[string]string{"0": "50001:0", "high": "50001:1"}
			},
			err: `parsing gateway community priority "high"`,
		},
		{
			name:    "vx-eslag-no-redundancy",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withESLAG(agent)
				agent.Spec.Switch.Redundancy = wiringapi.SwitchRedundancy{}
			},
			err: "conn server-02--eslag--leaf-01--leaf-02: eslag connection on a switch without eslag redundancy",
		},
		{
			name:    "vx-eslag-spine",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withESLAG(agent)
				agent.Spec.Switch.Role = wiringapi.SwitchRoleSpine
			},
			err: "eslag redundancy is only supported on leaves",
		},
		{
			name:    "vx-eslag-no-connection-id",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withESLAG(agent)
				agent.Spec.Catalog.ConnectionIDs = nil
			},
			err: "conn server-02--eslag--leaf-01--leaf-02: no connection ID",
		},
		{
			name:    "vx-eslag-invalid-mac-base",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withESLAG(agent)
				agent.Spec.Config.ESLAGMACBase = "f2:00:00:00:00"
			},
			err: `invalid ESLAG MAC base "f2:00:00:00:00"`,
		},
		{
			name:    "vx-eslag-mac-overflow",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withESLAG(agent)
				agent.Spec.Config.ESLAGMACBase = "ff:ff:ff:ff:ff:ff"
			},
			err: "ESLAG MAC base ff:ff:ff:ff:ff:ff plus connection ID 2 is past the last MAC",
		},
		{
			name:    "vx-eslag-esi-prefix-not-type-0",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withESLAG(agent)
				agent.Spec.Config.ESLAGESIPrefix = "03:f2:00:00:"
			},
			err: `ESLAG ESI prefix "03:f2:00:00:" isn't the first 4 bytes of a type-0 ESI`,
		},
		{
			name:    "vx-eslag-esi-prefix-length",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withESLAG(agent)
				agent.Spec.Config.ESLAGESIPrefix = "00:f2:00:"
			},
			err: `ESLAG ESI prefix "00:f2:00:" isn't the first 4 bytes of a type-0 ESI`,
		},
		{
			name:    "vx-static-external-spine",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withStaticExternal(func(*wiringapi.ConnStaticExternal) {})(agent)
				agent.Spec.Switch.Role = wiringapi.SwitchRoleSpine
			},
			err: "conn leaf-01--static-external: static externals are only supported on leaves",
		},
		{
			name:    "vx-static-external-invalid-next-hop",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withStaticExternal(func(se *wiringapi.ConnStaticExternal) { se.Link.Switch.NextHop = "172.31.1" }),
			err:     `conn leaf-01--static-external: invalid next hop "172.31.1"`,
		},
		{
			name:    "vx-static-external-vpc-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withStaticExternal(func(se *wiringapi.ConnStaticExternal) { se.WithinVPC = "vpc-09" }),
			err:     "conn leaf-01--static-external: vpc vpc-09 not found",
		},
		{
			name:    "vx-static-external-no-subnet-id",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withStaticExternal(func(se *wiringapi.ConnStaticExternal) { se.Link.Switch.Subnets = []string{"10.98.0.0/16"} }),
			err:     "conn leaf-01--static-external: subnet 10.98.0.0/16: ID 0 isn't between 100 and 65000",
		},
		{
			name:    "vx-static-external-port-used",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withStaticExternal(func(se *wiringapi.ConnStaticExternal) { se.Link.Switch.Port = "leaf-01/E1/1" }),
			err:     "port E1/1 is used more than once",
		},
		{
			name:    "vx-static-external-route-twice",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withStaticExternal(func(*wiringapi.ConnStaticExternal) {})(agent)
				agent.Spec.Catalog.SubnetIDs["172.31.2.0/24"] = 202
				agent.Spec.Connections["leaf-01--static-external-2"] = wiringapi.ConnectionSpec{StaticExternal: &wiringapi.ConnStaticExternal{
					Link: wiringapi.ConnStaticExternalLink{Switch: wiringapi.ConnStaticExternalLinkSwitch{
						BasePortName: wiringapi.BasePortName{Port: "leaf-01/E1/3"},
						IP:           "172.31.2.1/24",
						NextHop:      "172.31.2.2",
						Subnets:      []string{"10.99.0.0/16"},
					}},
				}}
			},
			err: "conn leaf-01--static-external-2: route to 10.99.0.0/16 is already configured in vrf default",
		},
		{
			name:    "vx-ext-attach-proxy-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01", Static: &vpcapi.ExternalAttachmentStatic{RemoteIP: "100.1.10.1", Proxy: true}},
			}),
			err: "external attachment leaf-01--ext-01: proxy-ARP isn't supported",
		},
		{
			name:    "vx-ext-attach-conn-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {Connection: "leaf-01--other", External: "ext-01"},
			}),
			err: "external attachment leaf-01--ext-01: conn leaf-01--other not found",
		},
		{
			name:    "vx-ext-attach-conn-not-external",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {Connection: "server-01--unbundled--leaf-01", External: "ext-01"},
			}),
			err: "external attachment leaf-01--ext-01: conn server-01--unbundled--leaf-01 isn't an external conn of the switch",
		},
		{
			name:    "vx-ext-attach-external-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-02": {Connection: "leaf-01--external", External: "ext-02"},
			}),
			err: "external attachment leaf-01--ext-02: external ext-02 not found",
		},
		{
			name:    "vx-ext-no-vni",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
					"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01"},
				})(agent)
				delete(agent.Spec.Catalog.VPCVNIs, "ext@ext-01")
			},
			err: "external ext-01: no VNI",
		},
		{
			name:    "vx-ext-attach-invalid-ip",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01", Switch: vpcapi.ExternalAttachmentSwitch{VLAN: 10, IP: "100.1.10.1"}},
			}),
			err: `external attachment leaf-01--ext-01: invalid IP "100.1.10.1"`,
		},
		{
			name:    "vx-ext-attach-vlan-twice",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01":   {Connection: "leaf-01--external", External: "ext-01", Static: &vpcapi.ExternalAttachmentStatic{VLAN: 10, IP: "100.1.10.1/24", RemoteIP: "100.1.10.2"}},
				"leaf-01--ext-01-2": {Connection: "leaf-01--external", External: "ext-01", Static: &vpcapi.ExternalAttachmentStatic{VLAN: 10, IP: "100.1.20.1/24", RemoteIP: "100.1.20.2"}},
			}),
			err: "external attachment leaf-01--ext-01-2: vlan 10 on port swp2 is already used by leaf-01--ext-01",
		},
		{
			name:    "vx-ext-attach-neighbor-twice",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {
					Connection: "leaf-01--external", External: "ext-01",
					Switch: vpcapi.ExternalAttachmentSwitch{VLAN: 10, IP: "100.1.10.1/24"}, Neighbor: vpcapi.ExternalAttachmentNeighbor{IP: "100.1.10.6", ASN: 64102},
				},
				"leaf-01--ext-01-2": {
					Connection: "leaf-01--external", External: "ext-01",
					Switch: vpcapi.ExternalAttachmentSwitch{VLAN: 20, IP: "100.1.20.1/24"}, Neighbor: vpcapi.ExternalAttachmentNeighbor{IP: "100.1.10.6", ASN: 64102},
				},
			}),
			err: "external attachment leaf-01--ext-01-2: neighbor 100.1.10.6 is already used by leaf-01--ext-01",
		},
		{
			name:    "vx-ext-attach-static-no-ip",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01", Static: &vpcapi.ExternalAttachmentStatic{VLAN: 10, RemoteIP: "100.1.10.1"}},
			}),
			err: "external attachment leaf-01--ext-01: no IP",
		},
		{
			name:    "vx-ext-attach-static-invalid-remote-ip",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01", Static: &vpcapi.ExternalAttachmentStatic{VLAN: 10, IP: "100.1.10.2/24", RemoteIP: "100.1.10"}},
			}),
			err: `external attachment leaf-01--ext-01: invalid remote IP "100.1.10"`,
		},
		{
			name:    "vx-ext-attach-static-no-base-community",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
					"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01", Static: &vpcapi.ExternalAttachmentStatic{VLAN: 10, IP: "100.1.10.2/24", RemoteIP: "100.1.10.1"}},
				})(agent)
				agent.Spec.Config.BaseVPCCommunity = ""
			},
			err: "external attachment leaf-01--ext-01: no base VPC community",
		},
		{
			name:    "vx-ext-static-invalid-prefix",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
					"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01", Static: &vpcapi.ExternalAttachmentStatic{VLAN: 10, IP: "100.1.10.2/24", RemoteIP: "100.1.10.1"}},
				})(agent)
				agent.Spec.Externals["ext-01"].Static.Prefixes = []string{"0.0.0.0"}
			},
			err: `external ext-01: invalid static prefix "0.0.0.0"`,
		},
		{
			name:    "vx-ext-attach-prefix-limit-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {
					Connection: "leaf-01--external", External: "ext-01",
					Switch:   vpcapi.ExternalAttachmentSwitch{VLAN: 10, IP: "100.1.10.1/24"},
					Neighbor: vpcapi.ExternalAttachmentNeighbor{IP: "100.1.10.6", PrefixLimit: &vpcapi.ExternalAttachmentPrefixLimit{Max: 100}},
				},
			}),
			err: "external attachment leaf-01--ext-01: prefix limit isn't supported",
		},
		{
			name:    "vx-ext-attach-acl-icmp-code-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withStaticExtACL(vpcapi.ACLStatement{
				Seq: 10, Action: vpcapi.ACLActionDeny, Protocol: vpcapi.ACLProtocolICMP, SrcPrefix: vpcapi.ACLAny, DstPrefix: vpcapi.ACLAny,
				ICMPType: new(uint8(3)), ICMPCode: new(uint8(1)),
			}),
			err: "external attachment leaf-01--ext-01: inbound ACL statement 10: ICMP code isn't supported",
		},
		{
			name:    "vx-ext-attach-acl-last-seq-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withStaticExtACL(vpcapi.ACLStatement{
				Seq: 65535, Action: vpcapi.ACLActionPermit, Protocol: vpcapi.ACLProtocolIP, SrcPrefix: vpcapi.ACLAny, DstPrefix: vpcapi.ACLAny,
			}),
			err: "external attachment leaf-01--ext-01: inbound ACL statement 65535: seq isn't supported",
		},
		{
			name:    "vx-ext-attach-acl-reserved-seq",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withStaticExtACL(vpcapi.ACLStatement{
				Seq: 5, Action: vpcapi.ACLActionPermit, Protocol: vpcapi.ACLProtocolIP, SrcPrefix: vpcapi.ACLAny, DstPrefix: vpcapi.ACLAny,
			}),
			err: "external attachment leaf-01--ext-01: inbound ACL: statement 5: seq is reserved",
		},
		{
			name:    "vx-ext-attach-acl-invalid-prefix",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withStaticExtACL(vpcapi.ACLStatement{
				Seq: 10, Action: vpcapi.ACLActionPermit, Protocol: vpcapi.ACLProtocolIP, SrcPrefix: "10.0.0.1", DstPrefix: vpcapi.ACLAny,
			}),
			err: "external attachment leaf-01--ext-01: inbound ACL: statement 10: invalid prefix \"10.0.0.1\"",
		},
		{
			name:    "vx-ext-static-ipns-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withStaticExtACL()(agent)
				agent.Spec.IPv4Namespaces = nil
			},
			err: "external ext-01: ipv4 namespace default not found",
		},
		{
			name:    "vx-ext-ipns-invalid-subnet",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				withStaticExtACL()(agent)
				agent.Spec.IPv4Namespaces["default"] = vpcapi.IPv4NamespaceSpec{Subnets: []string{"10.0.0.0"}}
			},
			err: "ipv4 namespace default: invalid subnet \"10.0.0.0\"",
		},
		{
			name:    "vx-ext-attach-bgp-no-neighbor-ip",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01", Switch: vpcapi.ExternalAttachmentSwitch{VLAN: 10, IP: "100.1.10.1/24"}},
			}),
			err: "external attachment leaf-01--ext-01: switch and neighbor IPs have to be both set or both empty",
		},
		{
			name:    "vx-ext-attach-bgp-invalid-neighbor-ip",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
				"leaf-01--ext-01": {
					Connection: "leaf-01--external", External: "ext-01",
					Switch:   vpcapi.ExternalAttachmentSwitch{VLAN: 10, IP: "100.1.10.1/24"},
					Neighbor: vpcapi.ExternalAttachmentNeighbor{IP: "100.1.10.6/24"},
				},
			}),
			err: `external attachment leaf-01--ext-01: invalid neighbor IP "100.1.10.6/24"`,
		},
		{
			name:    "vx-ext-bgp-ipns-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withBGPExtAttachment(func(agent *agentapi.Agent) { agent.Spec.IPv4Namespaces = nil }),
			err:     "external ext-01: ipv4 namespace default not found",
		},
		{
			name:    "vx-ext-bgp-invalid-inbound-prefix",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withBGPExtAttachment(func(agent *agentapi.Agent) {
				agent.Spec.Externals["ext-01"] = vpcapi.ExternalSpec{
					IPv4Namespace:   "default",
					InboundPrefixes: map[string]vpcapi.ExternalInboundPrefix{"10.99.0.0": {}},
				}
			}),
			err: `external ext-01: invalid inbound prefix "10.99.0.0"`,
		},
		{
			name:    "vx-ext-bgp-no-domain",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withBGPExtAttachment(func(agent *agentapi.Agent) { agent.Spec.Switch.Topology.Domains = nil }),
			err:     "switch with a BGP external has to be in a single domain",
		},
		{
			name:    "vx-ext-bgp-domain-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withBGPExtAttachment(func(agent *agentapi.Agent) { agent.Spec.Config.Domains = nil }),
			err:     "domain default not found",
		},
		{
			name:    "vx-ext-peering-external-not-attached",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withExtPeering(func(_ *agentapi.Agent, peering *vpcapi.ExternalPeeringSpec) { peering.Permit.External.Name = "ext-02" }),
			err:     "external peering vpc-01--ext-01: external ext-02 isn't attached to the switch",
		},
		{
			name:    "vx-ext-peering-vpc-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withExtPeering(func(_ *agentapi.Agent, peering *vpcapi.ExternalPeeringSpec) { peering.Permit.VPC.Name = "vpc-03" }),
			err:     "external peering vpc-01--ext-01: vpc vpc-03 not found",
		},
		{
			name:    "vx-ext-peering-ipns-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtPeering(func(agent *agentapi.Agent, _ *vpcapi.ExternalPeeringSpec) {
				ext := agent.Spec.Externals["ext-01"]
				ext.IPv4Namespace = "other"
				agent.Spec.Externals["ext-01"] = ext
				agent.Spec.IPv4Namespaces = map[string]vpcapi.IPv4NamespaceSpec{"other": {Subnets: []string{"10.1.0.0/16"}}}
			}),
			err: "external peering vpc-01--ext-01: ipv4 namespace default not found",
		},
		{
			name:    "vx-ext-peering-twice",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtPeering(func(agent *agentapi.Agent, peering *vpcapi.ExternalPeeringSpec) {
				agent.Spec.ExternalPeerings["vpc-01--ext-01-2"] = *peering
			}),
			err: "external peering vpc-01--ext-01-2: vpc vpc-01 is already peered with external ext-01",
		},
		{
			name:    "vx-ext-peering-ext-id-invalid",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtPeering(func(agent *agentapi.Agent, _ *vpcapi.ExternalPeeringSpec) {
				agent.Spec.Catalog.ExternalIDs["ext-01"] = 5
			}),
			err: "external ext-01: external ID 5 isn't between 10 and 5000",
		},
		{
			name:    "vx-ext-peering-subnet-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtPeering(func(_ *agentapi.Agent, peering *vpcapi.ExternalPeeringSpec) {
				peering.Permit.VPC.Subnets = []string{"other"}
			}),
			err: "external peering vpc-01--ext-01: subnet vpc-01/other not found",
		},
		{
			name:    "vx-ext-peering-no-prefix-id",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withExtPeering(func(agent *agentapi.Agent, _ *vpcapi.ExternalPeeringSpec) {
				delete(agent.Spec.Catalog.SubnetIDs, "0.0.0.0/0")
			}),
			err: "external peering vpc-01--ext-01: prefix 0.0.0.0/0: ID 0 isn't between 1 and 65000",
		},
		{
			name:    "vx-fabric-link-ip-on-one-side",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withFabricLink("172.30.128.1/31", "", 65100),
			err:     "conn spine-01--fabric--leaf-01: port swp2: IPs have to be set on both sides or on neither",
		},
		{
			name:    "vx-fabric-link-no-peer-asn",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withFabricLink("", "", 0),
			err:     "conn spine-01--fabric--leaf-01: no ASN for peer spine-01",
		},
		{
			name:    "vx-fabric-link-invalid-peer-ip",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withFabricLink("172.30.128.1/31", "172.30.128.0", 65100),
			err:     "conn spine-01--fabric--leaf-01: parsing peer IP 172.30.128.0",
		},
		{
			name:    "vx-leaf-no-base-community",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  asLeaf,
			err:     "no base VPC community",
		},
		{
			name:    "sn5610-speed-on-breakout-port",
			profile: &switchprofile.CumulusNvidiaSN5610,
			speeds:  map[string]string{"E1/2": "400G"},
			port:    "E1/1",
			err:     "port E1/2 speed can't be configured",
		},
		{
			name:     "vx-autoneg-not-allowed",
			profile:  &switchprofile.CumulusVX,
			autoNegs: map[string]bool{"E1/2": true},
			port:     "E1/1",
			err:      "port E1/2 auto-negotiation can't be configured",
		},
		{
			name:     "sn5610-autoneg-on-breakout-port",
			profile:  &switchprofile.CumulusNvidiaSN5610,
			autoNegs: map[string]bool{"E1/2": true},
			port:     "E1/1",
			err:      "port E1/2 auto-negotiation can't be configured, port isn't available",
		},
		{
			name:      "sn5610-fec-on-breakout-port",
			profile:   &switchprofile.CumulusNvidiaSN5610,
			breakouts: map[string]string{"E1/2": "4x100G"},
			fecs:      map[string]wiringapi.PortFECMode{"E1/2": wiringapi.PortFECModeRS},
			port:      "E1/1",
			err:       "port E1/2 FEC can't be configured",
		},
		{
			name:    "sn5610-fec-on-port-and-sub-port",
			profile: &switchprofile.CumulusNvidiaSN5610,
			fecs:    map[string]wiringapi.PortFECMode{"E1/2": wiringapi.PortFECModeRS, "E1/2/1": wiringapi.PortFECModeFC},
			port:    "E1/1",
			err:     "FEC is configured more than once",
		},
		{
			name:    "sn5610-fec-invalid",
			profile: &switchprofile.CumulusNvidiaSN5610,
			fecs:    map[string]wiringapi.PortFECMode{"E1/2": "bad"},
			port:    "E1/1",
			err:     `port E1/2 FEC mode "bad" is invalid`,
		},
		{
			name:    "vx-bundled-no-port-channel-id",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			bundled: []string{"E1/2", "E1/3"},
			err:     "conn server-02--bundled--leaf-01: no port channel ID",
		},
		{
			name:      "vx-bundled-port-used-twice",
			profile:   &switchprofile.CumulusVX,
			port:      "E1/1",
			bundled:   []string{"E1/1", "E1/2"},
			bundledID: 1,
			err:       "port E1/1 is used more than once",
		},
		{
			name:      "vx-bundled-native-vlan-twice",
			profile:   &switchprofile.CumulusVX,
			port:      "E1/1",
			bundled:   []string{"E1/2", "E1/3"},
			bundledID: 1,
			bundledTo: []string{"vpc-01/default", "vpc-02/default"},
			mutate: func(agent *agentapi.Agent) {
				for name, attach := range agent.Spec.VPCAttachments {
					if attach.Connection == "server-02--bundled--leaf-01" {
						attach.NativeVLAN = true
						agent.Spec.VPCAttachments[name] = attach
					}
				}
			},
			err: "native vlan is already 1001",
		},
		{
			name:      "vx-bundled-port-channel-id-used-twice",
			profile:   &switchprofile.CumulusVX,
			port:      "E1/1",
			bundled:   []string{"E1/2", "E1/3"},
			bundledID: 1,
			bundled2:  []string{"E1/4", "E1/5"},
			err:       "port channel ID 1 is already used by conn",
		},
		{
			name:    "vx-vpc-no-vni",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { delete(agent.Spec.Catalog.VPCVNIs, "vpc-01") },
			err:     "vpc vpc-01: no VNI",
		},
		{
			name:    "vx-vpc-vni-not-multiple-of-100",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.Catalog.VPCVNIs["vpc-02"] = 250 },
			err:     "vpc vpc-02: VNI 250 isn't a multiple of 100 below 4000000",
		},
		{
			name:    "vx-subnet-no-vni",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { delete(agent.Spec.Catalog.VPCSubnetVNIs, "vpc-01") },
			err:     "subnet vpc-01/default: no VNI",
		},
		{
			name:    "vx-attachment-vpc-not-found",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			subnets: []string{"vpc-09/default"},
			err:     "vpc vpc-09 not found",
		},
		{
			name:    "vx-svi-workaround-subnet-invalid",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.Config.CumulusSVIWorkaroundSubnet = "172.30.96.0" },
			err:     "config: cumulusSVIWorkaroundSubnet",
		},
		{
			name:    "vx-svi-workaround-subnet-ipv6",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.Config.CumulusSVIWorkaroundSubnet = "fd00::/64" },
			err:     "cumulusSVIWorkaroundSubnet must be IPv4 /20 or bigger",
		},
		{
			name:    "vx-svi-workaround-subnet-too-small",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.Config.CumulusSVIWorkaroundSubnet = "172.30.96.0/24" },
			err:     "cumulusSVIWorkaroundSubnet must be IPv4 /20 or bigger",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			agent := &agentapi.Agent{
				Name: "leaf-01",
				Spec: agentapi.AgentSpec{
					Config: agentapi.AgentSpecConfig{ControlVIP: "172.30.0.1/32", CumulusSVIWorkaroundSubnet: "172.30.96.0/19"},
					Switch: wiringapi.SwitchSpec{ProtocolIP: "172.30.8.1/32", PortBreakouts: tt.breakouts, PortSpeeds: tt.speeds, PortAutoNegs: tt.autoNegs, PortFECs: tt.fecs},
					Connections: map[string]wiringapi.ConnectionSpec{
						"server-01--unbundled--leaf-01": {Unbundled: &wiringapi.ConnUnbundled{
							Link: wiringapi.ServerToSwitchLink{
								Server: wiringapi.BasePortName{Port: "server-01/enp2s1"},
								Switch: wiringapi.BasePortName{Port: "leaf-01/" + tt.port},
							},
						}},
					},
					VPCAttachments: map[string]agentapi.VPCAttachmentSpecAnn{},
					VPCs: map[string]vpcapi.VPCSpec{
						"vpc-01": {Subnets: map[string]*vpcapi.VPCSubnet{"default": {Subnet: "10.0.1.0/24", Gateway: "10.0.1.1", VLAN: 1001}}},
						"vpc-02": {Subnets: map[string]*vpcapi.VPCSubnet{"default": {Subnet: "10.0.2.0/24", Gateway: "10.0.2.1", VLAN: 1002}}},
					},
					Catalog: agentapi.CatalogSpec{
						VPCVNIs:       map[string]uint32{"vpc-01": 100, "vpc-02": 200},
						VPCSubnetVNIs: map[string]map[string]uint32{"vpc-01": {"default": 101}, "vpc-02": {"default": 201}},
					},
				},
			}
			if tt.subnets == nil {
				tt.subnets = []string{"vpc-01/default"}
			}
			for _, subnet := range tt.subnets {
				agent.Spec.VPCAttachments["server-01--"+strings.ReplaceAll(subnet, "/", "--")] = agentapi.VPCAttachmentSpecAnn{
					Connection: "server-01--unbundled--leaf-01",
					Subnet:     subnet,
				}
			}
			agent.Spec.Catalog.PortChannelIDs = map[string]uint16{}
			for server, ports := range map[string][]string{"server-02": tt.bundled, "server-03": tt.bundled2} {
				if ports == nil {
					continue
				}

				bundled := &wiringapi.ConnBundled{}
				for idx, port := range ports {
					bundled.Links = append(bundled.Links, wiringapi.ServerToSwitchLink{
						Server: wiringapi.BasePortName{Port: fmt.Sprintf("%s/enp2s%d", server, idx+1)},
						Switch: wiringapi.BasePortName{Port: "leaf-01/" + port},
					})
				}
				connName := server + "--bundled--leaf-01"
				agent.Spec.Connections[connName] = wiringapi.ConnectionSpec{Bundled: bundled}
				if tt.bundledID != 0 {
					agent.Spec.Catalog.PortChannelIDs[connName] = tt.bundledID
				}
			}
			for _, subnet := range tt.bundledTo {
				agent.Spec.VPCAttachments["server-02--"+strings.ReplaceAll(subnet, "/", "--")] = agentapi.VPCAttachmentSpecAnn{
					Connection: "server-02--bundled--leaf-01",
					Subnet:     subnet,
				}
			}
			if tt.profile != nil {
				agent.Spec.SwitchProfile = tt.profile.Spec.DeepCopy()
			}
			if tt.mutate != nil {
				tt.mutate(agent)
			}

			err := checkSupported(agent)
			if err == nil {
				_, err = buildConfigFor(fullCfgTmpl, agent)
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func TestCheckSupportedPeeredVPC(t *testing.T) {
	agent := &agentapi.Agent{Spec: agentapi.AgentSpec{VPCs: map[string]vpcapi.VPCSpec{"vpc-01": {
		Mode:    vpcapi.VPCModeL2VNI,
		Subnets: map[string]*vpcapi.VPCSubnet{"default": {HostBGP: true, DHCP: vpcapi.VPCDHCP{Relay: "10.99.0.1"}}},
	}}}}
	require.NoError(t, checkSupported(agent))

	agent.Spec.AttachedVPCs = map[string]bool{"vpc-01": true}
	require.ErrorContains(t, checkSupported(agent), "vpc vpc-01: subnet default: host BGP isn't supported")
}

func withVPC01(mutate func(vpc *vpcapi.VPCSpec)) func(agent *agentapi.Agent) {
	return func(agent *agentapi.Agent) {
		vpc := agent.Spec.VPCs["vpc-01"]
		mutate(&vpc)
		agent.Spec.VPCs["vpc-01"] = vpc
	}
}

func withStaticRoutes(routes ...vpcapi.VPCStaticRoute) func(agent *agentapi.Agent) {
	return func(agent *agentapi.Agent) {
		withVPC01(func(vpc *vpcapi.VPCSpec) { vpc.StaticRoutes = routes })(agent)
		agent.Spec.AttachedVPCs = map[string]bool{"vpc-01": true}
	}
}

func withGateway(domain string, asn uint32, switchIP string) func(agent *agentapi.Agent) {
	return func(agent *agentapi.Agent) {
		if domain != "" {
			agent.Spec.Switch.Topology.Domains = []string{domain}
		}
		agent.Spec.Config.Domains = map[string]wiringapi.FabricDomainSpec{"default": {GatewayASN: asn}}
		agent.Spec.Config.VTEPSubnet = "172.30.12.0/22"
		agent.Spec.Connections["leaf-01--gateway--gateway-1"] = wiringapi.ConnectionSpec{Gateway: &wiringapi.ConnGateway{
			Links: []wiringapi.GatewayLink{{
				Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-01/E1/2"}, IP: switchIP},
				Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.BasePortName{Port: "gateway-1/enp2s1"}, IP: "172.30.128.21/31"},
			}},
		}}
	}
}

func withExtAttachments(attaches map[string]vpcapi.ExternalAttachmentSpec) func(agent *agentapi.Agent) {
	return func(agent *agentapi.Agent) {
		agent.Spec.Connections["leaf-01--external"] = wiringapi.ConnectionSpec{External: &wiringapi.ConnExternal{
			Link: wiringapi.ConnExternalLink{Switch: wiringapi.BasePortName{Port: "leaf-01/E1/2"}},
		}}
		agent.Spec.Externals = map[string]vpcapi.ExternalSpec{"ext-01": {
			IPv4Namespace: "default",
			Static:        &vpcapi.ExternalStaticSpec{Prefixes: []string{"0.0.0.0/0"}},
		}}
		agent.Spec.Catalog.VPCVNIs["ext@ext-01"] = 300
		agent.Spec.Config.BaseVPCCommunity = testBaseVPCCommunity
		agent.Spec.IPv4Namespaces = map[string]vpcapi.IPv4NamespaceSpec{"default": {Subnets: []string{"10.0.0.0/16"}}}
		agent.Spec.ExternalAttachments = attaches
	}
}

const testBaseVPCCommunity = "50000:0"

func asLeaf(agent *agentapi.Agent) {
	agent.Spec.Config.SpineLeaf = &agentapi.AgentSpecConfigSpineLeaf{}
	agent.Spec.Switch.Role = wiringapi.SwitchRoleServerLeaf
	agent.Spec.Switch.VTEPIP = "172.30.12.0/32"
}

// withFabricLink makes leaf-01 a leaf with a single fabric link on E1/2 to spine-01
func withFabricLink(leafIP, spineIP string, spineASN uint32) func(agent *agentapi.Agent) {
	return func(agent *agentapi.Agent) {
		asLeaf(agent)
		agent.Spec.Switches = map[string]wiringapi.SwitchSpec{"spine-01": {ASN: spineASN, ProtocolIP: "172.30.8.0/32"}}
		agent.Spec.Connections["spine-01--fabric--leaf-01"] = wiringapi.ConnectionSpec{Fabric: &wiringapi.ConnFabric{
			Links: []wiringapi.FabricLink{{
				Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "spine-01/E1/1"}, IP: spineIP},
				Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-01/E1/2"}, IP: leafIP},
			}},
		}}
	}
}

// withESLAG makes leaf-01 a leaf of an ESLAG group with server-02 attached to it and leaf-02 over E1/2
func withESLAG(agent *agentapi.Agent) {
	asLeaf(agent)
	agent.Spec.Config.BaseVPCCommunity = testBaseVPCCommunity
	agent.Spec.Config.ESLAGMACBase = "f2:00:00:00:00:00"
	agent.Spec.Config.ESLAGESIPrefix = "00:f2:00:00:"
	agent.Spec.Switch.Redundancy = wiringapi.SwitchRedundancy{Group: "eslag-1", Type: fmeta.RedundancyTypeESLAG}
	connName := "server-02--eslag--leaf-01--leaf-02"
	agent.Spec.Connections[connName] = wiringapi.ConnectionSpec{ESLAG: &wiringapi.ConnESLAG{
		Links: []wiringapi.ServerToSwitchLink{
			{Server: wiringapi.BasePortName{Port: "server-02/enp2s1"}, Switch: wiringapi.BasePortName{Port: "leaf-01/E1/2"}},
			{Server: wiringapi.BasePortName{Port: "server-02/enp2s2"}, Switch: wiringapi.BasePortName{Port: "leaf-02/E1/2"}},
		},
	}}
	agent.Spec.Catalog.PortChannelIDs[connName] = 2
	agent.Spec.Catalog.ConnectionIDs = map[string]uint32{connName: 2}
}

// withStaticExternal makes leaf-01 a leaf with a static external on E1/2 routing 10.99.0.0/16 via 172.31.1.2
func withStaticExternal(mutate func(se *wiringapi.ConnStaticExternal)) func(agent *agentapi.Agent) {
	return func(agent *agentapi.Agent) {
		asLeaf(agent)
		agent.Spec.Config.BaseVPCCommunity = testBaseVPCCommunity
		if agent.Spec.Catalog.SubnetIDs == nil {
			agent.Spec.Catalog.SubnetIDs = map[string]uint32{}
		}
		agent.Spec.Catalog.SubnetIDs["172.31.1.0/24"] = 200
		agent.Spec.Catalog.SubnetIDs["10.99.0.0/16"] = 201
		se := &wiringapi.ConnStaticExternal{Link: wiringapi.ConnStaticExternalLink{Switch: wiringapi.ConnStaticExternalLinkSwitch{
			BasePortName: wiringapi.BasePortName{Port: "leaf-01/E1/2"},
			IP:           "172.31.1.1/24",
			NextHop:      "172.31.1.2",
			Subnets:      []string{"10.99.0.0/16"},
		}}}
		mutate(se)
		agent.Spec.Connections["leaf-01--static-external"] = wiringapi.ConnectionSpec{StaticExternal: se}
	}
}

// withStaticExtACL is a static attachment of ext-01 with the given inbound ACL statements
func withStaticExtACL(stmts ...vpcapi.ACLStatement) func(agent *agentapi.Agent) {
	return withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
		"leaf-01--ext-01": {
			Connection: "leaf-01--external", External: "ext-01",
			Static:     &vpcapi.ExternalAttachmentStatic{VLAN: 10, IP: "100.1.10.2/24", RemoteIP: "100.1.10.1"},
			InboundACL: &vpcapi.ACLSpec{Statements: stmts},
		},
	})
}

// withBGPExtAttachment is an unnumbered BGP attachment of ext-01 on a switch in the default domain
func withBGPExtAttachment(mutate func(agent *agentapi.Agent)) func(agent *agentapi.Agent) {
	return func(agent *agentapi.Agent) {
		withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
			"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01", Switch: vpcapi.ExternalAttachmentSwitch{VLAN: 10}},
		})(agent)
		agent.Spec.Switch.Topology.Domains = []string{"default"}
		agent.Spec.Config.Domains = map[string]wiringapi.FabricDomainSpec{"default": {SpineASN: 65100, GatewayASN: 65534}}
		mutate(agent)
	}
}

// withExtPeering is vpc-01 peered with the static ext-01
func withExtPeering(mutate func(agent *agentapi.Agent, peering *vpcapi.ExternalPeeringSpec)) func(agent *agentapi.Agent) {
	return func(agent *agentapi.Agent) {
		withExtAttachments(map[string]vpcapi.ExternalAttachmentSpec{
			"leaf-01--ext-01": {Connection: "leaf-01--external", External: "ext-01", Static: &vpcapi.ExternalAttachmentStatic{VLAN: 10, IP: "100.1.10.2/24", RemoteIP: "100.1.10.1"}},
		})(agent)
		withVPC01(func(vpc *vpcapi.VPCSpec) { vpc.IPv4Namespace = "default" })(agent)
		agent.Spec.Catalog.ExternalIDs = map[string]uint16{"ext-01": 10}
		agent.Spec.Catalog.SubnetIDs = map[string]uint32{"10.0.1.0/24": 100, "0.0.0.0/0": 103}
		peering := vpcapi.ExternalPeeringSpec{Permit: vpcapi.ExternalPeeringSpecPermit{
			VPC:      vpcapi.ExternalPeeringSpecVPC{Name: "vpc-01", Subnets: []string{"default"}},
			External: vpcapi.ExternalPeeringSpecExternal{Name: "ext-01", Prefixes: []vpcapi.ExternalPeeringSpecPrefix{{Prefix: "0.0.0.0/0"}}},
		}}
		agent.Spec.ExternalPeerings = map[string]vpcapi.ExternalPeeringSpec{}
		mutate(agent, &peering)
		agent.Spec.ExternalPeerings["vpc-01--ext-01"] = peering
	}
}

const testdataDir = "testdata"

func TestBuildConfig(t *testing.T) {
	for _, tt := range []struct {
		name    string
		profile wiringapi.SwitchProfile
	}{
		// l2vni vlab with 2 spines and 2 standalone leaves with unbundled servers, numbered fabric links
		{name: "vx-spine-01", profile: switchprofile.CumulusVX},
		{name: "vx-leaf-01", profile: switchprofile.CumulusVX}, // route summ
		// SN5610 with breakouts, incl. an 8x one that disables the next port
		{name: "sn5610-spine-01", profile: switchprofile.CumulusNvidiaSN5610}, // fabric links on a broken out port and on a 1x sub-port, all ports enabled, BFD disabled
		{name: "sn5610-leaf-01", profile: switchprofile.CumulusNvidiaSN5610},  // servers on a breakout sub-port and a bonus port, fabric link on an explicit 1x port, user w/o SSH keys
	} {
		t.Run(tt.name, func(t *testing.T) {
			agData, err := os.ReadFile(filepath.Join(testdataDir, tt.name+".in.agent.yaml"))
			require.NoError(t, err, "reading agent file")

			ag := &agentapi.Agent{}
			require.NoError(t, kyaml.Unmarshal(agData, ag), "unmarshalling agent data")
			ag.Spec.SwitchProfile = tt.profile.Spec.DeepCopy()
			require.NoError(t, checkSupported(ag), "checking supported features")

			for _, cfg := range []struct {
				name string
				tmpl string
			}{
				{name: "ztp", tmpl: ztpCfgTmpl},
				{name: "full", tmpl: fullCfgTmpl},
			} {
				actual, err := buildConfigFor(cfg.tmpl, ag)
				require.NoError(t, err, "building %s config", cfg.name)

				requireGolden(t, tt.name+".out."+cfg.name+"-config", actual.Bytes())

				var parsed any
				require.NoError(t, kyaml.Unmarshal(actual.Bytes(), &parsed), "parsing %s config", cfg.name)
				requireNoNulls(t, parsed, "")
			}
		})
	}
}

// requireNoNulls fails on a null anywhere in a generated config, as NVUE rejects a set with a null value, which is what
// a template key with nothing rendered under it becomes
func requireNoNulls(t *testing.T, v any, path string) {
	t.Helper()

	switch v := v.(type) {
	case nil:
		require.Fail(t, "null value in config", "at %s", path)
	case map[string]any:
		for k, child := range v {
			requireNoNulls(t, child, path+"/"+k)
		}
	case []any:
		for i, child := range v {
			requireNoNulls(t, child, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

func requireGolden(t *testing.T, name string, actual []byte) {
	t.Helper()

	expectedFileName := filepath.Join(testdataDir, name+".expected.yaml")
	actualFileName := filepath.Join(testdataDir, name+".actual.yaml")

	require.NoError(t, os.WriteFile(actualFileName, actual, 0o600), "writing actual file")

	if os.Getenv("UPDATE") == "true" {
		require.NoError(t, os.WriteFile(expectedFileName, actual, 0o600), "writing expected file")
	}

	expected, err := os.ReadFile(expectedFileName)
	require.NoError(t, err, "reading expected file")

	require.Equal(t, string(expected), string(actual),
		"%s mismatch, you can compare expected and actual files in testdata dir or re-generate expected by running just test-update", name)
}
