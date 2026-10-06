// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConnectionReferencesFabric(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	vpc := &vpcapi.VPC{
		ObjectMeta: kmetav1.ObjectMeta{Name: "vpc-01", Namespace: kmetav1.NamespaceDefault},
		Spec:       vpcapi.VPCSpec{Topology: vpcapi.VPCTopology{Fabric: "backend"}},
	}
	gw := &gwapi.Gateway{
		ObjectMeta: kmetav1.ObjectMeta{Name: "gateway-1", Namespace: kmetav1.NamespaceDefault},
		Spec:       gwapi.GatewaySpec{Topology: gwapi.GatewayTopology{Fabric: "backend"}},
	}
	gwPlaneB := gw.DeepCopy()
	gwPlaneB.Spec.Topology.Domain = "plane-b"
	spine := &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Name: "spine-01", Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.SwitchSpec{Topology: wiringapi.SwitchTopology{Fabric: "backend"}},
	}

	vpcOnB := vpc.DeepCopy()
	vpcOnB.Spec.Topology.Domains = []string{"plane-b"}
	leaf := &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01", Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.SwitchSpec{Topology: wiringapi.SwitchTopology{Fabric: "backend"}},
	}
	leafOnB := leaf.DeepCopy()
	leafOnB.Spec.Topology.Domains = []string{"plane-b"}

	// stored objects are always defaulted, the ones created without a domain are in the default one
	for _, obj := range []interface{ Default() }{vpc, vpcOnB, gw, gwPlaneB, spine, leaf, leafOnB} {
		obj.Default()
	}

	staticExternal := func(fabricName string) *wiringapi.Connection {
		return &wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01--static-external", Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{
				Topology: wiringapi.ConnectionTopology{Fabric: fabricName},
				StaticExternal: &wiringapi.ConnStaticExternal{
					WithinVPC: vpc.Name,
					Link:      wiringapi.ConnStaticExternalLink{Switch: wiringapi.ConnStaticExternalLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-01/E1/1"}}},
				},
			},
		}
	}
	gateway := func(fabricName string) *wiringapi.Connection {
		return &wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: "spine-01--gateway--gateway-1", Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{
				Topology: wiringapi.ConnectionTopology{Fabric: fabricName},
				Gateway: &wiringapi.ConnGateway{Links: []wiringapi.GatewayLink{{
					Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "spine-01/E1/1"}},
					Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.BasePortName{Port: "gateway-1/enp2s1"}},
				}}},
			},
		}
	}

	for _, tt := range []struct {
		name    string
		conn    *wiringapi.Connection
		objects []kclient.Object
		err     string
	}{
		{name: "static external, vpc in the same fabric", conn: staticExternal("backend"), objects: []kclient.Object{vpc, leaf}},
		{
			name: "static external, vpc in another domain", conn: staticExternal("backend"), objects: []kclient.Object{vpcOnB, leaf},
			err: "vpc vpc-01 is in domains [plane-b] but switch leaf-01 is in domains [default]",
		},
		{
			name: "static external, vpc in the default domain on a leaf in another domain", conn: staticExternal("backend"), objects: []kclient.Object{vpc, leafOnB},
			err: "vpc vpc-01 is in domains [default] but switch leaf-01 is in domains [plane-b]",
		},
		{name: "static external, vpc in another fabric", conn: staticExternal("default"), objects: []kclient.Object{vpc}, err: "vpc vpc-01 is in fabric backend"},
		{name: "gateway in the same fabric", conn: gateway("backend"), objects: []kclient.Object{gw, spine}},
		{name: "gateway in another domain", conn: gateway("backend"), objects: []kclient.Object{gwPlaneB, spine}, err: "gateway gateway-1 is in domain plane-b but switch spine-01 is in domains [default]"},
		{name: "gateway in another fabric", conn: gateway("default"), objects: []kclient.Object{gw}, err: "gateway gateway-1 is in fabric backend"},
		{name: "gateway not created yet", conn: gateway("default"), err: "gateway gateway-1 not found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			w := &ConnectionWebhook{}

			err := w.validateStaticExternal(t.Context(), kube, tt.conn)
			if err == nil {
				err = w.validateGateway(t.Context(), kube, tt.conn)
			}
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}
