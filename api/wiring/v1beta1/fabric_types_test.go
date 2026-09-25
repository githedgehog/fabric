// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFabricValidation(t *testing.T) {
	fabricGen := func(name string, asnStart, asnEnd uint32, domains map[string]wiringapi.FabricDomainSpec) *wiringapi.Fabric {
		fabric := withName(name, &wiringapi.Fabric{Spec: wiringapi.FabricSpec{
			ASNStart: asnStart,
			ASNEnd:   asnEnd,
			Domains:  domains,
		}})
		fabric.Default()

		return fabric
	}
	spine := func(asn uint32) map[string]wiringapi.FabricDomainSpec {
		return map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {SpineASN: asn}}
	}

	implicit := fabricGen("backend", 64100, 64200, nil)
	require.Equal(t, map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {}}, implicit.Spec.Domains)

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, tt := range []struct {
		name   string
		fabric *wiringapi.Fabric
		others []kclient.Object
		err    string
	}{
		{name: "valid", fabric: fabricGen("backend", 64100, 64200, spine(64100))},
		{name: "valid, named domain", fabric: fabricGen("backend", 64100, 64200, map[string]wiringapi.FabricDomainSpec{"plane-a": {SpineASN: 64150}})},
		{name: "no ASN range", fabric: fabricGen("backend", 0, 0, spine(64100)), err: "asnStart and asnEnd are required"},
		{name: "inverted ASN range", fabric: fabricGen("backend", 64200, 64100, spine(64100)), err: "greater than asnEnd"},
		// the implicit domain gets no spine ASN from defaulting, so the user must set it
		{name: "implicit domain without spine ASN", fabric: implicit, err: "spineASN 0 is not within"},
		{name: "spine ASN outside the range", fabric: fabricGen("backend", 64100, 64200, spine(64300)), err: "spineASN 64300 is not within"},
		{
			name: "two domains", err: "more than one domain",
			fabric: fabricGen("backend", 64100, 64200, map[string]wiringapi.FabricDomainSpec{
				"plane-a": {SpineASN: 64100},
				"plane-b": {SpineASN: 64101},
			}),
		},
		{
			name: "long name", err: "too long",
			fabric: fabricGen(strings.Repeat("f", 64), 64100, 64200, spine(64100)),
		},
		{
			name:   "disjoint from other fabric",
			fabric: fabricGen("backend", 64100, 64200, spine(64100)),
			others: []kclient.Object{fabricGen("default", 65100, 65200, spine(65100))},
		},
		{
			name:   "overlaps other fabric",
			fabric: fabricGen("backend", 64100, 65100, spine(64100)),
			others: []kclient.Object{fabricGen("default", 65100, 65200, spine(65100))},
			err:    "overlaps with fabric default",
		},
		{
			name:   "updating itself is not an overlap",
			fabric: fabricGen("backend", 64100, 64200, spine(64100)),
			others: []kclient.Object{fabricGen("backend", 64100, 64200, spine(64100))},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.others...).Build()
			_, err := tt.fabric.Validate(t.Context(), kube, nil)
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}
