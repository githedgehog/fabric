// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
