// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
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
			if conn == "" {
				require.Nil(t, out.ConnectionName)
			} else {
				require.NotNil(t, out.ConnectionName)
				require.Equal(t, conn, *out.ConnectionName)
			}
		})
	}
}
