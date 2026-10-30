// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/testutil"
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
		{name: "no leaf ASN range", fabric: fabricGen("backend", 0, 0, domain(64100, 64201)), err: "spec.leafASNStart: Required value"},
		{name: "inverted leaf ASN range", fabric: fabricGen("backend", 64200, 64101, domain(64100, 64201)), err: "greater than leafASNEnd"},
		{name: "no domains", fabric: fabricGen("backend", 64101, 64200, nil), err: "at least one domain is required"},
		{name: "no spine ASN", fabric: fabricGen("backend", 64101, 64200, domain(0, 64201)), err: "spec.domains[default].spineASN: Required value"},
		{name: "no gateway ASN", fabric: fabricGen("backend", 64101, 64200, domain(64100, 0)), err: "spec.domains[default].gatewayASN: Required value"},
		{name: "spine and gateway share an ASN", fabric: fabricGen("backend", 64101, 64200, domain(64100, 64100)), err: "64100: already used as spec.domains[default]."},
		{name: "spine ASN in the leaf range", fabric: fabricGen("backend", 64101, 64200, domain(64150, 64201)), err: "spec.domains[default].spineASN: Invalid value: 64150: within the leaf ASN range"},
		{name: "gateway ASN in the leaf range", fabric: fabricGen("backend", 64101, 64200, domain(64100, 64150)), err: "spec.domains[default].gatewayASN: Invalid value: 64150: within the leaf ASN range"},
		{
			name: "two domains",
			fabric: fabricGen("backend", 64101, 64200, map[string]wiringapi.FabricDomainSpec{
				"plane-a": {SpineASN: 64100, GatewayASN: 64201},
				"plane-b": {SpineASN: 64099, GatewayASN: 64202},
			}),
		},
		{
			name: "two domains sharing a spine ASN", err: "64100: already used as spec.domains[plane-",
			fabric: fabricGen("backend", 64101, 64200, map[string]wiringapi.FabricDomainSpec{
				"plane-a": {SpineASN: 64100, GatewayASN: 64201},
				"plane-b": {SpineASN: 64100, GatewayASN: 64202},
			}),
		},
		{
			name: "invalid domain name", err: `spec.domains: Invalid value: "Plane_A"`,
			fabric: fabricGen("backend", 64101, 64200, map[string]wiringapi.FabricDomainSpec{"Plane_A": {SpineASN: 64100, GatewayASN: 64201}}),
		},
		{
			name: "long domain name", err: "spec.domains[" + strings.Repeat("d", 63) + "]: Too long",
			fabric: fabricGen("backend", 64101, 64200, map[string]wiringapi.FabricDomainSpec{strings.Repeat("d", 63): {SpineASN: 64100, GatewayASN: 64201}}),
		},
		{
			name: "long name", err: "metadata.name: Too long",
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
			err:    "spec.domains[default].spineASN: Invalid value: 65200: within fabric default leaf ASN range",
		},
		{
			name:   "gateway ASN used by other fabric",
			fabric: fabricGen("backend", 64101, 64200, domain(64100, 65534)),
			others: []kclient.Object{other},
			err:    "gatewayASN: Invalid value: 65534: already used by fabric default as spec.domains[default].gatewayASN",
		},
		{
			name:   "leaf range contains other fabric gateway ASN",
			fabric: fabricGen("backend", 65534, 65600, domain(64100, 64201)),
			others: []kclient.Object{other},
			err:    "contains fabric default spec.domains[default].gatewayASN 65534",
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
			testutil.RequireFieldError(t, err, tt.fabric)
		})
	}
}

func TestFabricReferenceRequired(t *testing.T) {
	// with or without a client to look it up
	_, err := wiringapi.CheckFabricExists(t.Context(), nil, "default", "")
	require.EqualError(t, err, "spec.topology.fabric: Required value")
	_, err = wiringapi.CheckFabricExists(t.Context(), fake.NewClientBuilder().Build(), "default", "")
	require.EqualError(t, err, "spec.topology.fabric: Required value")

	_, err = wiringapi.GetFabricSpec(t.Context(), fake.NewClientBuilder().Build(), "default", "")
	require.ErrorIs(t, err, wiringapi.ErrFabricNotSet)
}

func TestCheckFabricExists(t *testing.T) {
	spec, err := wiringapi.CheckFabricExists(t.Context(), nil, "default", "default")
	require.NoError(t, err)
	require.Nil(t, spec, "no spec without a client")

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(withName("default", &wiringapi.Fabric{Spec: wiringapi.FabricSpec{LeafASNStart: 65101}})).Build()

	spec, err = wiringapi.CheckFabricExists(t.Context(), kube, "default", "default")
	require.NoError(t, err)
	require.Equal(t, uint32(65101), spec.LeafASNStart)

	_, err = wiringapi.CheckFabricExists(t.Context(), kube, "default", "nope")
	require.EqualError(t, err, `spec.topology.fabric: Not found: "nope"`)
}
