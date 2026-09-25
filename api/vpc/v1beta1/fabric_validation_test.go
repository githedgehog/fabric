// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// nsOther is the IPv4Namespace of the fabric named "other"
const nsOther = "ns-other"

// Every object below is in the default fabric except the ones suffixed -other, which are in a
// fabric named "other". The objects in the default fabric leave the reference empty rather than
// setting it, which is what an object stored before the reference existed looks like.
func otherFabricObjs() []kclient.Object {
	return []kclient.Object{
		&v1beta1.IPv4Namespace{
			ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
			Spec:       v1beta1.IPv4NamespaceSpec{Subnets: []string{"10.0.0.0/16"}},
		},
		&v1beta1.IPv4Namespace{
			ObjectMeta: kmetav1.ObjectMeta{Name: nsOther, Namespace: kmetav1.NamespaceDefault},
			Spec: v1beta1.IPv4NamespaceSpec{
				Topology: v1beta1.IPv4NamespaceTopology{Fabric: "other"},
				Subnets:  []string{"10.1.0.0/16"},
			},
		},
		vpcGen("vpc-01"),
		vpcGen("vpc-other", func(vpc *v1beta1.VPC) {
			vpc.Spec.Topology.Fabric = "other"
			vpc.Spec.IPv4Namespace = nsOther
		}),
		extGen("ext-other", func(ext *v1beta1.External) {
			ext.Spec.Topology.Fabric = "other"
			ext.Spec.IPv4Namespace = nsOther
		}),
		&wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01--external", Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{
				External: &wiringapi.ConnExternal{
					Link: wiringapi.ConnExternalLink{
						Switch: wiringapi.BasePortName{Port: "leaf-01/E1/1"},
					},
				},
			},
		},
		&wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01--unbundled--server-01", Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{
				Unbundled: &wiringapi.ConnUnbundled{
					Link: wiringapi.ServerToSwitchLink{
						Server: wiringapi.NewBasePortName("server-01/enp2s1"),
						Switch: wiringapi.NewBasePortName("leaf-01/E1/2"),
					},
				},
			},
		},
	}
}

func TestFabricMismatchValidation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(otherFabricObjs()...).
		Build()

	for _, tt := range []struct {
		name string
		obj  meta.Validatable
	}{
		{
			name: "vpc against its ipv4 namespace",
			obj: vpcGen("vpc-bad", func(vpc *v1beta1.VPC) {
				vpc.Spec.IPv4Namespace = nsOther
			}),
		},
		{
			name: "external against its ipv4 namespace",
			obj: extGen("ext-bad", func(ext *v1beta1.External) {
				ext.Spec.IPv4Namespace = nsOther
			}),
		},
		{
			name: "vpc attachment against its vpc",
			obj: vpcAttachGen("attach-mismatch", func(attach *v1beta1.VPCAttachment) {
				attach.Spec.Subnet = "vpc-other/default"
			}),
		},
		{
			name: "vpc peering against one of its vpcs",
			obj:  vpcPeeringGen("vpc-peer-mismatch", "vpc-01", "vpc-other"),
		},
		{
			name: "external attachment against its external",
			obj: l3ExtAttGen("ext-att-mismatch", func(att *v1beta1.ExternalAttachment) {
				att.Spec.External = "ext-other"
			}),
		},
		{
			name: "external peering against its external",
			obj: extPeeringGen("ext-peer-mismatch", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.VPC.Name = "vpc-01"
				peering.Spec.Permit.VPC.Subnets = []string{"default"}
				peering.Spec.Permit.External.Name = "ext-other"
			}),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.obj.Validate(t.Context(), kube, &meta.FabricConfig{})
			require.ErrorContains(t, err, "is in fabric other")
		})
	}
}

func vpcAttachGen(name string, f ...func(attach *v1beta1.VPCAttachment)) *v1beta1.VPCAttachment {
	base := &v1beta1.VPCAttachment{
		ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
		Spec: v1beta1.VPCAttachmentSpec{
			Subnet:     "vpc-01/default",
			Connection: "leaf-01--unbundled--server-01",
		},
	}

	for _, fn := range f {
		fn(base)
	}
	base.Default()

	return base
}

func vpcPeeringGen(name, vpc1, vpc2 string) *v1beta1.VPCPeering {
	base := &v1beta1.VPCPeering{
		ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
		Spec: v1beta1.VPCPeeringSpec{
			Permit: []map[string]v1beta1.VPCPeer{{
				vpc1: {},
				vpc2: {},
			}},
		},
	}
	base.Default()

	return base
}

func TestFabricMustExistValidation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(otherFabricObjs()...).
		Build()

	for _, tt := range []struct {
		name string
		obj  meta.Validatable
	}{
		{
			name: "vpc",
			obj:  vpcGen("vpc-bad", func(vpc *v1beta1.VPC) { vpc.Spec.Topology.Fabric = "nope" }),
		},
		{
			name: "ipv4 namespace",
			obj: &v1beta1.IPv4Namespace{
				ObjectMeta: kmetav1.ObjectMeta{Name: "ns-bad", Namespace: kmetav1.NamespaceDefault},
				Spec: v1beta1.IPv4NamespaceSpec{
					Topology: v1beta1.IPv4NamespaceTopology{Fabric: "nope"},
					Subnets:  []string{"10.2.0.0/16"},
				},
			},
		},
		{
			name: "external peering",
			obj: extPeeringGen("ext-peer-bad", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Topology.Fabric = "nope"
			}),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.obj.Validate(t.Context(), kube, &meta.FabricConfig{})
			require.ErrorContains(t, err, "fabric nope not found")
		})
	}
}
