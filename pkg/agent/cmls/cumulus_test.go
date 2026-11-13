// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
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
			SwitchProfile: &wiringapi.SwitchProfileSpec{Ports: map[string]wiringapi.SwitchProfilePort{
				"M1":   {NOSName: "eth0", Management: true},
				"E1/1": {NOSName: "1", BaseNOSName: "swp"},
			}},
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

func TestSwp(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{in: "E1/1", want: "swp1"},
		{in: "E1/32", want: "swp32"},
		{in: "E1/1/1", want: "swp1s0"},
		{in: "E1/1/4", want: "swp1s3"},
		{in: "E1/12/2", want: "swp12s1"},
		{in: "E1/abc", want: "port-invalid-uint"},
		{in: "E1/1/abc", want: "port-invalid-uint"},
		{in: "Ethernet0", want: "port-invalid-prefix"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, swp(tt.in))
		})
	}
}

func minimalAgent() *agentapi.Agent {
	agent := &agentapi.Agent{
		Spec: agentapi.AgentSpec{
			Config: agentapi.AgentSpecConfig{
				ControlVIP: "172.30.1.1/32",
			},
			Switch: wiringapi.SwitchSpec{
				Profile:    switchprofile.CmlsNvidiaSN5610.Name,
				IP:         "172.30.0.10/21",
				ProtocolIP: "172.30.8.5/32",
				ASN:        65101,
			},
			SwitchProfile: &switchprofile.CmlsNvidiaSN5610.Spec,
			Users: []agentapi.UserCreds{
				{Name: "admin", Password: "$5$hash", Role: "admin", SSHKeys: []string{"ssh-ed25519 AAAA key"}},
				{Name: "op", Password: "$5$hash", Role: "operator"},
			},
		},
	}
	agent.Name = "test-switch"

	return agent
}

func TestBuildConfigForUsers(t *testing.T) {
	agentUser := &User{Name: nvue.AgentUser, HashedPassword: "$2a$10$hash", Role: nvue.AgentRole, SSHKeys: []SSHKey{}}

	cfgBuf, err := buildConfigFor(fullCfgTmpl, minimalAgent(), agentUser)
	require.NoError(t, err)

	var cfg []map[string]any
	require.NoError(t, kyaml.Unmarshal(cfgBuf.Bytes(), &cfg))

	requireNoNulls(t, cfg, "")

	users := cfg[0]["set"].(map[string]any)["system"].(map[string]any)["aaa"].(map[string]any)["user"].(map[string]any)
	require.Len(t, users, 3)
	// the agent's account has no keys, and an empty authorized-key would be a null that nv config replace rejects
	require.Equal(t, map[string]any{
		"hashed-password": "$2a$10$hash",
		"role":            nvue.AgentRole,
	}, users[nvue.AgentUser])
	require.Equal(t, "system-admin", users["admin"].(map[string]any)["role"])
	require.Equal(t, "nvue-monitor", users["op"].(map[string]any)["role"])
	require.Contains(t, users["admin"].(map[string]any)["ssh"], "authorized-key")

	// ZTP runs before the agent has its account
	ztpBuf, err := buildConfigFor(ztpCfgTmpl, minimalAgent(), nil)
	require.NoError(t, err)
	require.NotContains(t, ztpBuf.String(), nvue.AgentUser)

	var ztpCfg []map[string]any
	require.NoError(t, kyaml.Unmarshal(ztpBuf.Bytes(), &ztpCfg))
	requireNoNulls(t, ztpCfg, "")

	// the name is taken
	agent := minimalAgent()
	agent.Spec.Users = append(agent.Spec.Users, agentapi.UserCreds{Name: nvue.AgentUser, Password: "$5$x", Role: "operator"})
	_, err = buildConfigFor(fullCfgTmpl, agent, agentUser)
	require.ErrorContains(t, err, "reserved")

	// the front port count comes from the profile, there's no config without one
	agent = minimalAgent()
	agent.Spec.SwitchProfile = nil
	_, err = buildConfigFor(fullCfgTmpl, agent, agentUser)
	require.ErrorIs(t, err, errNoSwitchProfile)
}

func TestAgentUserFor(t *testing.T) {
	basedir := t.TempDir()

	user, err := agentUserFor(basedir)
	require.NoError(t, err)
	require.Equal(t, nvue.AgentUser, user.Name)
	require.Equal(t, nvue.AgentRole, user.Role)
	require.True(t, strings.HasPrefix(user.HashedPassword, "$2a$"), user.HashedPassword)

	// nothing changes between applies
	again, err := agentUserFor(basedir)
	require.NoError(t, err)
	require.Equal(t, user, again)
}

// requireNoNulls fails on a null anywhere in a rendered config: NVUE rejects a set with a null value, which is what a
// template key with nothing rendered under it becomes
func requireNoNulls(t *testing.T, v any, path string) {
	t.Helper()

	switch v := v.(type) {
	case nil:
		require.Fail(t, "null value in config", "at %s", path)
	case map[string]any:
		for k, child := range v {
			requireNoNulls(t, child, path+"/"+k)
		}
	case []map[string]any:
		for i, child := range v {
			requireNoNulls(t, child, fmt.Sprintf("%s[%d]", path, i))
		}
	case []any:
		for i, child := range v {
			requireNoNulls(t, child, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}
