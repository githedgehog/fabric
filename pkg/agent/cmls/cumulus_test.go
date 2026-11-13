// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"os"
	"path/filepath"
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
			Config: agentapi.AgentSpecConfig{ControlVIP: "172.30.0.1/32"},
			Switch: wiringapi.SwitchSpec{ProtocolIP: "172.30.8.1/32"},
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
		{name: "sn5610-spine-01", profile: switchprofile.CumulusNvidiaSN5610}, // fabric link on a broken out port
		{name: "sn5610-leaf-01", profile: switchprofile.CumulusNvidiaSN5610},  // servers on a breakout sub-port and a bonus port
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
