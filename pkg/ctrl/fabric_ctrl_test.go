// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureDefaultFabric(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	cfg := &meta.FabricConfig{
		SpineASN:              65100,
		LeafASNStart:          65101,
		LeafASNEnd:            65533,
		GatewayASN:            65534,
		FabricMTU:             9100,
		ServerFacingMTUOffset: 64,
		DefaultMaxPathsEBGP:   64,
		DisableBFD:            true,
	}
	key := ktypes.NamespacedName{Name: wiringapi.DefaultFabric, Namespace: kmetav1.NamespaceDefault}
	configFields := wiringapi.FabricSpec{FabricMTU: 9100, ServerFacingMTUOffset: 64, DefaultMaxPathsEBGP: 64, DisableBFD: true}

	for _, tt := range []struct {
		name     string
		existing []kclient.Object
		leafEnd  uint32
	}{
		{name: "created", leafEnd: 65533},
		// written before the per-fabric config fields existed, with ASNs that no longer match the config
		{name: "filled in", leafEnd: 65400, existing: []kclient.Object{&wiringapi.Fabric{
			ObjectMeta: kmetav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
			Spec: wiringapi.FabricSpec{
				LeafASNStart: 65101,
				LeafASNEnd:   65400,
				Domains:      map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {SpineASN: 65100, GatewayASN: 65534}},
			},
		}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.existing...).Build()
			i := &FabricInitializer{Client: kube, cfg: cfg}

			require.NoError(t, i.ensureDefaultFabric(t.Context()))

			fabric := &wiringapi.Fabric{}
			require.NoError(t, kube.Get(t.Context(), key, fabric))

			expected := configFields
			expected.LeafASNStart, expected.LeafASNEnd = 65101, tt.leafEnd
			expected.Domains = map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {SpineASN: 65100, GatewayASN: 65534}}
			require.Equal(t, expected, fabric.Spec)

			require.NoError(t, i.ensureDefaultFabric(t.Context()))
		})
	}
}
