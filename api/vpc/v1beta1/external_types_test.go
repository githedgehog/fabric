// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

const (
	InboundCommunity  = "50000:1001"
	OutboundCommunity = "50000:1002"
)

func extGen(name string, f ...func(att *v1beta1.External)) *v1beta1.External {
	base := &v1beta1.External{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      name,
			Namespace: kmetav1.NamespaceDefault,
		},
		Spec: v1beta1.ExternalSpec{
			IPv4Namespace: "default",
		},
	}

	for _, fn := range f {
		fn(base)
	}
	base.Default()

	return base
}

func TestExternalValidation(t *testing.T) {
	tests := []struct {
		name     string
		external *v1beta1.External
		err      bool
	}{
		{
			name: "valid bgp external",
			external: extGen("valid-bgp", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
				ext.Spec.OutboundCommunity = OutboundCommunity
			}),
		},
		{
			name: "name too long",
			external: extGen("this-is-a-name-longer-than-possible", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
				ext.Spec.OutboundCommunity = OutboundCommunity
			}),
			err: true,
		},
		{
			name: "bgp missing inbound",
			external: extGen("invalid-bgp", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
			}),
			err: true,
		},
		{
			name: "bgp missing outbound",
			external: extGen("invalid-bgp", func(ext *v1beta1.External) {
				ext.Spec.OutboundCommunity = OutboundCommunity
			}),
			err: true,
		},
		{
			name: "bgp invalid inbound",
			external: extGen("invalid-bgp", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = "InboundCommunity"
			}),
			err: true,
		},
		{
			name: "bgp invalid outbound",
			external: extGen("invalid-bgp", func(ext *v1beta1.External) {
				ext.Spec.OutboundCommunity = "OutboundCommunity"
			}),
			err: true,
		},
		{
			name: "valid Static",
			external: extGen("valid-st", func(ext *v1beta1.External) {
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
		},
		{
			name: "l2 with inbound community",
			external: extGen("invalid-st", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
			err: true,
		},
		{
			name: "l2 with outbound community",
			external: extGen("invalid-st", func(ext *v1beta1.External) {
				ext.Spec.OutboundCommunity = OutboundCommunity
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
			err: true,
		},
		{
			name: "l2 without prefixes",
			external: extGen("invalid-st", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{},
				}
			}),
			err: true,
		},
		{
			name: "l2 with overlapping prefixes",
			external: extGen("invalid-st", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0", "10.10.0.0/24"},
				}
			}),
			err: true,
		},
		{
			name: "l2 with invalid prefix",
			external: extGen("invalid-st", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.4350/0"},
				}
			}),
			err: true,
		},
	}
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			_, err := test.external.Validate(ctx, nil, nil)
			if test.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
