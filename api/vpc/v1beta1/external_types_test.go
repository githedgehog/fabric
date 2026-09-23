// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	asnCfg := &meta.FabricConfig{SpineASN: 65100, LeafASNStart: 65101, LeafASNEnd: 65131}
	tests := []struct {
		name     string
		external *v1beta1.External
		cfg      *meta.FabricConfig
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
			name: "bgp missing outbound",
			external: extGen("no-out", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
			}),
			err: false,
		},
		{
			name: "bgp missing inbound",
			external: extGen("no-in", func(ext *v1beta1.External) {
				ext.Spec.OutboundCommunity = OutboundCommunity
			}),
			err: false,
		},
		{
			name:     "bgp missing both communities",
			external: extGen("no-comms", func(ext *v1beta1.External) {}),
			err:      false,
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
			// static routes carry the fabric-owned identity community too now, so a mixed
			// external can filter what its BGP attachments accept
			name: "static with inbound community",
			external: extGen("st-in-comm", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
		},
		{
			name: "static with outbound community",
			external: extGen("st-out-comm", func(ext *v1beta1.External) {
				ext.Spec.OutboundCommunity = OutboundCommunity
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
		},
		{
			name: "static with invalid inbound community",
			external: extGen("st-bad-comm", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = "not-a-community"
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
			err: true,
		},
		{
			name: "bgp with priority",
			external: extGen("bgp-prio", func(ext *v1beta1.External) {
				ext.Spec.InboundCommunity = InboundCommunity
				ext.Spec.Priority = 3
			}),
		},
		{
			name: "priority above the maximum",
			external: extGen("bgp-prio-max", func(ext *v1beta1.External) {
				ext.Spec.Priority = 4
			}),
			err: true,
		},
		{
			name: "static with priority",
			external: extGen("st-prio", func(ext *v1beta1.External) {
				ext.Spec.Priority = 1
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
			err: true,
		},
		{
			name: "l2 without prefixes",
			external: extGen("invalid-st", func(ext *v1beta1.External) {
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{},
				}
			}),
			err: true,
		},
		{
			name: "l2 with overlapping prefixes",
			external: extGen("invalid-st", func(ext *v1beta1.External) {
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0", "10.10.0.0/24"},
				}
			}),
			err: true,
		},
		{
			name: "l2 with invalid prefix",
			external: extGen("invalid-st", func(ext *v1beta1.External) {
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.4350/0"},
				}
			}),
			err: true,
		},
		{
			name: "bgp with advertise",
			external: extGen("bgp-adv", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{
					Prepend:     ptr(uint8(2)),
					Prefixes:    []string{"100.100.0.0/24"},
					Communities: []string{"65102:100"},
				}
			}),
		},
		{
			name: "advertise with an invalid prefix",
			external: extGen("bgp-adv-pfx", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{
					Prefixes: []string{"100.100.0.0"},
				}
			}),
			err: true,
		},
		{
			name: "advertise with an invalid community",
			external: extGen("bgp-adv-comm", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{
					Communities: []string{"not-a-community"},
				}
			}),
			err: true,
		},
		{
			name: "advertise community in a fabric-owned namespace",
			external: extGen("bgp-adv-own", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{
					Communities: []string{fmt.Sprintf("%d:1", meta.ExtRankCommBase)},
				}
			}),
			err: true,
		},
		{
			name: "advertise community in the gateway namespace",
			external: extGen("bgp-adv-gw", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{
					Communities: []string{"50001:7"},
				}
			}),
			cfg: &meta.FabricConfig{GatewayCommunities: map[uint32]string{0: "50001:0"}},
			err: true,
		},
		{
			name: "advertise community in the VPC namespace",
			external: extGen("bgp-adv-vpc", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{
					Communities: []string{"50000:7"},
				}
			}),
			cfg: &meta.FabricConfig{BaseVPCCommunity: "50000:0"},
			err: true,
		},
		{
			name: "advertise with an IPv6 prefix",
			external: extGen("bgp-adv-v6", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{
					Prefixes: []string{"2001:db8::/32"},
				}
			}),
			err: true,
		},
		{
			name: "static with advertise",
			external: extGen("st-adv", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{Prepend: ptr(uint8(1))}
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
			err: true,
		},
		{
			name: "valid localASN",
			external: extGen("las-ok", func(ext *v1beta1.External) {
				ext.Spec.LocalASN = 65199
			}),
			cfg: asnCfg,
		},
		{
			name: "localASN inside the leaf range",
			external: extGen("las-leaf", func(ext *v1beta1.External) {
				ext.Spec.LocalASN = 65102
			}),
			cfg: asnCfg,
			err: true,
		},
		{
			name: "localASN equal to the spine ASN",
			external: extGen("las-spine", func(ext *v1beta1.External) {
				ext.Spec.LocalASN = 65100
			}),
			cfg: asnCfg,
			err: true,
		},
		{
			name: "static with localASN",
			external: extGen("st-las", func(ext *v1beta1.External) {
				ext.Spec.LocalASN = 65199
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
			cfg: asnCfg,
			err: true,
		},
	}
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			_, err := test.external.Validate(ctx, nil, test.cfg)
			if test.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// the overlap and neighbor-ASN checks are the ones that need to read other objects
func TestExternalValidationWithKube(t *testing.T) {
	ipns := &v1beta1.IPv4Namespace{
		ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
		Spec:       v1beta1.IPv4NamespaceSpec{Subnets: []string{"10.0.0.0/16"}},
	}
	attach := &v1beta1.ExternalAttachment{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      "leaf-01--ext",
			Namespace: kmetav1.NamespaceDefault,
			Labels:    map[string]string{v1beta1.LabelExternal: "ext-01"},
		},
		Spec: v1beta1.ExternalAttachmentSpec{
			External:   "ext-01",
			Connection: "leaf-01--external",
			Neighbor:   v1beta1.ExternalAttachmentNeighbor{ASN: 64102, IP: "100.1.10.6"},
			Switch:     v1beta1.ExternalAttachmentSwitch{IP: "100.1.10.1/24"},
		},
	}

	tests := []struct {
		name     string
		external *v1beta1.External
		err      bool
	}{
		{
			name: "advertise prefix outside the IPv4Namespace",
			external: extGen("ext-01", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{Prefixes: []string{"100.100.0.0/24"}}
			}),
		},
		{
			// already advertised by the IPNS subnets statement
			name: "advertise prefix inside the IPv4Namespace",
			external: extGen("ext-01", func(ext *v1beta1.External) {
				ext.Spec.Advertise = &v1beta1.ExternalAdvertiseSpec{Prefixes: []string{"10.0.5.0/24"}}
			}),
			err: true,
		},
		{
			name: "localASN not used by any attachment",
			external: extGen("ext-01", func(ext *v1beta1.External) {
				ext.Spec.LocalASN = 65199
			}),
		},
		{
			name: "localASN equal to an attachment neighbor ASN",
			external: extGen("ext-01", func(ext *v1beta1.External) {
				ext.Spec.LocalASN = 64102
			}),
			err: true,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ipns, attach).Build()
			_, err := test.external.Validate(t.Context(), kube, nil)
			if test.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
