// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSwitchPortsBreakout(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	sw := &wiringapi.Switch{Spec: wiringapi.SwitchSpec{Profile: "profile"}}
	sw.Name, sw.Namespace = "leaf-1", kmetav1.NamespaceDefault

	sp := &wiringapi.SwitchProfile{}
	sp.Name, sp.Namespace = "profile", kmetav1.NamespaceDefault

	// the connection uses the port name, the agent reports the interface of the port in a 1x breakout
	conn := &wiringapi.Connection{Spec: wiringapi.ConnectionSpec{Unbundled: &wiringapi.ConnUnbundled{
		Link: wiringapi.ServerToSwitchLink{
			Server: wiringapi.NewBasePortName("server-1/enp2s1"),
			Switch: wiringapi.NewBasePortName("leaf-1/E1/29"),
		},
	}}}
	conn.Name, conn.Namespace = "server-1--unbundled--leaf-1", kmetav1.NamespaceDefault
	conn.Default()

	agent := &agentapi.Agent{Status: agentapi.AgentStatus{State: agentapi.SwitchState{
		Interfaces: map[string]agentapi.SwitchStateInterface{
			"E1/29/1": {AdminStatus: agentapi.AdminStatusUp, OperStatus: agentapi.OperStatusUp, Speed: "400G"},
		},
		Breakouts:    map[string]agentapi.SwitchStateBreakout{"E1/29": {Mode: "1x800G"}},
		Transceivers: map[string]agentapi.SwitchStateTransceiver{"E1/29": {OperStatus: "active"}},
	}}}
	agent.Name, agent.Namespace = "leaf-1", kmetav1.NamespaceDefault

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sw, sp, conn, agent).Build()

	out, err := Switch(t.Context(), kube, SwitchIn{Name: "leaf-1"})
	require.NoError(t, err)
	require.Len(t, out.Ports, 2)

	cage := out.Ports[0]
	require.Equal(t, "E1/29", cage.PortName)
	require.Empty(t, cage.ConnectionName)
	require.Nil(t, cage.InterfaceState)
	require.NotNil(t, cage.BreakoutState)
	require.NotNil(t, cage.TransceiverState)

	iface := out.Ports[1]
	require.Equal(t, "E1/29/1", iface.PortName)
	require.Equal(t, "server-1--unbundled--leaf-1", iface.ConnectionName)
	require.NotNil(t, iface.InterfaceState)
	require.Nil(t, iface.BreakoutState)
	require.Nil(t, iface.TransceiverState)
}
