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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureDefaultFabric(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	i := &FabricInitializer{
		Client: kube,
		cfg: &meta.FabricConfig{
			SpineASN:     65100,
			LeafASNStart: 65101,
			LeafASNEnd:   65533,
			GatewayASN:   65534,
		},
	}

	require.NoError(t, i.ensureDefaultFabric(t.Context()))

	fabric := &wiringapi.Fabric{}
	require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{
		Name:      wiringapi.DefaultFabric,
		Namespace: kmetav1.NamespaceDefault,
	}, fabric))

	// the spine ASN sits below the leaf range and the gateway ASN above it, as in the defaults
	require.Equal(t, uint32(65100), fabric.Spec.ASNStart)
	require.Equal(t, uint32(65534), fabric.Spec.ASNEnd)
	require.Equal(t, uint32(65100), fabric.Spec.Domains[wiringapi.DefaultFabricDomain].SpineASN)

	require.NoError(t, i.ensureDefaultFabric(t.Context()))
}
