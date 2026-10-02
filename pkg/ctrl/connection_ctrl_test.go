// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConnectionReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	eslag := func(name string) *wiringapi.Connection {
		conn := &wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{
				ESLAG: &wiringapi.ConnESLAG{
					Links: []wiringapi.ServerToSwitchLink{
						{Server: wiringapi.NewBasePortName(name + "/enp2s1"), Switch: wiringapi.NewBasePortName("leaf-1/E1/1")},
						{Server: wiringapi.NewBasePortName(name + "/enp2s2"), Switch: wiringapi.NewBasePortName("leaf-2/E1/1")},
					},
				},
			},
		}
		conn.Default()

		return conn
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		eslag("conn-1"),
		// left over from a connection deleted while the controller wasn't running
		&agentapi.Catalog{
			ObjectMeta: kmetav1.ObjectMeta{Name: librarian.CatConns, Namespace: librarian.Namespace},
			Spec:       agentapi.CatalogSpec{ConnectionIDs: map[string]uint32{"gone": 7}},
		},
	).Build()

	r := &ConnectionReconciler{Client: kube, libr: librarian.NewManager(&meta.FabricConfig{})}

	reconcile := func(name string) {
		t.Helper()

		_, err := r.Reconcile(t.Context(), kctrl.Request{NamespacedName: ktypes.NamespacedName{Name: name, Namespace: kmetav1.NamespaceDefault}})
		require.NoError(t, err)
	}

	catalog := func() *agentapi.Catalog {
		t.Helper()

		cat := &agentapi.Catalog{}
		require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Name: librarian.CatConns, Namespace: librarian.Namespace}, cat))

		return cat
	}

	// allocating rebuilds the catalog, which also releases the IDs of the deleted connections
	reconcile("conn-1")
	cat := catalog()
	require.Contains(t, cat.Spec.ConnectionIDs, "conn-1")
	require.NotContains(t, cat.Spec.ConnectionIDs, "gone")

	// already allocated, the catalog isn't touched
	reconcile("conn-1")
	require.Equal(t, cat.ResourceVersion, catalog().ResourceVersion)

	// a new connection gets an ID, the existing ones keep theirs
	require.NoError(t, kube.Create(t.Context(), eslag("conn-2")))
	reconcile("conn-2")
	ids := catalog().Spec.ConnectionIDs
	require.Contains(t, ids, "conn-2")
	require.Equal(t, cat.Spec.ConnectionIDs["conn-1"], ids["conn-1"])

	// a deletion alone doesn't touch the catalog
	cat = catalog()
	require.NoError(t, kube.Delete(t.Context(), eslag("conn-2")))
	reconcile("conn-2")
	require.Equal(t, cat.ResourceVersion, catalog().ResourceVersion)

	// its ID is only released with the next allocation
	require.NoError(t, kube.Create(t.Context(), eslag("conn-3")))
	reconcile("conn-3")
	ids = catalog().Spec.ConnectionIDs
	require.Contains(t, ids, "conn-3")
	require.NotContains(t, ids, "conn-2")
}
