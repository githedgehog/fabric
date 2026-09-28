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
	fabricGen := func(name string, leafASNStart, leafASNEnd uint32, domains map[string]wiringapi.FabricDomainSpec) *wiringapi.Fabric {
		fabric := withName(name, &wiringapi.Fabric{Spec: wiringapi.FabricSpec{
			LeafASNStart: leafASNStart,
			LeafASNEnd:   leafASNEnd,
			Domains:      domains,
		}})
		fabric.Default()

		return fabric
	}
	domain := func(spineASN, gatewayASN uint32) map[string]wiringapi.FabricDomainSpec {
		return map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {SpineASN: spineASN, GatewayASN: gatewayASN}}
	}

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	other := fabricGen("default", 65101, 65533, domain(65100, 65534))

	for _, tt := range []struct {
		name   string
		fabric *wiringapi.Fabric
		others []kclient.Object
		err    string
	}{
		{name: "valid", fabric: fabricGen("backend", 64101, 64200, domain(64100, 64201))},
		{name: "valid, named domain", fabric: fabricGen("backend", 64101, 64200, map[string]wiringapi.FabricDomainSpec{"plane-a": {SpineASN: 64100, GatewayASN: 64201}})},
		{name: "no leaf ASN range", fabric: fabricGen("backend", 0, 0, domain(64100, 64201)), err: "leafASNStart and leafASNEnd are required"},
		{name: "inverted leaf ASN range", fabric: fabricGen("backend", 64200, 64101, domain(64100, 64201)), err: "greater than leafASNEnd"},
		{name: "no domains", fabric: fabricGen("backend", 64101, 64200, nil), err: "at least one domain is required"},
		{name: "no spine ASN", fabric: fabricGen("backend", 64101, 64200, domain(0, 64201)), err: "domain default spineASN is required"},
		{name: "no gateway ASN", fabric: fabricGen("backend", 64101, 64200, domain(64100, 0)), err: "domain default gatewayASN is required"},
		{name: "spine and gateway share an ASN", fabric: fabricGen("backend", 64101, 64200, domain(64100, 64100)), err: "64100 is already used as domain default"},
		{name: "spine ASN in the leaf range", fabric: fabricGen("backend", 64101, 64200, domain(64150, 64201)), err: "domain default spineASN 64150 is within the leaf ASN range"},
		{name: "gateway ASN in the leaf range", fabric: fabricGen("backend", 64101, 64200, domain(64100, 64150)), err: "domain default gatewayASN 64150 is within the leaf ASN range"},
		{
			name: "two domains", err: "more than one domain",
			fabric: fabricGen("backend", 64101, 64200, map[string]wiringapi.FabricDomainSpec{
				"plane-a": {SpineASN: 64100, GatewayASN: 64201},
				"plane-b": {SpineASN: 64099, GatewayASN: 64202},
			}),
		},
		{
			name: "long name", err: "too long",
			fabric: fabricGen(strings.Repeat("f", 64), 64101, 64200, domain(64100, 64201)),
		},
		{
			name:   "disjoint from other fabric",
			fabric: fabricGen("backend", 64101, 64200, domain(64100, 64201)),
			others: []kclient.Object{other},
		},
		{
			name:   "leaf range overlaps other fabric",
			fabric: fabricGen("backend", 64101, 65101, domain(64100, 65600)),
			others: []kclient.Object{other},
			err:    "overlaps with fabric default leaf ASN range",
		},
		{
			name:   "spine ASN in other fabric leaf range",
			fabric: fabricGen("backend", 64101, 64200, domain(65200, 64201)),
			others: []kclient.Object{other},
			err:    "domain default spineASN 65200 is within fabric default leaf ASN range",
		},
		{
			name:   "gateway ASN used by other fabric",
			fabric: fabricGen("backend", 64101, 64200, domain(64100, 65534)),
			others: []kclient.Object{other},
			err:    "gatewayASN 65534 is already used by fabric default as domain default gatewayASN",
		},
		{
			name:   "leaf range contains other fabric gateway ASN",
			fabric: fabricGen("backend", 65534, 65600, domain(64100, 64201)),
			others: []kclient.Object{other},
			err:    "contains fabric default domain default gatewayASN 65534",
		},
		{
			name:   "updating itself is not an overlap",
			fabric: fabricGen("backend", 64101, 64200, domain(64100, 64201)),
			others: []kclient.Object{fabricGen("backend", 64101, 64200, domain(64100, 64201))},
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
