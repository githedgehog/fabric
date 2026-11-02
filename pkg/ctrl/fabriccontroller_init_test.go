// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
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
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestWriteStatusConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, fcintapi.AddToScheme(scheme))

	key := ktypes.NamespacedName{Namespace: "fab", Name: fcintapi.FabricControllerName}
	stored := &fcintapi.FabricController{Namespace: key.Namespace, Name: key.Name}

	conflicts := 0
	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(stored).
		WithStatusSubresource(&fcintapi.FabricController{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c kclient.Client, sub string, obj kclient.Object, opts ...kclient.SubResourceUpdateOption) error {
				// as if someone changed the spec after the status writer read the object
				if conflicts < 2 {
					conflicts++

					return kapierrors.NewConflict(schema.GroupResource{Group: "fcint.githedgehog.com", Resource: "fabriccontrollers"}, obj.GetName(), nil)
				}

				return c.SubResource(sub).Update(ctx, obj, opts...)
			},
		}).
		Build()

	i := &FabricControllerInitializer{Client: kube, apiReader: kube, key: key}

	fc := &fcintapi.FabricController{}
	require.NoError(t, kube.Get(t.Context(), key, fc))
	fc.Status.InitializedVersion = "v1.2.3"
	setReady(fc, fc.Generation, kmetav1.ConditionTrue, fcintapi.ReasonInitialized, "v1.2.3")

	require.NoError(t, i.writeStatus(t.Context(), fc))
	require.Equal(t, 2, conflicts)

	got := &fcintapi.FabricController{}
	require.NoError(t, kube.Get(t.Context(), key, got))
	require.Equal(t, "v1.2.3", got.Status.InitializedVersion, "the status is written again after re-reading")
	require.True(t, kmeta.IsStatusConditionTrue(got.Status.Conditions, fcintapi.ConditionReady))
}

func reconcileInit(t *testing.T, i *FabricControllerInitializer) (kctrl.Result, error) {
	t.Helper()

	return i.Reconcile(t.Context(), kctrl.Request{NamespacedName: i.key})
}

func initScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, fcintapi.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	return scheme
}

func initFabricConfig() *meta.FabricConfig {
	return &meta.FabricConfig{
		SpineASN:                 65100,
		LeafASNStart:             65101,
		LeafASNEnd:               65533,
		GatewayASN:               65534,
		DisableBFD:               true,
		AllowExtraSwitchProfiles: true,
	}
}

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

	cfg := initFabricConfig()

	defaultFabric := ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric}

	// devel builds never count as initialized
	reinit := ver == develVersion

	// objects without a spec are invalid, so they fail the initialization: it still does all its steps
	for _, tt := range []struct {
		name          string
		objs          []kclient.Object
		initializes   bool
		invalid       bool
		defaultFabric bool
		deletedConns  []string
		keptConns     []string
	}{
		{
			name:        "fresh deployment",
			initializes: true,
		},
		{
			name: "upgrade from before fabrics",
			objs: []kclient.Object{
				&wiringapi.Switch{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"},
			},
			initializes:   true,
			invalid:       true,
			defaultFabric: true,
		},
		{
			name: "fabrics already exist",
			objs: []kclient.Object{
				&wiringapi.Fabric{Namespace: kmetav1.NamespaceDefault, Name: "other"},
				&wiringapi.Switch{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"},
			},
			initializes: true,
			invalid:     true,
		},
		{
			name: "upgrade with vpc-loopback connections",
			objs: []kclient.Object{
				&wiringapi.Fabric{Namespace: kmetav1.NamespaceDefault, Name: "other"},
				&wiringapi.Connection{
					Namespace: kmetav1.NamespaceDefault, Name: "leaf-01--vpc-loopback",
					Labels: map[string]string{wiringapi.LabelConnectionType: "vpc-loopback"},
				},
				&wiringapi.Connection{
					Namespace: kmetav1.NamespaceDefault, Name: "spine-01--fabric--leaf-01",
					Labels: map[string]string{wiringapi.LabelConnectionType: wiringapi.ConnectionTypeFabric},
				},
			},
			initializes:  true,
			invalid:      true,
			deletedConns: []string{"leaf-01--vpc-loopback"},
			keptConns:    []string{"spine-01--fabric--leaf-01"},
		},
		{
			name: "already initialized",
			objs: []kclient.Object{
				readyFabricController(ns, ver, 1),
				&wiringapi.Switch{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"},
			},
			initializes:   reinit,
			invalid:       true,
			defaultFabric: reinit,
		},
		{
			name: "initialized for an older spec",
			objs: []kclient.Object{
				readyFabricController(ns, ver, 2),
				&wiringapi.Switch{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"},
			},
			initializes:   true,
			invalid:       true,
			defaultFabric: true,
		},
		{
			name: "initialized by another version",
			objs: []kclient.Object{
				readyFabricController(ns, "v0.0.1", 1),
				&wiringapi.Switch{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"},
			},
			initializes:   true,
			invalid:       true,
			defaultFabric: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().
				WithScheme(initScheme(t)).
				WithObjects(tt.objs...).
				WithStatusSubresource(&fcintapi.FabricController{}).
				Build()

			profiles := switchprofile.NewDefaultSwitchProfiles()
			require.NoError(t, profiles.RegisterAll(t.Context(), kube, cfg))

			i := &FabricControllerInitializer{
				Client:    kube,
				apiReader: kube,
				key:       ktypes.NamespacedName{Namespace: ns, Name: fcintapi.FabricControllerName},
				version:   ver,
				cfg:       cfg,
				profiles:  profiles,
			}

			res, err := reconcileInit(t, i)
			require.NoError(t, err)
			require.Equal(t, kctrl.Result{RequeueAfter: initCheckInterval}, res)

			fc := &fcintapi.FabricController{}
			require.NoError(t, kube.Get(t.Context(), i.key, fc))
			failed := tt.initializes && tt.invalid
			require.Equal(t, !failed, fc.IsReady(ver))
			require.Equal(t, failed, kmeta.IsStatusConditionPresentAndEqual(fc.Status.Conditions, fcintapi.ConditionReady, kmetav1.ConditionFalse))
			require.Equal(t, failed, len(fc.Status.Problems) > 0)
			require.Equal(t, ver, fc.Status.Version)
			require.Empty(t, fc.Status.Error)

			fabric := &wiringapi.Fabric{}
			err = kube.Get(t.Context(), defaultFabric, fabric)
			if tt.defaultFabric {
				require.NoError(t, err)
				require.Equal(t, wiringapi.DefaultFabricSpec(cfg), fabric.Spec)
			} else {
				require.True(t, kapierrors.IsNotFound(err), "expected no default fabric, got %v", err)
			}

			require.Equal(t, tt.initializes, !fc.Status.Refresh.StartedAt.IsZero(), "refreshed")

			for _, name := range tt.deletedConns {
				err := kube.Get(t.Context(), ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: name}, &wiringapi.Connection{})
				require.True(t, kapierrors.IsNotFound(err), "expected connection %s to be deleted, got %v", name, err)
			}
			for _, name := range tt.keptConns {
				require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: name}, &wiringapi.Connection{}))
			}

			// built-in switch profiles are enforced on every start, an already initialized version included
			sps := &wiringapi.SwitchProfileList{}
			require.NoError(t, kube.List(t.Context(), sps))
			require.NotEmpty(t, sps.Items)
			require.Len(t, sps.Items, len(profiles.List()))

			if failed {
				require.Equal(t, fcintapi.ReasonFailed, kmeta.FindStatusCondition(fc.Status.Conditions, fcintapi.ConditionReady).Reason)

				return
			}

			// reconciling an initialized one again is a no-op
			startedAt := fc.Status.Refresh.StartedAt
			_, err = reconcileInit(t, i)
			require.NoError(t, err)
			require.NoError(t, kube.Get(t.Context(), i.key, fc))
			require.Equal(t, startedAt, fc.Status.Refresh.StartedAt)
		})
	}
}

// readyFabricController is initialized for the generation 1, gen is the current one
func readyFabricController(ns, ver string, gen int64) *fcintapi.FabricController {
	fc := &fcintapi.FabricController{
		Namespace: ns, Name: fcintapi.FabricControllerName, Generation: gen,
		Status: fcintapi.FabricControllerStatus{InitializedVersion: ver},
	}
	setReady(fc, 1, kmetav1.ConditionTrue, fcintapi.ReasonInitialized, ver)

	return fc
}

func TestFabricControllerInitializerLifecycle(t *testing.T) {
	const ver = "v1.2.3"

	// refreshes are counted by the Switch lists, only the initialization lists them
	lists := 0
	failVPCs := false
	kube := fake.NewClientBuilder().
		WithScheme(initScheme(t)).
		WithObjects(&wiringapi.Fabric{
			Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric,
			Spec: wiringapi.DefaultFabricSpec(initFabricConfig()),
		}).
		WithStatusSubresource(&fcintapi.FabricController{}).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
				switch list.(type) {
				case *wiringapi.SwitchList:
					lists++
				case *vpcapi.VPCList:
					if failVPCs {
						return kapierrors.NewServiceUnavailable("etcd is down")
					}
				}

				return c.List(ctx, list, opts...)
			},
		}).
		Build()

	newInitializer := func() *FabricControllerInitializer {
		cfg := initFabricConfig()
		profiles := switchprofile.NewDefaultSwitchProfiles()
		require.NoError(t, profiles.RegisterAll(t.Context(), kube, cfg))

		return &FabricControllerInitializer{
			Client:    kube,
			apiReader: kube,
			key:       ktypes.NamespacedName{Namespace: "fab", Name: fcintapi.FabricControllerName},
			version:   ver,
			cfg:       cfg,
			profiles:  profiles,
		}
	}
	i := newInitializer()

	get := func() *fcintapi.FabricController {
		fc := &fcintapi.FabricController{}
		require.NoError(t, kube.Get(t.Context(), i.key, fc))

		return fc
	}
	// the fake client doesn't bump the generation on spec changes
	setMaintenance := func(maintenance bool) {
		fc := get()
		fc.Spec.Maintenance = maintenance
		fc.Generation++
		require.NoError(t, kube.Update(t.Context(), fc))
	}
	requireState := func(reason, msg string) *fcintapi.FabricController {
		fc := get()
		ready := kmeta.FindStatusCondition(fc.Status.Conditions, fcintapi.ConditionReady)
		require.NotNil(t, ready)
		require.Equal(t, reason, ready.Reason)
		require.Equal(t, msg, ready.Message)
		require.Equal(t, fc.Generation, ready.ObservedGeneration)

		return fc
	}

	t.Run("new deployment", func(t *testing.T) {
		_, err := reconcileInit(t, i)
		require.NoError(t, err)
		require.True(t, requireState(fcintapi.ReasonInitialized, ver).IsReady(ver))
		require.Equal(t, 1, lists)
	})

	t.Run("maintenance pauses it", func(t *testing.T) {
		setMaintenance(true)
		res, err := reconcileInit(t, i)
		require.NoError(t, err)
		require.Equal(t, kctrl.Result{}, res)
		require.False(t, requireState(fcintapi.ReasonMaintenance, "nothing is deployed").IsReady(ver))
		require.Equal(t, 1, lists, "no refresh in maintenance")
	})

	t.Run("leaving maintenance initializes again", func(t *testing.T) {
		setMaintenance(false)
		_, err := reconcileInit(t, i)
		require.NoError(t, err)
		require.True(t, requireState(fcintapi.ReasonInitialized, ver).IsReady(ver))
		require.Equal(t, 2, lists)
	})

	t.Run("restart of the same version", func(t *testing.T) {
		i = newInitializer()
		_, err := reconcileInit(t, i)
		require.NoError(t, err)
		require.True(t, get().IsReady(ver))
		require.Equal(t, 2, lists, "no refresh on a restart")
		require.True(t, i.enforced, "built-in switch profiles are enforced on every start")
	})

	t.Run("retried after an error", func(t *testing.T) {
		require.NoError(t, kube.Delete(t.Context(), get()))

		failVPCs = true
		_, err := reconcileInit(t, i)
		require.ErrorContains(t, err, "etcd is down")
		fc := requireState(fcintapi.ReasonInitializing, "retrying, see status.error")
		require.Contains(t, fc.Status.Error, "etcd is down")
		require.False(t, fc.IsReady(ver))

		failVPCs = false
		_, err = reconcileInit(t, i)
		require.NoError(t, err)
		fc = requireState(fcintapi.ReasonInitialized, ver)
		require.Empty(t, fc.Status.Error)
		require.True(t, fc.IsReady(ver))
	})
}

// a spec change while initializing, e.g. maintenance turned on and off again, needs another initialization
func TestFabricControllerInitializerSpecChange(t *testing.T) {
	const ver = "v1.2.3"

	key := ktypes.NamespacedName{Namespace: "fab", Name: fcintapi.FabricControllerName}
	changed := false
	kube := fake.NewClientBuilder().
		WithScheme(initScheme(t)).
		WithObjects(&fcintapi.FabricController{Namespace: key.Namespace, Name: key.Name, Generation: 1}).
		WithStatusSubresource(&fcintapi.FabricController{}).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
				if _, ok := list.(*wiringapi.SwitchList); ok && !changed {
					changed = true

					fc := &fcintapi.FabricController{}
					require.NoError(t, c.Get(ctx, key, fc))
					fc.Generation++
					require.NoError(t, c.Update(ctx, fc))
				}

				return c.List(ctx, list, opts...)
			},
		}).
		Build()

	i := &FabricControllerInitializer{
		Client:    kube,
		apiReader: kube,
		key:       key,
		version:   ver,
		cfg:       initFabricConfig(),
		profiles:  switchprofile.NewDefaultSwitchProfiles(),
	}

	_, err := reconcileInit(t, i)
	require.NoError(t, err)
	require.True(t, changed)

	fc := &fcintapi.FabricController{}
	require.NoError(t, kube.Get(t.Context(), key, fc))
	require.False(t, fc.IsReady(ver), "initialized for the spec it started with")

	_, err = reconcileInit(t, i)
	require.NoError(t, err)
	require.NoError(t, kube.Get(t.Context(), key, fc))
	require.True(t, fc.IsReady(ver))
}
