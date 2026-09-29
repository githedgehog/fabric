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

func TestConnectionSwitches(t *testing.T) {
	sw := &wiringapi.Switch{ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01", Namespace: kmetav1.NamespaceDefault}}
	unbundled := func(name, leaf string) *wiringapi.Connection {
		return &wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{Unbundled: &wiringapi.ConnUnbundled{
				Link: wiringapi.ServerToSwitchLink{
					Server: wiringapi.NewBasePortName("server-01/enp2s1"),
					Switch: wiringapi.NewBasePortName(leaf + "/E1/1"),
				},
			}},
		}
	}

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		sw,
		unbundled("on-leaf-01", "leaf-01"),
		unbundled("on-missing", "leaf-missing"),
	).Build()

	// connections and switches that don't exist are skipped, and each switch is listed once
	switches, err := v1beta1.ConnectionSwitches(t.Context(), kube, kmetav1.NamespaceDefault, []string{"on-leaf-01", "on-leaf-01", "on-missing", "nope"})
	require.NoError(t, err)
	require.Len(t, switches, 1)
	require.Contains(t, switches, "leaf-01")
}

func TestDomainPin(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	kubeWith := func(domains ...string) kclient.Reader {
		fabric := &wiringapi.Fabric{
			ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
			Spec:       wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{}},
		}
		for _, domain := range domains {
			fabric.Spec.Domains[domain] = wiringapi.FabricDomainSpec{}
		}

		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&v1beta1.IPv4Namespace{
					ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
					Spec:       v1beta1.IPv4NamespaceSpec{Subnets: []string{"10.0.0.0/16"}},
				},
				&wiringapi.VLANNamespace{
					ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
					Spec:       wiringapi.VLANNamespaceSpec{Ranges: []meta.VLANRange{{From: 100, To: 200}}},
				},
				fabric,
			).
			Build()
	}
	withDefault := kubeWith(wiringapi.DefaultFabricDomain, "plane-b")
	// a fabric with only named domains has no default for an object to fall into
	namedOnly := kubeWith("plane-a", "plane-b")

	for _, tt := range []struct {
		name   string
		kube   kclient.Reader
		domain string
		err    string
	}{
		{name: "defaulted", kube: withDefault},
		{name: "pinned", kube: withDefault, domain: "plane-b"},
		{name: "pinned to a domain not in the fabric", kube: withDefault, domain: "plane-c", err: "domain plane-c not found in fabric default"},
		{name: "defaulted, fabric has no default domain", kube: namedOnly, err: "domain default not found in fabric default, topology.domain must name one of its domains"},
		{name: "pinned, fabric has no default domain", kube: namedOnly, domain: "plane-a"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			expected := tt.domain
			if expected == "" {
				expected = wiringapi.DefaultFabricDomain
			}

			vpc := vpcGen("vpc-pin", func(vpc *v1beta1.VPC) { vpc.Spec.Topology.Domain = tt.domain })
			require.Equal(t, expected, vpc.Spec.Topology.Domain)
			require.Contains(t, vpc.Labels, wiringapi.ListLabelDomain(expected))
			_, vpcErr := vpc.Validate(t.Context(), tt.kube, &meta.FabricConfig{})

			ext := extGen("ext-pin", func(ext *v1beta1.External) { ext.Spec.Topology.Domain = tt.domain })
			require.Equal(t, expected, ext.Spec.Topology.Domain)
			require.Contains(t, ext.Labels, wiringapi.ListLabelDomain(expected))
			_, extErr := ext.Validate(t.Context(), tt.kube, &meta.FabricConfig{})

			for _, err := range []error{vpcErr, extErr} {
				if tt.err == "" {
					require.NoError(t, err)

					continue
				}
				require.ErrorContains(t, err, tt.err)
			}
		})
	}
}
