// Copyright 2026 Hedgehog
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

func TestVPCAndExternalDomain(t *testing.T) {
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
				ipv4NamespaceObj(),
				defaulted(&wiringapi.VLANNamespace{
					ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
					Spec:       wiringapi.VLANNamespaceSpec{Ranges: []meta.VLANRange{{From: 100, To: 200}}},
				}),
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
		{name: "set", kube: withDefault, domain: "plane-b"},
		{name: "set to a domain not in the fabric", kube: withDefault, domain: "plane-c", err: "domain plane-c not found in fabric default"},
		{name: "defaulted, fabric has no default domain", kube: namedOnly, err: "domain default not found in fabric default"},
		{name: "set, fabric has no default domain", kube: namedOnly, domain: "plane-a"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			expected := tt.domain
			if expected == "" {
				expected = wiringapi.DefaultFabricDomain
			}

			vpc := vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				if tt.domain != "" {
					vpc.Spec.Topology.Domains = []string{tt.domain}
				}
			})
			require.Equal(t, []string{expected}, vpc.Spec.Topology.Domains)
			require.Contains(t, vpc.Labels, wiringapi.ListLabelDomain(expected))
			_, vpcErr := vpc.Validate(t.Context(), tt.kube, &meta.FabricConfig{})

			ext := extGen("ext-01", func(ext *v1beta1.External) { ext.Spec.Topology.Domain = tt.domain })
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

func TestVPCDomains(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			ipv4NamespaceObj(),
			defaulted(&wiringapi.VLANNamespace{
				ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
				Spec:       wiringapi.VLANNamespaceSpec{Ranges: []meta.VLANRange{{From: 100, To: 200}}},
			}),
			&wiringapi.Fabric{
				ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
				Spec: wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{
					"plane-a": {}, "plane-b": {},
				}},
			},
		).
		Build()

	for _, tt := range []struct {
		name    string
		domains []string
		err     string
	}{
		{name: "two domains", domains: []string{"plane-b", "plane-a"}},
		{name: "one domain not in the fabric", domains: []string{"plane-a", "plane-c"}, err: "domain plane-c not found in fabric default"},
		{name: "duplicate", domains: []string{"plane-a", "plane-a"}, err: "domains must be unique"},
		{name: "empty name", domains: []string{"plane-a", ""}, err: "domain name cannot be empty"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vpc := vpcGen("vpc-domains", func(vpc *v1beta1.VPC) { vpc.Spec.Topology.Domains = slices.Clone(tt.domains) })
			require.NotContains(t, vpc.Labels, wiringapi.ListLabelDomain(""))
			_, err := vpc.Validate(t.Context(), kube, &meta.FabricConfig{})
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{"plane-a", "plane-b"}, vpc.Spec.Topology.Domains)
			require.Contains(t, vpc.Labels, wiringapi.ListLabelDomain("plane-a"))
			require.Contains(t, vpc.Labels, wiringapi.ListLabelDomain("plane-b"))
		})
	}
}

// the two-plane case: a VPC on the leaves shared by both planes, peered with a VPC of each
func TestPeeringDomains(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	vpcIn := func(name string, domains ...string) kclient.Object {
		return vpcGen(name, func(vpc *v1beta1.VPC) { vpc.Spec.Topology.Domains = domains })
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&wiringapi.Fabric{
			ObjectMeta: kmetav1.ObjectMeta{Name: wiringapi.DefaultFabric, Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{
				"plane-1": {}, "plane-2": {},
			}},
		},
		vpcIn("plane-1", "plane-1"),
		vpcIn("plane-2", "plane-2"),
		vpcIn("storage", "plane-1", "plane-2"),
		extGen("ext-1", func(ext *v1beta1.External) { ext.Spec.Topology.Domain = "plane-1" }),
	).Build()

	vpcPeering := func(vpc1, vpc2 string) func() error {
		return func() error {
			peering := &v1beta1.VPCPeering{
				ObjectMeta: kmetav1.ObjectMeta{Name: vpc1 + "--" + vpc2, Namespace: kmetav1.NamespaceDefault},
				Spec:       v1beta1.VPCPeeringSpec{Permit: []map[string]v1beta1.VPCPeer{{vpc1: {}, vpc2: {}}}},
			}
			peering.Default()
			_, err := peering.Validate(t.Context(), kube, nil)

			return err //nolint:wrapcheck
		}
	}
	extPeering := func(vpc, ext string) func() error {
		return func() error {
			peering := &v1beta1.ExternalPeering{
				ObjectMeta: kmetav1.ObjectMeta{Name: vpc + "--" + ext, Namespace: kmetav1.NamespaceDefault},
				Spec: v1beta1.ExternalPeeringSpec{Permit: v1beta1.ExternalPeeringSpecPermit{
					VPC:      v1beta1.ExternalPeeringSpecVPC{Name: vpc},
					External: v1beta1.ExternalPeeringSpecExternal{Name: ext},
				}},
			}
			peering.Default()
			_, err := peering.Validate(t.Context(), kube, nil)

			return err //nolint:wrapcheck
		}
	}

	for _, tt := range []struct {
		name     string
		validate func() error
		err      string
	}{
		{name: "vpc peering sharing one domain", validate: vpcPeering("plane-1", "storage")},
		{name: "vpc peering sharing the other domain", validate: vpcPeering("storage", "plane-2")},
		{name: "vpc peering sharing no domain", validate: vpcPeering("plane-1", "plane-2"), err: "vpc plane-1 is in domains [plane-1] and vpc plane-2 in domains [plane-2], they must share one"},
		{name: "external peering in the vpc domain", validate: extPeering("plane-1", "ext-1")},
		{name: "external peering in one of the vpc domains", validate: extPeering("storage", "ext-1")},
		{name: "external peering outside the vpc domains", validate: extPeering("plane-2", "ext-1"), err: "external ext-1 is in domain plane-1 but vpc plane-2 is in domains [plane-2]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate()
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}
