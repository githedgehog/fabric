// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func withName[T kclient.Object](name string, obj T) T {
	obj.SetName(name)
	obj.SetNamespace(kmetav1.NamespaceDefault)

	return obj
}

// defaulted models a stored object, which the mutating webhook has defaulted
func defaulted[T interface{ Default() }](obj T) T {
	obj.Default()

	return obj
}

func gwa(name string, f ...func(gw *v1alpha1.Gateway)) *v1alpha1.Gateway {
	gw := withName(name, &v1alpha1.Gateway{
		Spec: v1alpha1.GatewaySpec{
			ProtocolIP: "172.30.8.3/32",
			VTEPIP:     "172.30.12.1/32",
			ASN:        65101,
			VTEPMAC:    "ca:fe:ba:be:00:01",
			VTEPMTU:    1500,
			Interfaces: map[string]v1alpha1.GatewayInterface{
				"port0": {
					Kernel: "eth0",
					IPs:    []string{"172.30.128.3/31"},
					MTU:    1500,
				},
			},
			Neighbors: []v1alpha1.GatewayBGPNeighbor{
				{
					Source: "eth0",
					IP:     "172.30.128.1",
					ASN:    65100,
				},
			},
		},
	})

	for _, fn := range f {
		fn(gw)
	}

	return gw
}

func withObjs(base []kclient.Object, objs ...kclient.Object) []kclient.Object {
	return append(slices.Clone(base), objs...)
}

func TestGatewayValidate(t *testing.T) {
	cfg := &meta.FabricConfig{
		EnableGateway: true,
		SpineASN:      65100,
		LeafASNStart:  65101,
		LeafASNEnd:    65200,
		GatewayASN:    65101,
		GatewayCommunities: map[uint32]string{
			0: "50000:1000",
			1: "50000:1001",
		},
	}

	const planeB = "plane-b"
	oneDomain := withName(wiringapi.DefaultFabric, &wiringapi.Fabric{Spec: wiringapi.DefaultFabricSpec(cfg)})
	twoDomains := withName(wiringapi.DefaultFabric, &wiringapi.Fabric{Spec: wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{
		"default": {SpineASN: 65100, GatewayASN: 65101},
		planeB:    {SpineASN: 65098, GatewayASN: 65099},
	}}})
	groupB := defaulted(withName("gr-b", &v1alpha1.GatewayGroup{Spec: v1alpha1.GatewayGroupSpec{Topology: v1alpha1.GatewayGroupTopology{Domain: planeB}}}))

	// everything but the fabric, which is either oneDomain or twoDomains
	common := []kclient.Object{
		defaulted(&v1alpha1.GatewayGroup{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      v1alpha1.DefaultGatewayGroup,
				Namespace: "default",
			},
		}),
		defaulted(withName("gw-2", &v1alpha1.Gateway{
			Spec: v1alpha1.GatewaySpec{
				ProtocolIP: "172.30.8.2/32",
				VTEPIP:     "172.30.12.0/32",
				ASN:        65101,
				VTEPMAC:    "ca:fe:ba:be:00:02",
				VTEPMTU:    1500,
				Interfaces: map[string]v1alpha1.GatewayInterface{
					"eth0": {
						IPs: []string{"172.30.128.1/31"},
						MTU: 1500,
					},
				},
				Neighbors: []v1alpha1.GatewayBGPNeighbor{
					{
						Source: "eth0",
						IP:     "172.30.128.0",
						ASN:    65100,
					},
				},
			},
		})),
		defaulted(withName("sw-1", &wiringapi.Switch{
			Spec: wiringapi.SwitchSpec{
				ProtocolIP: "172.30.8.45/32",
				VTEPIP:     "172.30.12.45/32",
			},
		})),
	}
	base := withObjs(common, oneDomain)

	tests := []struct {
		name string
		gw   v1alpha1.Gateway
		objs []kclient.Object
		err  error
	}{
		{
			name: "test-no-overlap",
			gw:   *gwa("gw-1"),
			objs: base,
		},
		{
			name: "test-name-max-length",
			gw:   *gwa(strings.Repeat("g", v1alpha1.MaxGatewayNameLength)),
			objs: base,
		},
		{
			name: "test-name-too-long",
			gw:   *gwa(strings.Repeat("g", v1alpha1.MaxGatewayNameLength+1)),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-proto-ip-overlap",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.ProtocolIP = "172.30.8.2/32" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-vtep-ip-overlap",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPIP = "172.30.12.0/32" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-invalid-proto-ip",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.ProtocolIP = "172.30.12.0.1/32" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-non-32-proto-ip",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.ProtocolIP = "172.30.12.0/24" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-non-v4-proto-ip",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.ProtocolIP = "2001:db8::1/32" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-invalid-vtep-ip",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPIP = "172.30.12.0.1/32" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-non-32-vtep-ip",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPIP = "172.30.12.0/24" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-non-v4-vtep-ip",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPIP = "2001:db8::1/32" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-localhost-vtep-ip",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPIP = "127.0.1.2/32" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-invalid-mac",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPMAC = "00:11:22:33:44:55:66" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-all-zeros-mac",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPMAC = "00:00:00:00:00:00" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-multicast-mac",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPMAC = "01:00:5E:00:00:00" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-no-asn",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.ASN = 0 }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-asn-not-fabric-gateway-asn",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.ASN = 65102 }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-domain-not-in-fabric",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.Topology.Domain = planeB }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-domain-gateway-asn",
			gw: *gwa("gw-1", func(gw *v1alpha1.Gateway) {
				gw.Spec.Topology.Domain = planeB
				gw.Spec.ASN = 65099
				gw.Spec.Groups = []v1alpha1.GatewayGroupMembership{{Name: "gr-b"}}
			}),
			objs: withObjs(common, twoDomains, groupB),
		},
		{
			name: "test-group-in-another-domain",
			gw: *gwa("gw-1", func(gw *v1alpha1.Gateway) {
				gw.Spec.Groups = []v1alpha1.GatewayGroupMembership{{Name: "gr-b"}}
			}),
			objs: withObjs(common, twoDomains, groupB),
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-domain-group-in-default-domain",
			gw: *gwa("gw-1", func(gw *v1alpha1.Gateway) {
				gw.Spec.Topology.Domain = planeB
				gw.Spec.ASN = 65099
			}),
			objs: withObjs(common, twoDomains),
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-asn-of-another-domain",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.Topology.Domain = planeB }),
			objs: withObjs(common, twoDomains),
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-no-interfaces",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.Interfaces = map[string]v1alpha1.GatewayInterface{} }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-interface-invalid-ip",
			gw: *gwa("gw-1", func(gw *v1alpha1.Gateway) {
				gw.Spec.Interfaces["eth0"] = v1alpha1.GatewayInterface{IPs: []string{"172.30.128.256/31"}, MTU: 1500}
			}),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-no-neighbors",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.Neighbors = []v1alpha1.GatewayBGPNeighbor{} }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-neighbor-invalid-ip",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.Neighbors[0].IP = "172.30.128.256" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-neighbor-no-asn",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.Neighbors[0].ASN = 0 }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-too-many-gws-in-group",
			gw: *gwa("gw-1", func(gw *v1alpha1.Gateway) {
				gw.Spec.Groups = []v1alpha1.GatewayGroupMembership{{Name: "gr1", Priority: 0}}
			}),
			objs: withObjs(base,
				defaulted(withName("gr1", &v1alpha1.GatewayGroup{})),
				defaulted(withName("gw-3", &v1alpha1.Gateway{
					Spec: v1alpha1.GatewaySpec{
						Groups: []v1alpha1.GatewayGroupMembership{{Name: "gr1", Priority: 1}},
					},
				})),
				defaulted(withName("gw-4", &v1alpha1.Gateway{
					Spec: v1alpha1.GatewaySpec{
						Groups: []v1alpha1.GatewayGroupMembership{{Name: "gr1", Priority: 2}},
					},
				})),
			),
			err: v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-fits-in-gw-group",
			gw: *gwa("gw-1", func(gw *v1alpha1.Gateway) {
				gw.Spec.Groups = []v1alpha1.GatewayGroupMembership{{Name: "gr1", Priority: 0}}
			}),
			objs: withObjs(base,
				defaulted(withName("gr1", &v1alpha1.GatewayGroup{})),
				defaulted(withName("gw-3", &v1alpha1.Gateway{
					Spec: v1alpha1.GatewaySpec{
						Groups: []v1alpha1.GatewayGroupMembership{{Name: "gr1", Priority: 1}},
					},
				})),
			),
		},
		{
			name: "test-proto-ip-overlap-with-switch",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.ProtocolIP = "172.30.8.45/32" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-vtep-ip-overlap-with-switch",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPIP = "172.30.12.45/32" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-proto-ip-no-overlap-with-switch-vtep",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.ProtocolIP = "172.30.12.45/32" }),
			objs: base,
		},
		{
			name: "test-vtep-mac-overlap",
			gw:   *gwa("gw-1", func(gw *v1alpha1.Gateway) { gw.Spec.VTEPMAC = "ca:fe:ba:be:00:02" }),
			objs: base,
			err:  v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-log-rate-limit-explicit",
			gw: *gwa("gw-1", func(gw *v1alpha1.Gateway) {
				gw.Spec.Logs.RateLimit = &v1alpha1.GatewayLogRateLimit{Burst: 100, ReplenishPerSecond: 10}
			}),
			objs: base,
		},
		{
			name: "test-log-rate-limit-empty-defaults",
			gw: *gwa("gw-1", func(gw *v1alpha1.Gateway) {
				gw.Spec.Logs.RateLimit = &v1alpha1.GatewayLogRateLimit{}
			}),
			objs: base,
		},
		{
			name: "test-log-rate-limit-absent-disabled",
			gw:   *gwa("gw-1"),
			objs: base,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme), "should add gateway API to scheme")
	require.NoError(t, wiringapi.AddToScheme(scheme), "should add wiring API to scheme")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()

			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objs...).
				Build()

			tt.gw.Default()
			actual := tt.gw.Validate(ctx, kube, cfg)
			assert.ErrorIs(t, actual, tt.err, "validate should return expected error")
		})
	}
}

func TestGatewayLogRateLimitDefaulting(t *testing.T) {
	t.Run("absent stays disabled", func(t *testing.T) {
		gw := gwa("gw-1")
		gw.Default()
		assert.Nil(t, gw.Spec.Logs.RateLimit, "absent rateLimit must remain nil (disabled)")
	})

	t.Run("empty defaults to 50:5", func(t *testing.T) {
		gw := gwa("gw-1", func(gw *v1alpha1.Gateway) {
			gw.Spec.Logs.RateLimit = &v1alpha1.GatewayLogRateLimit{}
		})
		gw.Default()
		require.NotNil(t, gw.Spec.Logs.RateLimit)
		assert.Equal(t, v1alpha1.DefaultGatewayLogRateLimitBurst, gw.Spec.Logs.RateLimit.Burst)
		assert.Equal(t, v1alpha1.DefaultGatewayLogRateLimitReplenishPerSecond, gw.Spec.Logs.RateLimit.ReplenishPerSecond)
	})

	t.Run("explicit values are kept", func(t *testing.T) {
		gw := gwa("gw-1", func(gw *v1alpha1.Gateway) {
			gw.Spec.Logs.RateLimit = &v1alpha1.GatewayLogRateLimit{Burst: 100, ReplenishPerSecond: 10}
		})
		gw.Default()
		require.NotNil(t, gw.Spec.Logs.RateLimit)
		assert.Equal(t, uint32(100), gw.Spec.Logs.RateLimit.Burst)
		assert.Equal(t, uint32(10), gw.Spec.Logs.RateLimit.ReplenishPerSecond)
	})
}

func TestGatewayGroupDomain(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		withName("default", &wiringapi.Fabric{Spec: wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{
			"default": {SpineASN: 65100, GatewayASN: 65101},
			"plane-b": {SpineASN: 65098, GatewayASN: 65099},
		}}}),
	).Build()

	for _, tt := range []struct {
		group  string
		domain string
		err    string
	}{
		{group: "gr-1", domain: ""},
		{group: "gr-1", domain: "plane-b"},
		{group: "gr-1", domain: "plane-c", err: "domain plane-c not found in fabric default"},
		// the default group is checked like any other
		{group: v1alpha1.DefaultGatewayGroup, domain: "plane-c", err: "domain plane-c not found in fabric default"},
	} {
		t.Run(tt.group+" in domain "+tt.domain, func(t *testing.T) {
			group := withName(tt.group, &v1alpha1.GatewayGroup{Spec: v1alpha1.GatewayGroupSpec{Topology: v1alpha1.GatewayGroupTopology{Domain: tt.domain}}})
			group.Default()
			// an unset domain is defaulted to the default one
			expectedDomain := tt.domain
			if expectedDomain == "" {
				expectedDomain = wiringapi.DefaultFabricDomain
			}
			require.Equal(t, expectedDomain, group.Spec.Topology.Domain)
			require.Contains(t, group.Labels, wiringapi.ListLabelDomain(expectedDomain))

			err := group.Validate(t.Context(), kube, &meta.FabricConfig{EnableGateway: true})
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}

// without a client nothing that needs other objects is checked, the fabric neither
func TestGatewayValidateWithoutClient(t *testing.T) {
	cfg := &meta.FabricConfig{EnableGateway: true}

	gw := gwa("gw-1")
	gw.Default()
	require.NoError(t, gw.Validate(t.Context(), nil, cfg))

	group := withName("gr-1", &v1alpha1.GatewayGroup{})
	group.Default()
	require.NoError(t, group.Validate(t.Context(), nil, cfg))
}
