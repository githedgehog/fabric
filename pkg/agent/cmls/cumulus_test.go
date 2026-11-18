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
				agent.Spec.Connections["server-02--eslag--leaf-01--leaf-02"] = wiringapi.ConnectionSpec{ESLAG: &wiringapi.ConnESLAG{
					Links: []wiringapi.ServerToSwitchLink{{
						Server: wiringapi.BasePortName{Port: "server-02/enp2s1"},
						Switch: wiringapi.BasePortName{Port: "leaf-01/E1/2"},
					}},
				}}
				agent.Spec.VPCAttachments["server-02--vpc-01--default"] = agentapi.VPCAttachmentSpecAnn{
					Connection: "server-02--eslag--leaf-01--leaf-02",
					Subnet:     "vpc-01/default",
				}
			},
			err: "conn server-02--eslag--leaf-01--leaf-02 type isn't supported",
		},
		{
			name:    "vx-vpc-mode-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withVPC01(func(vpc *vpcapi.VPCSpec) { vpc.Mode = vpcapi.VPCModeL3VNI }),
			err:     "vpc vpc-01: mode l3vni isn't supported",
		},
		{
			name:    "vx-vpc-permit-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withVPC01(func(vpc *vpcapi.VPCSpec) { vpc.Permit = [][]string{{"default"}} }),
			err:     "vpc vpc-01: permit lists aren't supported",
		},
		{
			name:    "vx-vpc-static-routes-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: withVPC01(func(vpc *vpcapi.VPCSpec) {
				vpc.StaticRoutes = []vpcapi.VPCStaticRoute{{Prefix: "10.99.0.0/24", NextHops: []string{"10.0.1.10"}}}
			}),
			err: "vpc vpc-01: static routes aren't supported",
		},
		{
			name:    "vx-subnet-host-bgp-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.VPCs["vpc-01"].Subnets["default"].HostBGP = true },
			err:     "vpc vpc-01: subnet default: host BGP isn't supported",
		},
		{
			name:    "vx-subnet-dhcp-relay-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.VPCs["vpc-01"].Subnets["default"].DHCP.Relay = "10.99.0.1" },
			err:     "vpc vpc-01: subnet default: DHCP relay to other servers isn't supported",
		},
		{
			name:    "vx-subnet-isolated-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  withVPC01(func(vpc *vpcapi.VPCSpec) { vpc.DefaultIsolated = true }),
			err:     "vpc vpc-01: subnet default: isolated and restricted subnets aren't supported",
		},
		{
			name:    "vx-subnet-restricted-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.VPCs["vpc-01"].Subnets["default"].Restricted = new(true) },
			err:     "vpc vpc-01: subnet default: isolated and restricted subnets aren't supported",
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
			name:    "vx-vpc-peering-subnets-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.VPCPeerings = map[string]vpcapi.VPCPeeringSpec{"vpc-01--vpc-02": {
					Permit: []map[string]vpcapi.VPCPeer{{"vpc-01": {Subnets: []string{"default"}}, "vpc-02": {}}},
				}}
			},
			err: "vpc peering vpc-01--vpc-02: permitting only some subnets isn't supported",
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
			name:    "vx-external-attachment-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.ExternalAttachments = map[string]vpcapi.ExternalAttachmentSpec{"leaf-01--ext-01": {}}
			},
			err: "externals aren't supported",
		},
		{
			name:    "vx-static-external-unsupported",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate: func(agent *agentapi.Agent) {
				agent.Spec.Connections["leaf-01--static-external"] = wiringapi.ConnectionSpec{StaticExternal: &wiringapi.ConnStaticExternal{}}
			},
			err: "conn leaf-01--static-external: static externals aren't supported",
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
			err:     "parsing Cumulus SVI workaround subnet",
		},
		{
			name:    "vx-svi-workaround-subnet-ipv6",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.Config.CumulusSVIWorkaroundSubnet = "fd00::/64" },
			err:     "parsing Cumulus SVI workaround subnet: fd00::/64 isn't IPv4",
		},
		{
			name:    "vx-svi-workaround-subnet-too-small",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			mutate:  func(agent *agentapi.Agent) { agent.Spec.Config.CumulusSVIWorkaroundSubnet = "172.30.96.0/24" },
			err:     "vlan 1001 doesn't fit Cumulus SVI workaround subnet 172.30.96.0/24",
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

func withVPC01(mutate func(vpc *vpcapi.VPCSpec)) func(agent *agentapi.Agent) {
	return func(agent *agentapi.Agent) {
		vpc := agent.Spec.VPCs["vpc-01"]
		mutate(&vpc)
		agent.Spec.VPCs["vpc-01"] = vpc
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
		{name: "sn5610-spine-01", profile: switchprofile.CumulusNvidiaSN5610}, // fabric links on a broken out port and on a 1x sub-port, all ports enabled
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
