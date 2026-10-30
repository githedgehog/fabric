// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"testing"

	"github.com/stretchr/testify/require"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIPConnections(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	// stored connections are always defaulted, which sets the connection type label
	connGen := func(name string, spec wiringapi.ConnectionSpec) kclient.Object {
		conn := &wiringapi.Connection{Spec: spec}
		conn.Name, conn.Namespace = name, kmetav1.NamespaceDefault
		conn.Default()

		return conn
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		connGen("spine-1--fabric--leaf-1", wiringapi.ConnectionSpec{Fabric: &wiringapi.ConnFabric{Links: []wiringapi.FabricLink{{
			Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("spine-1/E1/3"), IP: "172.30.128.0/31"},
			Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("leaf-1/E1/2"), IP: "172.30.128.1/31"},
		}}}}),
		connGen("leaf-1--mesh--leaf-2", wiringapi.ConnectionSpec{Mesh: &wiringapi.ConnMesh{Links: []wiringapi.MeshLink{{
			Leaf1: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("leaf-1/E1/5"), IP: "172.30.129.0/31"},
			Leaf2: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("leaf-2/E1/5"), IP: "172.30.129.1/31"},
		}}}}),
		connGen("leaf-1--static-external", wiringapi.ConnectionSpec{StaticExternal: &wiringapi.ConnStaticExternal{Link: wiringapi.ConnStaticExternalLink{
			Switch: wiringapi.ConnStaticExternalLinkSwitch{
				BasePortName: wiringapi.NewBasePortName("leaf-1/E1/6"),
				IP:           "172.31.0.1/24",
				NextHop:      "172.31.0.2",
				Subnets:      []string{"10.99.0.0/16"},
			},
		}}}),
		connGen("server-1--unbundled--leaf-1", wiringapi.ConnectionSpec{Unbundled: &wiringapi.ConnUnbundled{Link: wiringapi.ServerToSwitchLink{
			Server: wiringapi.NewBasePortName("server-1/enp2s1"),
			Switch: wiringapi.NewBasePortName("leaf-1/E1/1"),
		}}}),
	).Build()

	for ip, conn := range map[string]string{
		"172.30.128.0": "spine-1--fabric--leaf-1",
		"172.30.128.1": "spine-1--fabric--leaf-1",
		"172.30.129.1": "leaf-1--mesh--leaf-2",
		"172.31.0.2":   "leaf-1--static-external",
		"10.99.1.1":    "leaf-1--static-external",
		"172.30.130.1": "",
	} {
		t.Run(ip, func(t *testing.T) {
			out, err := IP(t.Context(), kube, IPIn{IP: ip})
			require.NoError(t, err)
			if conn == "" {
				require.Empty(t, out.Connections)
			} else {
				require.Len(t, out.Connections, 1)
				require.Equal(t, conn, out.Connections[0].Name)
			}
		})
	}
}
