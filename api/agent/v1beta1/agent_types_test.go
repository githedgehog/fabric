// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/require"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kyaml "sigs.k8s.io/yaml"
)

func TestAgentMarshalInstallConfig(t *testing.T) {
	agent := &Agent{
		Name:          "leaf-1",
		Namespace:     kmetav1.NamespaceDefault,
		Labels:        map[string]string{"some": "label"},
		ManagedFields: []kmetav1.ManagedFieldsEntry{{Manager: "fabric-ctrl"}},
		Spec:          AgentSpec{Description: "leaf one", Version: AgentVersion{Default: "v1.2.3"}},
		Status:        AgentStatus{RunID: "old-run-id", Version: "v1.2.2"},
	}
	orig := agent.DeepCopy()

	data, err := agent.MarshalInstallConfig()
	require.NoError(t, err)
	require.NotContains(t, string(data), "old-run-id")
	require.NotContains(t, string(data), "managedFields")
	require.NotContains(t, string(data), "fabric-ctrl")

	got := &Agent{}
	require.NoError(t, kyaml.UnmarshalStrict(data, got))
	require.Equal(t, &Agent{
		APIVersion: GroupVersion.String(), Kind: KindAgent,
		Name:      "leaf-1",
		Namespace: kmetav1.NamespaceDefault,
		Labels:    map[string]string{"some": "label"},
		Spec:      orig.Spec,
	}, got)

	require.Equal(t, orig, agent, "the agent itself is left as is")
}

func TestAgentStatusBuildNotes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		status   AgentStatus
		expected string
	}{
		{name: "empty"},
		{
			name:     "mutated without applied rules",
			status:   AgentStatus{Mutate: MutateStatus{Enabled: true}},
			expected: "mutated (0)",
		},
		{
			name:   "mutate disabled",
			status: AgentStatus{Mutate: MutateStatus{Applied: []string{"keep@acl/x"}}},
		},
		{
			name:     "reboot required",
			status:   AgentStatus{RebootRequired: true},
			expected: "reboot-required",
		},
		{
			name: "all",
			status: AgentStatus{
				Mutate:         MutateStatus{Enabled: true, Applied: []string{"keep@acl/x", "override@routeMap/y/1"}},
				RebootRequired: true,
			},
			expected: "mutated (2), reboot-required",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.status.BuildNotes())
		})
	}
}
