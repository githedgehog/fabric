// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/pkg/version"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFabricControllerLock(t *testing.T) {
	const ns = "fab"

	for _, tt := range []struct {
		name     string
		fc       *fcintapi.FabricController
		unlocked bool // the lock state before reconcile
		locked   bool
	}{
		{
			name:   "not found",
			locked: true,
		},
		{
			name:     "not found locks an unlocked one",
			unlocked: true,
			locked:   true,
		},
		{
			name: "not initialized",
			fc: &fcintapi.FabricController{
				ObjectMeta: kmetav1.ObjectMeta{Namespace: ns, Name: fcintapi.FabricControllerName},
			},
			locked: true,
		},
		{
			name: "initialized",
			fc: &fcintapi.FabricController{
				ObjectMeta: kmetav1.ObjectMeta{Namespace: ns, Name: fcintapi.FabricControllerName},
				Status:     fcintapi.FabricControllerStatus{InitializedVersion: version.Version},
			},
			locked: false,
		},
		{
			name: "initialized by another version",
			fc: &fcintapi.FabricController{
				ObjectMeta: kmetav1.ObjectMeta{Namespace: ns, Name: fcintapi.FabricControllerName},
				Status:     fcintapi.FabricControllerStatus{InitializedVersion: "v0.0.1"},
			},
			unlocked: true,
			locked:   true,
		},
		{
			name: "force unlocked",
			fc: &fcintapi.FabricController{
				ObjectMeta: kmetav1.ObjectMeta{Namespace: ns, Name: fcintapi.FabricControllerName},
				Spec:       fcintapi.FabricControllerSpec{ForceUnlock: true},
				Status:     fcintapi.FabricControllerStatus{InitializedVersion: "v0.0.1"},
			},
			locked: false,
		},
		{
			name: "other namespace",
			fc: &fcintapi.FabricController{
				ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: fcintapi.FabricControllerName},
				Status:     fcintapi.FabricControllerStatus{InitializedVersion: version.Version},
			},
			locked: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, fcintapi.AddToScheme(scheme))

			objs := []kclient.Object{}
			if tt.fc != nil {
				objs = append(objs, tt.fc)
			}

			lock := NewLock("system:serviceaccount:fab:fabric-ctrl")
			lock.unlocked.Store(tt.unlocked)

			r := &FabricControllerReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
				key:    ktypes.NamespacedName{Namespace: ns, Name: fcintapi.FabricControllerName},
				lock:   lock,
			}

			_, err := r.Reconcile(t.Context(), kctrl.Request{})
			require.NoError(t, err)
			require.Equal(t, tt.locked, lock.Locked())
		})
	}
}

func TestLockStartsLocked(t *testing.T) {
	lock := NewLock("system:serviceaccount:fab:fabric-ctrl")

	require.True(t, lock.Locked())
	require.True(t, lock.IsSelf("system:serviceaccount:fab:fabric-ctrl"))
	require.False(t, lock.IsSelf("system:admin"))
}
