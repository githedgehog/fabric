// Copyright 2025 Hedgehog
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
			// the BGP attachments added while migrating from static need it from the start
			name: "l2 with localASN",
			external: extGen("valid-st", func(ext *v1beta1.External) {
				ext.Spec.LocalASN = 64999
				ext.Spec.Static = &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				}
			}),
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

func TestExternalLocalASNValidation(t *testing.T) {
	ipns := ipv4NamespaceObj()
	backendFabric := &wiringapi.Fabric{
		ObjectMeta: kmetav1.ObjectMeta{Name: "backend", Namespace: kmetav1.NamespaceDefault},
		Spec: wiringapi.FabricSpec{
			LeafASNStart: 64101,
			LeafASNEnd:   64199,
			Domains:      map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {SpineASN: 64100, GatewayASN: 64200}},
		},
	}
	backendExt := defaulted(&v1beta1.External{
		ObjectMeta: kmetav1.ObjectMeta{Name: "backend-ext", Namespace: kmetav1.NamespaceDefault},
		Spec:       v1beta1.ExternalSpec{IPv4Namespace: "backend", Topology: v1beta1.ExternalTopology{Fabric: "backend"}, LocalASN: 64999},
	})
	attach := l3ExtAttGen("ext-att-01", func(att *v1beta1.ExternalAttachment) {
		att.Spec.External = "external-01"
		att.Spec.Neighbor.ASN = 64000
	})
	// Fabric/default, added to the objects of every test, is created from this
	cfg := &meta.FabricConfig{SpineASN: 65100, LeafASNStart: 65101, LeafASNEnd: 65200, GatewayASN: 65534}
	defaultFabric := fabricObj(wiringapi.DefaultFabric, cfg)
	withLocalASN := func(asn uint32) *v1beta1.External {
		return extGen("external-01", func(ext *v1beta1.External) { ext.Spec.LocalASN = asn })
	}

	tests := []struct {
		name     string
		external *v1beta1.External
		objects  []kclient.Object
		err      bool
		warns    bool
	}{
		{
			name:     "valid localASN",
			external: withLocalASN(64999),
			objects:  []kclient.Object{ipns, backendFabric, attach},
		},
		{
			name:     "localASN in own fabric leaf range",
			external: withLocalASN(65150),
			objects:  []kclient.Object{ipns},
			err:      true,
		},
		{
			name:     "localASN is own fabric spine ASN",
			external: withLocalASN(65100),
			objects:  []kclient.Object{ipns},
			err:      true,
		},
		{
			name:     "localASN is own fabric gateway ASN",
			external: withLocalASN(65534),
			objects:  []kclient.Object{ipns},
			err:      true,
		},
		{
			name:     "localASN is an attachment neighbor ASN",
			external: withLocalASN(64000),
			objects:  []kclient.Object{ipns, attach},
			err:      true,
		},
		{
			name:     "localASN in another fabric leaf range",
			external: withLocalASN(64150),
			objects:  []kclient.Object{ipns, backendFabric},
			warns:    true,
		},
		{
			name:     "localASN shared with an external of another fabric",
			external: withLocalASN(64999),
			objects:  []kclient.Object{ipns, backendFabric, backendExt},
			warns:    true,
		},
		{
			name:     "localASN shared with an external of the same fabric",
			external: withLocalASN(64999),
			objects: []kclient.Object{ipns, extGen("other", func(ext *v1beta1.External) {
				ext.Spec.LocalASN = 64999
			})},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(withObjs(test.objects, defaultFabric)...).
				Build()
			warns, err := test.external.Validate(t.Context(), kube, cfg)
			if test.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if test.warns {
				require.NotEmpty(t, warns)
			} else {
				require.Empty(t, warns)
			}
		})
	}
}

// defaulting sets it, so only an External the refresh on fabric-ctrl initialization had to leave alone or one
// validated without defaulting can miss it
func TestExternalDomainRequired(t *testing.T) {
	ext := &v1beta1.External{ObjectMeta: kmetav1.ObjectMeta{Name: "ext-1", Namespace: kmetav1.NamespaceDefault}}
	ext.Default()
	ext.Spec.Topology.Domain = ""

	_, err := ext.Validate(t.Context(), nil, nil)
	require.ErrorContains(t, err, "topology.domain is required")
}
