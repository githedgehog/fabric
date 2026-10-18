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
		ObjectMeta: kmetav1.ObjectMeta{
			Name:          "leaf-1",
			Namespace:     kmetav1.NamespaceDefault,
			Labels:        map[string]string{"some": "label"},
			ManagedFields: []kmetav1.ManagedFieldsEntry{{Manager: "fabric-ctrl"}},
		},
		Spec:   AgentSpec{Description: "leaf one", Version: AgentVersion{Default: "v1.2.3"}},
		Status: AgentStatus{RunID: "old-run-id", Version: "v1.2.2"},
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
		TypeMeta: kmetav1.TypeMeta{APIVersion: GroupVersion.String(), Kind: KindAgent},
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      "leaf-1",
			Namespace: kmetav1.NamespaceDefault,
			Labels:    map[string]string{"some": "label"},
		},
		Spec: orig.Spec,
	}, got)

	require.Equal(t, orig, agent, "the agent itself is left as is")
}
