// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"context"
	"errors"
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

func extPeeringGen(name string, f ...func(peering *v1beta1.ExternalPeering)) *v1beta1.ExternalPeering {
	base := &v1beta1.ExternalPeering{
		Name:      name,
		Namespace: kmetav1.NamespaceDefault,
		Spec: v1beta1.ExternalPeeringSpec{
			Permit: v1beta1.ExternalPeeringSpecPermit{
				VPC: v1beta1.ExternalPeeringSpecVPC{
					Name:    "vpc-01",
					Subnets: []string{"subnet-a"},
				},
				External: v1beta1.ExternalPeeringSpecExternal{
					Name: "external-01",
					Prefixes: []v1beta1.ExternalPeeringSpecPrefix{
						{Prefix: "0.0.0.0/0"},
					},
				},
			},
		},
	}

	for _, fn := range f {
		fn(base)
	}
	base.Default()

	return base
}

func TestExternalPeeringValidation(t *testing.T) {
	baseObjs := []kclient.Object{
		fabricObj(wiringapi.DefaultFabric, &meta.FabricConfig{}),
		defaulted(&v1beta1.VPC{
			Name:      "vpc-01",
			Namespace: kmetav1.NamespaceDefault,
			Spec: v1beta1.VPCSpec{
				IPv4Namespace: "default",
				Subnets: map[string]*v1beta1.VPCSubnet{
					"subnet-a": {Subnet: "10.0.1.0/24", VLAN: 101},
					"subnet-b": {Subnet: "10.0.2.0/24", VLAN: 102},
				},
			},
		}),
		defaulted(&v1beta1.External{
			Name:      "external-01",
			Namespace: kmetav1.NamespaceDefault,
			Spec: v1beta1.ExternalSpec{
				IPv4Namespace:     "default",
				InboundCommunity:  "50000:1001",
				OutboundCommunity: "50000:1002",
			},
		}),
		defaulted(&v1beta1.External{
			Name:      "external-other-ns",
			Namespace: kmetav1.NamespaceDefault,
			Spec: v1beta1.ExternalSpec{
				IPv4Namespace:     "other",
				InboundCommunity:  "50000:2001",
				OutboundCommunity: "50000:2002",
			},
		}),
	}

	tests := []struct {
		name    string
		peering *v1beta1.ExternalPeering
		objects []kclient.Object
		err     bool
	}{
		{
			name:    "valid external peering",
			peering: extPeeringGen("ext-peer-01"),
			objects: baseObjs,
			err:     false,
		},
		{
			name: "valid external peering with multiple subnets and prefixes",
			peering: extPeeringGen("ext-peer-02", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.VPC.Subnets = []string{"subnet-b", "subnet-a"}
				peering.Spec.Permit.External.Prefixes = []v1beta1.ExternalPeeringSpecPrefix{
					{Prefix: "10.10.0.0/16"},
					{Prefix: "0.0.0.0/0"},
				}
			}),
			objects: baseObjs,
			err:     false,
		},
		{
			name: "vpc name is required",
			peering: extPeeringGen("ext-peer-03", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.VPC.Name = ""
			}),
			objects: baseObjs,
			err:     true,
		},
		{
			name: "external name is required",
			peering: extPeeringGen("ext-peer-04", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.External.Name = ""
			}),
			objects: baseObjs,
			err:     true,
		},
		{
			name: "external prefix is required",
			peering: extPeeringGen("ext-peer-05", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.External.Prefixes = []v1beta1.ExternalPeeringSpecPrefix{
					{Prefix: ""},
				}
			}),
			objects: baseObjs,
			err:     true,
		},
		{
			name: "vpc does not exist",
			peering: extPeeringGen("ext-peer-06", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.VPC.Name = "vpc-456"
			}),
			objects: baseObjs,
			err:     true,
		},
		{
			name: "external does not exist",
			peering: extPeeringGen("ext-peer-07", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.External.Name = "external-456"
			}),
			objects: baseObjs,
			err:     true,
		},
		{
			name: "vpc and external ipv4 namespace mismatch",
			peering: extPeeringGen("ext-peer-08", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.External.Name = "external-other-ns"
			}),
			objects: baseObjs,
			err:     true,
		},
		{
			name: "vpc subnet does not exist on referenced vpc",
			peering: extPeeringGen("ext-peer-09", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.VPC.Subnets = []string{"subnet-missing"}
			}),
			objects: baseObjs,
			err:     true,
		},
		{
			name: "works with empty vpc subnets list",
			peering: extPeeringGen("ext-peer-10", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.VPC.Subnets = nil
			}),
			objects: baseObjs,
			err:     false,
		},
		{
			name: "works with empty external prefixes list",
			peering: extPeeringGen("ext-peer-11", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.External.Prefixes = nil
			}),
			objects: baseObjs,
			err:     false,
		},
		{
			name:    "kube nil still validates required fields only",
			peering: extPeeringGen("ext-peer-12"),
			objects: nil,
			err:     false,
		},
		{
			name: "kube nil with missing required fields still fails",
			peering: extPeeringGen("ext-peer-13", func(peering *v1beta1.ExternalPeering) {
				peering.Spec.Permit.VPC.Name = ""
			}),
			objects: nil,
			err:     true,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var kube kclient.Reader
			if test.objects != nil {
				kube = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(test.objects...).
					Build()
			}

			_, err := test.peering.Validate(t.Context(), kube, &meta.FabricConfig{
				ExtraValidators: meta.ExtraValidators{
					Peering: func(context.Context, kclient.Reader, kclient.Object) error { return nil },
				},
			})
			if test.err {
				require.Error(t, err, "expected error but got none")
			} else {
				require.NoError(t, err, "unexpected error during validation")
			}
		})
	}
}

var errExtraValidator = errors.New("extra validator")

func TestPeeringValidationExtraValidator(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fabricObj(wiringapi.DefaultFabric, &meta.FabricConfig{})).
		Build()

	vpcPeering := vpcPeeringGen("vpc-01--vpc-02", "vpc-01", "vpc-02")
	extPeering := extPeeringGen("vpc-01--external-01")

	cfg := &meta.FabricConfig{}
	_, err := vpcPeering.Validate(t.Context(), kube, cfg)
	require.ErrorContains(t, err, "peering extra validator is not set")
	_, err = extPeering.Validate(t.Context(), kube, cfg)
	require.ErrorContains(t, err, "peering extra validator is not set")

	var validated []string
	cfg.ExtraValidators.Peering = func(_ context.Context, _ kclient.Reader, obj kclient.Object) error {
		validated = append(validated, obj.GetName())

		return errExtraValidator
	}
	_, err = vpcPeering.Validate(t.Context(), kube, cfg)
	require.ErrorIs(t, err, errExtraValidator)
	_, err = extPeering.Validate(t.Context(), kube, cfg)
	require.ErrorIs(t, err, errExtraValidator)
	require.Equal(t, []string{vpcPeering.Name, extPeering.Name}, validated)
}
