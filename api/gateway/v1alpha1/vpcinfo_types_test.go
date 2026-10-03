// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/gateway/v1alpha1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
