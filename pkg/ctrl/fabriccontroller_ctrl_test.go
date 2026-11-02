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
		name   string
		fc     *fcintapi.FabricController
		before lockState
		after  lockState
	}{
		{
			name:  "not found",
			after: lockLocked,
		},
		{
			name:   "not found locks an unlocked one",
			before: lockUnlocked,
			after:  lockLocked,
		},
		{
			name: "not initialized",
			fc: &fcintapi.FabricController{
				Namespace: ns, Name: fcintapi.FabricControllerName,
			},
			after: lockLocked,
		},
		{
			name:  "initialized",
			fc:    readyFabricController(ns, version.Version, 1),
			after: lockUnlocked,
		},
		{
			name:   "initialized by another version",
			fc:     readyFabricController(ns, "v0.0.1", 1),
			before: lockUnlocked,
			after:  lockLocked,
		},
		{
			name:   "initialized for an older spec",
			fc:     readyFabricController(ns, version.Version, 2),
			before: lockUnlocked,
			after:  lockLocked,
		},
		{
			name: "initialized version without ready",
			fc: &fcintapi.FabricController{
				Namespace: ns, Name: fcintapi.FabricControllerName,
				Status: fcintapi.FabricControllerStatus{InitializedVersion: version.Version},
			},
			after: lockLocked,
		},
		{
			name: "maintenance",
			fc: &fcintapi.FabricController{
				Namespace: ns, Name: fcintapi.FabricControllerName,
				Spec:   fcintapi.FabricControllerSpec{Maintenance: true},
				Status: fcintapi.FabricControllerStatus{InitializedVersion: "v0.0.1"},
			},
			after: lockMaintenance,
		},
		{
			name: "maintenance of an initialized one",
			fc: func() *fcintapi.FabricController {
				fc := readyFabricController(ns, version.Version, 1)
				fc.Spec.Maintenance = true

				return fc
			}(),
			before: lockUnlocked,
			after:  lockMaintenance,
		},
		{
			name:  "other namespace",
			fc:    readyFabricController(kmetav1.NamespaceDefault, version.Version, 1),
			after: lockLocked,
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
			lock.state.Store(int32(tt.before))

			r := &FabricControllerReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
				key:    ktypes.NamespacedName{Namespace: ns, Name: fcintapi.FabricControllerName},
				lock:   lock,
			}

			_, err := r.Reconcile(t.Context(), kctrl.Request{})
			require.NoError(t, err)
			require.Equal(t, tt.after, lock.get())
		})
	}
}

func TestLockStartsLocked(t *testing.T) {
	lock := NewLock("system:serviceaccount:fab:fabric-ctrl")

	require.True(t, lock.Locked())
	require.True(t, lock.IsSelf("system:serviceaccount:fab:fabric-ctrl"))
	require.False(t, lock.IsSelf("system:admin"))
}
