// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestPortConnection(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	// stored connections are always defaulted, which sets the switch list labels
	connGen := func(name string, spec wiringapi.ConnectionSpec) kclient.Object {
		conn := &wiringapi.Connection{Spec: spec}
		conn.Name, conn.Namespace = name, kmetav1.NamespaceDefault
		conn.Default()

		return conn
	}
	unbundled := func(server, sw string) wiringapi.ConnectionSpec {
		return wiringapi.ConnectionSpec{Unbundled: &wiringapi.ConnUnbundled{Link: wiringapi.ServerToSwitchLink{
			Server: wiringapi.NewBasePortName(server),
			Switch: wiringapi.NewBasePortName(sw),
		}}}
	}

	connSelector := ""
	kube := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, kube kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			if _, ok := list.(*wiringapi.ConnectionList); ok {
				listOpts := &kclient.ListOptions{}
				listOpts.ApplyOptions(opts)
				connSelector = fmt.Sprint(listOpts.LabelSelector)
			}

			return kube.List(ctx, list, opts...)
		},
	}).WithObjects(
		connGen("server-1--unbundled--leaf-1", unbundled("server-1/enp2s1", "leaf-1/E1/1")),
		connGen("server-2--unbundled--leaf-2", unbundled("server-2/enp2s1", "leaf-2/E1/1")),
		connGen("spine-1--fabric--leaf-1", wiringapi.ConnectionSpec{Fabric: &wiringapi.ConnFabric{Links: []wiringapi.FabricLink{{
			Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("spine-1/E1/3")},
			Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("leaf-1/E1/2")},
		}}}}),
	).Build()

	for port, conn := range map[string]string{
		"leaf-1/E1/1":  "server-1--unbundled--leaf-1",
		"leaf-2/E1/1":  "server-2--unbundled--leaf-2",
		"leaf-1/E1/2":  "spine-1--fabric--leaf-1",
		"spine-1/E1/3": "spine-1--fabric--leaf-1",
		"leaf-1/E1/4":  "",
	} {
		t.Run(port, func(t *testing.T) {
			out, err := Port(t.Context(), kube, PortIn{Port: port})
			require.NoError(t, err)
			swName, _, _ := strings.Cut(port, "/")
			require.Equal(t, wiringapi.ListLabelSwitch(swName)+"="+wiringapi.ListLabelValue, connSelector, "only the switch's connections should be listed")
			if conn == "" {
				require.Nil(t, out.ConnectionName)
			} else {
				require.NotNil(t, out.ConnectionName)
				require.Equal(t, conn, *out.ConnectionName)
			}
		})
	}
}

func TestPortCollected(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	now := time.Now().Truncate(time.Second)
	sw := func(name string) kclient.Object {
		return &wiringapi.Switch{Name: name, Namespace: kmetav1.NamespaceDefault}
	}
	agent := func(name string, heartbeat time.Time) kclient.Object {
		return &agentapi.Agent{
			Name: name, Namespace: kmetav1.NamespaceDefault,
			Status: agentapi.AgentStatus{LastHeartbeat: kmetav1.Time{Time: heartbeat}},
		}
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		sw("leaf-1"), agent("leaf-1", now.Add(-10*time.Second)),
		sw("leaf-2"), agent("leaf-2", now.Add(-5*time.Minute)),
		// leaf-3 has no agent yet
		sw("leaf-3"),
	).Build()

	for _, tt := range []struct {
		port   string
		stale  bool
		header string
	}{
		{port: "leaf-1/E1/1", header: "Switch: leaf-1 (collected 10 seconds ago)"},
		{port: "leaf-2/E1/1", stale: true, header: "Switch: leaf-2 (collected 5 minutes ago, STALE: the agent missed heartbeats)"},
		{port: "leaf-3/E1/1"},
	} {
		t.Run(tt.port, func(t *testing.T) {
			in := PortIn{Port: tt.port}
			out, err := Port(t.Context(), kube, in)
			require.NoError(t, err)

			text, err := out.MarshalText(in, now)
			require.NoError(t, err)

			if tt.header == "" {
				require.Nil(t, out.Collected, "without an agent there is no state")
				require.NotContains(t, text, "Switch:")

				return
			}

			require.NotNil(t, out.Collected)
			require.Equal(t, tt.stale, out.Collected.Stale)
			require.Contains(t, text, tt.header+"\n")
		})
	}
}
