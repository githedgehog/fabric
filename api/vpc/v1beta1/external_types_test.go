// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"net/netip"
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

func TestExternalInboundPrefixes(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		fabricObj(wiringapi.DefaultFabric, &meta.FabricConfig{}), ipv4NamespaceObj(),
	).Build()

	for _, tt := range []struct {
		name     string
		prefixes map[string]v1beta1.ExternalInboundPrefix
		static   bool
		err      string
	}{
		{name: "exact", prefixes: map[string]v1beta1.ExternalInboundPrefix{"0.0.0.0/0": {}, "10.1.1.0/24": {}}},
		{name: "ranges", prefixes: map[string]v1beta1.ExternalInboundPrefix{
			"10.1.0.0/16": {MaxPrefixLen: 24}, "172.16.0.0/12": {MinPrefixLen: 16, MaxPrefixLen: 32}, "192.168.0.0/16": {MinPrefixLen: 24},
		}},
		{name: "around the namespace", prefixes: map[string]v1beta1.ExternalInboundPrefix{"10.0.0.0/8": {MaxPrefixLen: 32}}},
		{name: "inside the namespace", prefixes: map[string]v1beta1.ExternalInboundPrefix{"10.0.1.0/24": {}}, err: "inbound prefix 10.0.1.0/24 is inside subnet 10.0.0.0/16"},
		{name: "invalid", prefixes: map[string]v1beta1.ExternalInboundPrefix{"10.1.1.0/33": {}}, err: "invalid inbound prefix"},
		{name: "IPv6", prefixes: map[string]v1beta1.ExternalInboundPrefix{"fd00::/64": {}}, err: "is not IPv4"},
		{name: "host bits", prefixes: map[string]v1beta1.ExternalInboundPrefix{"10.1.1.1/24": {}}, err: "has host bits set"},
		{name: "min shorter than the prefix", prefixes: map[string]v1beta1.ExternalInboundPrefix{"10.1.0.0/16": {MinPrefixLen: 8}}, err: "minPrefixLen 8 is not between 16 and 32"},
		{name: "min above 32", prefixes: map[string]v1beta1.ExternalInboundPrefix{"10.1.0.0/16": {MinPrefixLen: 33}}, err: "minPrefixLen 33 is not between 16 and 32"},
		{name: "max below min", prefixes: map[string]v1beta1.ExternalInboundPrefix{"10.1.0.0/16": {MinPrefixLen: 24, MaxPrefixLen: 20}}, err: "maxPrefixLen 20 is not between 24 and 32"},
		{name: "max below the prefix", prefixes: map[string]v1beta1.ExternalInboundPrefix{"10.1.0.0/16": {MaxPrefixLen: 8}}, err: "maxPrefixLen 8 is not between 16 and 32"},
		{name: "max above 32", prefixes: map[string]v1beta1.ExternalInboundPrefix{"10.1.0.0/16": {MaxPrefixLen: 33}}, err: "maxPrefixLen 33 is not between 16 and 32"},
		{name: "static", static: true, prefixes: map[string]v1beta1.ExternalInboundPrefix{"0.0.0.0/0": {}}, err: "inboundPrefixes must be empty"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ext := extGen("ext-01", func(ext *v1beta1.External) {
				ext.Spec.InboundPrefixes = tt.prefixes
				if tt.static {
					ext.Spec.Static = &v1beta1.ExternalStaticSpec{Prefixes: []string{"0.0.0.0/0"}}
				}
			})
			_, err := ext.Validate(t.Context(), kube, &meta.FabricConfig{})
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestExternalInboundPrefixLens(t *testing.T) {
	for _, tt := range []struct {
		prefix   string
		lens     v1beta1.ExternalInboundPrefix
		min, max uint8
	}{
		{prefix: "10.1.0.0/16", min: 16, max: 16},
		{prefix: "10.1.0.0/16", lens: v1beta1.ExternalInboundPrefix{MaxPrefixLen: 24}, min: 16, max: 24},
		{prefix: "10.1.0.0/16", lens: v1beta1.ExternalInboundPrefix{MinPrefixLen: 24}, min: 24, max: 24},
		{prefix: "0.0.0.0/0", lens: v1beta1.ExternalInboundPrefix{MaxPrefixLen: 32}, min: 0, max: 32},
	} {
		minLen, maxLen := tt.lens.PrefixLens(netip.MustParsePrefix(tt.prefix))
		require.Equal(t, [2]uint8{tt.min, tt.max}, [2]uint8{minLen, maxLen}, "%s %+v", tt.prefix, tt.lens)
	}
}

// a VPC interconnect generates the External with its name
func TestExternalVPCInterconnectName(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		fabricObj(wiringapi.DefaultFabric, &meta.FabricConfig{}), ipv4NamespaceObj(), icGen("ic-01"),
	).Build()

	_, err := extGen("ic-01").Validate(t.Context(), kube, &meta.FabricConfig{})
	require.ErrorContains(t, err, "VPC interconnect ic-01 already exists")

	_, err = extGen("ic-01", func(ext *v1beta1.External) { ext.OwnerReferences = ownedBy("ic-01") }).Validate(t.Context(), kube, &meta.FabricConfig{})
	require.NoError(t, err)

	_, err = extGen("ext-01").Validate(t.Context(), kube, &meta.FabricConfig{})
	require.NoError(t, err)
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
	attachNoASN := attach.DeepCopy()
	attachNoASN.Name = "ext-att-02"
	attachNoASN.Spec.Neighbor.ASN = 0
	staticAttach := staticExtAttGen("ext-att-03", func(att *v1beta1.ExternalAttachment) { att.Spec.External = attach.Spec.External })
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
			name:     "localASN with an attachment without a neighbor ASN",
			external: withLocalASN(64999),
			objects:  []kclient.Object{ipns, attachNoASN},
			err:      true,
		},
		{
			name:     "localASN with a static attachment",
			external: withLocalASN(64999),
			objects:  []kclient.Object{ipns, staticAttach},
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
