// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestIP(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	gw := &gwapi.Gateway{Spec: gwapi.GatewaySpec{
		ProtocolIP: "172.30.8.1/32",
		VTEPIP:     "172.30.12.1/32",
		Interfaces: map[string]gwapi.GatewayInterface{
			"enp2s1": {IPs: []string{"172.30.128.11/31"}},
		},
	}}
	gw.Name, gw.Namespace = "gw-1", kmetav1.NamespaceDefault

	// stored connections are always defaulted, which sets the connection type label
	connGen := func(name string, spec wiringapi.ConnectionSpec) kclient.Object {
		conn := &wiringapi.Connection{Spec: spec}
		conn.Name, conn.Namespace = name, kmetav1.NamespaceDefault
		conn.Default()

		return conn
	}

	var connSelector labels.Selector
	kube := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, kube kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			if _, ok := list.(*wiringapi.ConnectionList); ok {
				listOpts := &kclient.ListOptions{}
				listOpts.ApplyOptions(opts)
				connSelector = listOpts.LabelSelector
			}

			return kube.List(ctx, list, opts...)
		},
	}).WithObjects(
		gw,
		connGen("spine-1--gateway--gw-1", wiringapi.ConnectionSpec{Gateway: &wiringapi.ConnGateway{Links: []wiringapi.GatewayLink{{
			Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("spine-1/E1/7"), IP: "172.30.128.10/31"},
			Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.NewBasePortName("gw-1/enp2s1"), IP: "172.30.128.11/31"},
		}}}}),
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

	for ip, tt := range map[string]struct {
		conn, gw string
	}{
		"172.30.128.0":  {conn: "spine-1--fabric--leaf-1"},
		"172.30.128.1":  {conn: "spine-1--fabric--leaf-1"},
		"172.30.129.1":  {conn: "leaf-1--mesh--leaf-2"},
		"172.31.0.2":    {conn: "leaf-1--static-external"},
		"10.99.1.1":     {conn: "leaf-1--static-external"},
		"172.30.128.10": {conn: "spine-1--gateway--gw-1"},
		"172.30.128.11": {conn: "spine-1--gateway--gw-1", gw: "gw-1"},
		"172.30.8.1":    {gw: "gw-1"},
		"172.30.12.1":   {gw: "gw-1"},
		"172.30.130.1":  {},
	} {
		t.Run(ip, func(t *testing.T) {
			connSelector = nil
			out, err := IP(t.Context(), kube, IPIn{IP: ip})
			require.NoError(t, err)
			require.NotNil(t, connSelector, "connections should be listed by type")
			for _, connType := range []string{wiringapi.ConnectionTypeFabric, wiringapi.ConnectionTypeMesh, wiringapi.ConnectionTypeGateway, wiringapi.ConnectionTypeStaticExternal} {
				require.True(t, connSelector.Matches(labels.Set{wiringapi.LabelConnectionType: connType}), connType)
			}
			require.False(t, connSelector.Matches(labels.Set{wiringapi.LabelConnectionType: wiringapi.ConnectionTypeUnbundled}))
			if tt.conn == "" {
				require.Empty(t, out.Connections)
			} else {
				require.Len(t, out.Connections, 1)
				require.Equal(t, tt.conn, out.Connections[0].Name)
			}
			if tt.gw == "" {
				require.Nil(t, out.Gateway)
			} else {
				require.NotNil(t, out.Gateway)
				require.Equal(t, tt.gw, out.Gateway.Name)
			}
		})
	}
}
