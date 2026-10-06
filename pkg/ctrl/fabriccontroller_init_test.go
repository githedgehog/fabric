// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFabricControllerInitializer(t *testing.T) {
	for _, ver := range []string{develVersion, "v1.2.3"} {
		t.Run(ver, func(t *testing.T) {
			testFabricControllerInitializer(t, ver)
		})
	}
}

func testFabricControllerInitializer(t *testing.T, ver string) {
	t.Helper()

	const ns = "fab"

	cfg := &meta.FabricConfig{
		SpineASN:                 65100,
		LeafASNStart:             65101,
		LeafASNEnd:               65533,
		GatewayASN:               65534,
		DisableBFD:               true,
		AllowExtraSwitchProfiles: true,
	}

	defaultFabric := ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric}

	// devel builds never count as initialized
	reinit := ver == develVersion

	for _, tt := range []struct {
		name          string
		objs          []kclient.Object
		initializes   bool
		defaultFabric bool
	}{
		{
			name:        "fresh deployment",
			initializes: true,
		},
		{
			name: "upgrade from before fabrics",
			objs: []kclient.Object{
				&wiringapi.Switch{ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"}},
			},
			initializes:   true,
			defaultFabric: true,
		},
		{
			name: "fabrics already exist",
			objs: []kclient.Object{
				&wiringapi.Fabric{ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: "other"}},
				&wiringapi.Switch{ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"}},
			},
			initializes: true,
		},
		{
			name: "already initialized",
			objs: []kclient.Object{
				&fcintapi.FabricController{
					ObjectMeta: kmetav1.ObjectMeta{Namespace: ns, Name: fcintapi.FabricControllerName},
					Status:     fcintapi.FabricControllerStatus{InitializedVersion: ver},
				},
				&wiringapi.Switch{ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"}},
			},
			initializes:   reinit,
			defaultFabric: reinit,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, fcintapi.AddToScheme(scheme))
			require.NoError(t, wiringapi.AddToScheme(scheme))
			require.NoError(t, vpcapi.AddToScheme(scheme))
			require.NoError(t, gwapi.AddToScheme(scheme))

			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objs...).
				WithStatusSubresource(&fcintapi.FabricController{}).
				Build()

			profiles := switchprofile.NewDefaultSwitchProfiles()

			i := &FabricControllerInitializer{
				Client:   kube,
				key:      ktypes.NamespacedName{Namespace: ns, Name: fcintapi.FabricControllerName},
				version:  ver,
				cfg:      cfg,
				profiles: profiles,
			}

			require.NoError(t, i.Start(t.Context()))

			fc := &fcintapi.FabricController{}
			require.NoError(t, kube.Get(t.Context(), i.key, fc))
			require.Equal(t, ver, fc.Status.InitializedVersion)

			fabric := &wiringapi.Fabric{}
			err := kube.Get(t.Context(), defaultFabric, fabric)
			if tt.defaultFabric {
				require.NoError(t, err)
				require.Equal(t, wiringapi.DefaultFabricSpec(cfg), fabric.Spec)
			} else {
				require.True(t, kapierrors.IsNotFound(err), "expected no default fabric, got %v", err)
			}

			require.Equal(t, tt.initializes, kmeta.IsStatusConditionTrue(fc.Status.Conditions, fcintapi.ConditionInitialized))
			require.Equal(t, tt.initializes, profiles.IsInitialized())

			// initializing again is a no-op
			require.NoError(t, i.Start(t.Context()))
		})
	}
}
