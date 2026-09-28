// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"slices"
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

func TestSwitchDomainChange(t *testing.T) {
	const planeB = "plane-b"

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	objMeta := func(name string) kmetav1.ObjectMeta {
		return kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}
	}
	sw := func(name string, role wiringapi.SwitchRole, domains ...string) *wiringapi.Switch {
		return &wiringapi.Switch{
			ObjectMeta: objMeta(name),
			Spec:       wiringapi.SwitchSpec{Role: role, Topology: wiringapi.SwitchTopology{Domains: domains}},
		}
	}
	unbundled := func(leaf string) *wiringapi.Connection {
		return &wiringapi.Connection{ObjectMeta: objMeta(leaf + "--unbundled"), Spec: wiringapi.ConnectionSpec{Unbundled: &wiringapi.ConnUnbundled{
			Link: wiringapi.ServerToSwitchLink{
				Server: wiringapi.NewBasePortName("server-01/enp2s1"),
				Switch: wiringapi.NewBasePortName(leaf + "/E1/1"),
			},
		}}}
	}
	attach := func(leaf string) *vpcapi.VPCAttachment {
		return &vpcapi.VPCAttachment{
			ObjectMeta: objMeta("vpc-01--" + leaf),
			Spec:       vpcapi.VPCAttachmentSpec{Subnet: "vpc-01/default", Connection: leaf + "--unbundled"},
		}
	}
	fabricConn := &wiringapi.Connection{ObjectMeta: objMeta("spine-a--fabric--leaf-01"), Spec: wiringapi.ConnectionSpec{Fabric: &wiringapi.ConnFabric{
		Links: []wiringapi.FabricLink{{
			Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "spine-a/E1/1"}},
			Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-01/E1/2"}},
		}},
	}}}
	gwConn := &wiringapi.Connection{ObjectMeta: objMeta("leaf-03--gateway--gw-1"), Spec: wiringapi.ConnectionSpec{Gateway: &wiringapi.ConnGateway{
		Links: []wiringapi.GatewayLink{{
			Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-03/E1/1"}},
			Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.BasePortName{Port: "gw-1/enp2s1"}},
		}},
	}}}

	staticExt := &wiringapi.Connection{ObjectMeta: objMeta("leaf-03--static-external"), Spec: wiringapi.ConnectionSpec{StaticExternal: &wiringapi.ConnStaticExternal{
		WithinVPC: "vpc-01",
		Link:      wiringapi.ConnStaticExternalLink{Switch: wiringapi.ConnStaticExternalLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-03/E1/2"}}},
	}}}

	base := []kclient.Object{
		sw("spine-a", wiringapi.SwitchRoleSpine),
		sw("leaf-01", wiringapi.SwitchRoleServerLeaf),
		sw("leaf-02", wiringapi.SwitchRoleServerLeaf),
		sw("leaf-03", wiringapi.SwitchRoleServerLeaf),
		unbundled("leaf-01"),
		unbundled("leaf-02"),
		&vpcapi.VPC{ObjectMeta: objMeta("vpc-01")},
		attach("leaf-01"),
		attach("leaf-02"),
		&gwapi.Gateway{ObjectMeta: objMeta("gw-1")},
	}

	for _, tt := range []struct {
		name    string
		sw      *wiringapi.Switch
		objects []kclient.Object
		err     string
	}{
		{
			name:    "leaf joins a second domain",
			sw:      sw("leaf-01", wiringapi.SwitchRoleServerLeaf, wiringapi.DefaultFabricDomain, planeB),
			objects: []kclient.Object{fabricConn},
		},
		{
			name:    "leaf leaves the domain of its spine",
			sw:      sw("leaf-01", wiringapi.SwitchRoleServerLeaf, planeB),
			objects: []kclient.Object{fabricConn},
			err:     "connection spine-a--fabric--leaf-01: spine spine-a is in domain default but leaf leaf-01 is in domains [plane-b]",
		},
		{
			name: "leaf moves away from the rest of its vpc",
			sw:   sw("leaf-01", wiringapi.SwitchRoleServerLeaf, planeB),
			err:  "vpc vpc-01: switch leaf-0",
		},
		{
			name:    "spine moves away from its leaves",
			sw:      sw("spine-a", wiringapi.SwitchRoleSpine, planeB),
			objects: []kclient.Object{fabricConn},
			err:     "spine spine-a is in domain plane-b but leaf leaf-01 is in domains [default]",
		},
		{
			name:    "gateway leaf joins a second domain",
			sw:      sw("leaf-03", wiringapi.SwitchRoleServerLeaf, wiringapi.DefaultFabricDomain, planeB),
			objects: []kclient.Object{gwConn},
			err:     "with a gateway connection must be in exactly one domain",
		},
		{
			name:    "gateway leaf moves away from its gateway",
			sw:      sw("leaf-03", wiringapi.SwitchRoleServerLeaf, planeB),
			objects: []kclient.Object{gwConn},
			err:     "connection leaf-03--gateway--gw-1 cables it to gateway gw-1 in domain default",
		},
		{
			name:    "static external leaf moves away from the rest of its vpc",
			sw:      sw("leaf-03", wiringapi.SwitchRoleServerLeaf, planeB),
			objects: []kclient.Object{staticExt},
			err:     "vpc vpc-01: switch leaf-0",
		},
		{
			name: "switch with nothing cabled",
			sw:   sw("leaf-03", wiringapi.SwitchRoleServerLeaf, planeB),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			objects := []kclient.Object{}
			for _, obj := range append(slices.Clone(base), tt.objects...) {
				// the lookups go by the labels admission sets
				obj = obj.DeepCopyObject().(kclient.Object) //nolint:forcetypeassert
				if defaulter, ok := obj.(interface{ Default() }); ok {
					defaulter.Default()
				}
				objects = append(objects, obj)
			}
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			w := &SwitchWebhook{KubeClient: kube}

			err := w.validateDomainChange(t.Context(), tt.sw)
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}
