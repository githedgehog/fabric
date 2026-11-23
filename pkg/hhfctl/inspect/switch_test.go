// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSwitchStateSummaryStale(t *testing.T) {
	agent := func(heartbeat time.Time) *agentapi.Agent {
		return &agentapi.Agent{Status: agentapi.AgentStatus{LastHeartbeat: kmetav1.Time{Time: heartbeat}}}
	}

	require.False(t, switchStateSummary(agent(time.Now())).Stale)
	require.True(t, switchStateSummary(agent(time.Now().Add(-5*time.Minute))).Stale)
	require.True(t, switchStateSummary(agent(time.Time{})).Stale, "an agent that never reported is stale")
	require.False(t, switchStateSummary(nil).Stale, "without an agent there is nothing to be stale")
}

func TestFabricOutHeartbeat(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	sw := func(name string, heartbeat time.Time, stale bool) *FabricOutSwitch {
		return &FabricOutSwitch{Name: name, State: &AgentState{LastHeartbeat: kmetav1.Time{Time: heartbeat}, Stale: stale}}
	}

	out := &FabricOut{Switches: []*FabricOutSwitch{
		sw("leaf-1", now.Add(-10*time.Second), false),
		sw("leaf-2", now.Add(-5*time.Minute), true),
		sw("leaf-3", time.Time{}, true),
	}}

	text, err := out.MarshalText(FabricIn{}, now)
	require.NoError(t, err)
	require.Contains(t, text, "10 seconds ago")
	require.NotContains(t, text, "10 seconds ago (stale)")
	require.Contains(t, text, "5 minutes ago (stale)")
	require.Contains(t, text, "never (stale)")
}

func TestSwitchWithoutAgent(t *testing.T) {
	scheme := fabricScheme(t)

	sw := &wiringapi.Switch{
		Name: "leaf-1", Namespace: kmetav1.NamespaceDefault,
		Spec: wiringapi.SwitchSpec{Profile: meta.SwitchProfileVS},
	}
	profile := switchprofile.VS.DeepCopy()
	profile.Namespace = kmetav1.NamespaceDefault

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sw, profile).Build()

	out, err := Switch(t.Context(), kube, SwitchIn{Name: "leaf-1"})
	require.NoError(t, err)
	require.Equal(t, "Unknown", out.State.Summary)

	text, err := out.MarshalText(SwitchIn{}, time.Now())
	require.NoError(t, err)
	require.Contains(t, text, "never")
	require.NotContains(t, text, "(stale)")
}
