// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"slices"
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

func l3ExtAttGen(name string, f ...func(att *v1beta1.ExternalAttachment)) *v1beta1.ExternalAttachment {
	base := &v1beta1.ExternalAttachment{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      name,
			Namespace: kmetav1.NamespaceDefault,
		},
		Spec: v1beta1.ExternalAttachmentSpec{
			External:   "external-01",
			Connection: "leaf-01--external",
			Neighbor: v1beta1.ExternalAttachmentNeighbor{
				ASN: 64000,
				IP:  "10.90.0.4",
			},
			Switch: v1beta1.ExternalAttachmentSwitch{
				VLAN: 100,
				IP:   "10.90.0.5/31",
			},
		},
	}

	for _, fn := range f {
		fn(base)
	}
	base.Default()

	return base
}

func staticExtAttGen(name string, f ...func(att *v1beta1.ExternalAttachment)) *v1beta1.ExternalAttachment {
	base := &v1beta1.ExternalAttachment{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      name,
			Namespace: kmetav1.NamespaceDefault,
		},
		Spec: v1beta1.ExternalAttachmentSpec{
			External:   "external-02",
			Connection: "leaf-01--external",
			Static: &v1beta1.ExternalAttachmentStatic{
				RemoteIP: "10.45.0.2",
				VLAN:     200,
				Proxy:    true,
			},
		},
	}
	for _, fn := range f {
		fn(base)
	}
	base.Default()

	return base
}

func withObjs(base []kclient.Object, objs ...kclient.Object) []kclient.Object {
	return append(slices.Clone(base), objs...)
}

func TestExternalAttachmentValidation(t *testing.T) {
	baseObjs := []kclient.Object{
		&v1beta1.External{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      "external-01",
				Namespace: kmetav1.NamespaceDefault,
			},
			Spec: v1beta1.ExternalSpec{
				IPv4Namespace:     "default",
				InboundCommunity:  "50000:1001",
				OutboundCommunity: "50000:1002",
			},
		},
		&v1beta1.External{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      "external-02",
				Namespace: kmetav1.NamespaceDefault,
			},
			Spec: v1beta1.ExternalSpec{
				IPv4Namespace: "default",
				Static: &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				},
			},
		},
		&wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      "leaf-01--external",
				Namespace: kmetav1.NamespaceDefault,
			},
			Spec: wiringapi.ConnectionSpec{
				External: &wiringapi.ConnExternal{
					Link: wiringapi.ConnExternalLink{
						Switch: wiringapi.BasePortName{
							Port: "leaf-01/E1/1",
						},
					},
				},
			},
		},
	}
	tests := []struct {
		name    string
		extAtt  *v1beta1.ExternalAttachment
		objects []kclient.Object
		err     bool
	}{
		{
			name:    "valid BGP external attachment",
			extAtt:  l3ExtAttGen("ext-att-01"),
			objects: baseObjs,
			err:     false,
		},
		{
			name:    "valid static external attachment",
			extAtt:  staticExtAttGen("ext-att-02"),
			objects: baseObjs,
			err:     false,
		},
		{
			name:    "external does not exist",
			extAtt:  l3ExtAttGen("ext-att-03", func(att *v1beta1.ExternalAttachment) { att.Spec.External = "external-456" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:    "connection does not exist",
			extAtt:  l3ExtAttGen("ext-att-04", func(att *v1beta1.ExternalAttachment) { att.Spec.Connection = "conn-456" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:    "l3 attach with static external",
			extAtt:  l3ExtAttGen("ext-att-05", func(att *v1beta1.ExternalAttachment) { att.Spec.External = "external-02" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:    "static attach with l3 external",
			extAtt:  staticExtAttGen("ext-att-06", func(att *v1beta1.ExternalAttachment) { att.Spec.External = "external-01" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:   "multiple attaches same vlan",
			extAtt: l3ExtAttGen("ext-att-07"),
			objects: withObjs(baseObjs,
				staticExtAttGen("vlan-clash", func(att *v1beta1.ExternalAttachment) { att.Spec.Static.VLAN = 100 })),
			err: true,
		},
		{
			name:   "multiple attaches different vlans",
			extAtt: l3ExtAttGen("ext-att-08"),
			objects: withObjs(baseObjs,
				staticExtAttGen("no-clash")),
			err: false,
		},
		{
			name:    "static attach with both proxy and IP specified",
			extAtt:  staticExtAttGen("ext-att-09", func(att *v1beta1.ExternalAttachment) { att.Spec.Static.IP = "10.45.0.1/24" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:    "static attach with neither proxy nor IP specified",
			extAtt:  staticExtAttGen("ext-att-09", func(att *v1beta1.ExternalAttachment) { att.Spec.Static.Proxy = false }),
			objects: baseObjs,
			err:     true,
		},
		{
			name: "valid static attach without proxy",
			extAtt: staticExtAttGen("ext-att-09", func(att *v1beta1.ExternalAttachment) {
				att.Spec.Static.IP = "10.45.0.1/24"
				att.Spec.Static.Proxy = false
			}),
			objects: baseObjs,
		},
		// TODO: add tests to validate individual fields
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(test.objects...).
				Build()
			_, err := test.extAtt.Validate(t.Context(), kube, &meta.FabricConfig{})
			if test.err {
				require.Error(t, err, "expected error but got none")
			} else {
				require.NoError(t, err, "unexpected error during validation")
			}
		})
	}
}
