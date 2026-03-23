// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"slices"
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

func gwa(name string, f ...func(gw *v1alpha1.Gateway)) *v1alpha1.Gateway { //nolint:unparam
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
	base := []kclient.Object{
		&v1alpha1.GatewayGroup{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      v1alpha1.DefaultGatewayGroup,
				Namespace: "default",
			},
		},
		withName("gw-2", &v1alpha1.Gateway{
			Spec: v1alpha1.GatewaySpec{
				ProtocolIP: "172.30.8.2/32",
				VTEPIP:     "172.30.12.0/32",
				ASN:        65101,
				VTEPMAC:    "ca:fe:ba:be:00:01",
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
		}),
		withName("sw-1", &wiringapi.Switch{
			Spec: wiringapi.SwitchSpec{
				ProtocolIP: "172.30.8.45/32",
				VTEPIP:     "172.30.12.45/32",
			},
		}),
	}

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
				withName("gr1", &v1alpha1.GatewayGroup{}),
				withName("gw-3", &v1alpha1.Gateway{
					Spec: v1alpha1.GatewaySpec{
						Groups: []v1alpha1.GatewayGroupMembership{{Name: "gr1", Priority: 1}},
					},
				}),
				withName("gw-4", &v1alpha1.Gateway{
					Spec: v1alpha1.GatewaySpec{
						Groups: []v1alpha1.GatewayGroupMembership{{Name: "gr1", Priority: 2}},
					},
				}),
			),
			err: v1alpha1.ErrInvalidGW,
		},
		{
			name: "test-fits-in-gw-group",
			gw: *gwa("gw-1", func(gw *v1alpha1.Gateway) {
				gw.Spec.Groups = []v1alpha1.GatewayGroupMembership{{Name: "gr1", Priority: 0}}
			}),
			objs: withObjs(base,
				withName("gr1", &v1alpha1.GatewayGroup{}),
				withName("gw-3", &v1alpha1.Gateway{
					Spec: v1alpha1.GatewaySpec{
						Groups: []v1alpha1.GatewayGroupMembership{{Name: "gr1", Priority: 1}},
					},
				}),
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
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme), "should add gateway API to scheme")
	require.NoError(t, wiringapi.AddToScheme(scheme), "should add wiring API to scheme")
	cfg := &meta.FabricConfig{
		EnableGateway: true,
		GatewayCommunities: map[uint32]string{
			0: "50000:1000",
			1: "50000:1001",
		},
	}

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
