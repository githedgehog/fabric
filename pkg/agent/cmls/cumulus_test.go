// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
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
		port      string
		subnets   []string
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
			name:    "vx-port-used-twice",
			profile: &switchprofile.CumulusVX,
			port:    "E1/1",
			subnets: []string{"vpc-01/default", "vpc-02/default"},
			err:     "port E1/1 is used more than once",
		},
		{
			name:    "sn5610-speed-on-breakout-port",
			profile: &switchprofile.CumulusNvidiaSN5610,
			speeds:  map[string]string{"E1/2": "400G"},
			port:    "E1/1",
			err:     "port E1/2 speed can't be configured",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			agent := &agentapi.Agent{
				Name: "leaf-01",
				Spec: agentapi.AgentSpec{
					Config: agentapi.AgentSpecConfig{ControlVIP: "172.30.0.1/32"},
					Switch: wiringapi.SwitchSpec{ProtocolIP: "172.30.8.1/32", PortBreakouts: tt.breakouts, PortSpeeds: tt.speeds},
					Connections: map[string]wiringapi.ConnectionSpec{
						"server-01--unbundled--leaf-01": {Unbundled: &wiringapi.ConnUnbundled{
							Link: wiringapi.ServerToSwitchLink{
								Server: wiringapi.BasePortName{Port: "server-01/enp2s1"},
								Switch: wiringapi.BasePortName{Port: "leaf-01/" + tt.port},
							},
						}},
					},
					VPCAttachments: map[string]agentapi.VPCAttachmentSpecAnn{},
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
			if tt.profile != nil {
				agent.Spec.SwitchProfile = tt.profile.Spec.DeepCopy()
			}

			_, err := buildConfigFor(fullCfgTmpl, agent)
			require.ErrorContains(t, err, tt.err)
		})
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
		{name: "vx-leaf-01", profile: switchprofile.CumulusVX}, // route summ, p2p link on an attachment
		// SN5610 with breakouts, incl. an 8x one that disables the next port
		{name: "sn5610-spine-01", profile: switchprofile.CumulusNvidiaSN5610}, // fabric links on a broken out port and on a 1x sub-port
		{name: "sn5610-leaf-01", profile: switchprofile.CumulusNvidiaSN5610},  // servers on a breakout sub-port and a bonus port, fabric link on an explicit 1x port
	} {
		t.Run(tt.name, func(t *testing.T) {
			agData, err := os.ReadFile(filepath.Join(testdataDir, tt.name+".in.agent.yaml"))
			require.NoError(t, err, "reading agent file")

			ag := &agentapi.Agent{}
			require.NoError(t, kyaml.Unmarshal(agData, ag), "unmarshalling agent data")
			ag.Spec.SwitchProfile = tt.profile.Spec.DeepCopy()

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
			}
		})
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
