// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/gateway/v1alpha1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVPCInfoDefaultTopology(t *testing.T) {
	for _, tt := range []struct {
		name     string
		topology v1alpha1.VPCInfoTopology
		labels   map[string]string
		expected v1alpha1.VPCInfoTopology
		expLabel map[string]string
	}{
		{
			name:     "empty",
			expected: v1alpha1.VPCInfoTopology{Fabric: wiringapi.DefaultFabric, Domains: []string{wiringapi.DefaultFabricDomain}},
			expLabel: map[string]string{
				wiringapi.ListLabelFabric(wiringapi.DefaultFabric):       v1alpha1.ListLabelValue,
				wiringapi.ListLabelDomain(wiringapi.DefaultFabricDomain): v1alpha1.ListLabelValue,
			},
		},
		{
			name:     "domains sorted, stale labels replaced",
			topology: v1alpha1.VPCInfoTopology{Fabric: "f1", Domains: []string{"d2", "d1"}},
			labels: map[string]string{
				wiringapi.ListLabelFabric(wiringapi.DefaultFabric): v1alpha1.ListLabelValue,
				"other": "kept",
			},
			expected: v1alpha1.VPCInfoTopology{Fabric: "f1", Domains: []string{"d1", "d2"}},
			expLabel: map[string]string{
				wiringapi.ListLabelFabric("f1"): v1alpha1.ListLabelValue,
				wiringapi.ListLabelDomain("d1"): v1alpha1.ListLabelValue,
				wiringapi.ListLabelDomain("d2"): v1alpha1.ListLabelValue,
				"other":                         "kept",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vpc := &v1alpha1.VPCInfo{
				ObjectMeta: kmetav1.ObjectMeta{Name: "vpc-1", Labels: tt.labels},
				Spec:       v1alpha1.VPCInfoSpec{Topology: tt.topology},
			}
			vpc.Default()

			require.Equal(t, kmetav1.NamespaceDefault, vpc.Namespace)
			require.Equal(t, tt.expected, vpc.Spec.Topology)
			require.Equal(t, tt.expLabel, vpc.Labels)
		})
	}
}

func TestVPCInfoDomainsInFabric(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	fabric := &wiringapi.Fabric{
		ObjectMeta: kmetav1.ObjectMeta{Name: wiringapi.DefaultFabric, Namespace: kmetav1.NamespaceDefault},
		Spec: wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{
			wiringapi.DefaultFabricDomain: {SpineASN: 65100, GatewayASN: 65534},
			"plane-b":                     {SpineASN: 65099, GatewayASN: 65535},
		}},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fabric).Build()

	for _, tt := range []struct {
		name    string
		domains []string
		noKube  bool
		err     string
	}{
		{name: "default domain", domains: []string{wiringapi.DefaultFabricDomain}},
		{name: "both domains", domains: []string{wiringapi.DefaultFabricDomain, "plane-b"}},
		{name: "unknown domain", domains: []string{"plane-c"}, err: "domain plane-c not found in fabric default"},
		{name: "known and unknown domain", domains: []string{wiringapi.DefaultFabricDomain, "plane-c"}, err: "domain plane-c not found in fabric default"},
		// nothing to look the fabric up with
		{name: "unknown domain without a client", domains: []string{"plane-c"}, noKube: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vpc := &v1alpha1.VPCInfo{
				ObjectMeta: kmetav1.ObjectMeta{Name: "vpc-1"},
				Spec: v1alpha1.VPCInfoSpec{
					Topology: v1alpha1.VPCInfoTopology{Domains: tt.domains},
					VNI:      100,
				},
			}
			vpc.Default()

			var reader kclient.Reader = kube
			if tt.noKube {
				reader = nil
			}

			err := vpc.Validate(t.Context(), reader, nil)
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}
