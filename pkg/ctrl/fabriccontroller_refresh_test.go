// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"strings"
	"sync"
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
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestDefaultedEqual(t *testing.T) {
	sw := &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"},
		Spec:       wiringapi.SwitchSpec{Profile: "vs"},
	}
	sw.Default()

	same := sw.DeepCopy()
	same.ResourceVersion = "42"
	same.ManagedFields = []kmetav1.ManagedFieldsEntry{{Manager: "kubectl"}}
	require.True(t, defaultedEqual(sw, same), "resource version and managed fields don't count")

	label := sw.DeepCopy()
	label.Labels["extra"] = "true"
	require.False(t, defaultedEqual(sw, label))

	ann := sw.DeepCopy()
	ann.Annotations = map[string]string{"extra": "true"}
	require.False(t, defaultedEqual(sw, ann))

	spec := sw.DeepCopy()
	spec.Spec.Description = "changed"
	require.False(t, defaultedEqual(sw, spec))

	emptyLabels := &wiringapi.VLANNamespace{}
	nilLabels := emptyLabels.DeepCopy()
	emptyLabels.Labels = map[string]string{}
	require.True(t, defaultedEqual(emptyLabels, nilLabels), "nil and empty labels are the same")
}

func TestFabricControllerRefresh(t *testing.T) {
	const ns = "fab"

	scheme := runtime.NewScheme()
	require.NoError(t, fcintapi.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	newSwitch := func(name string, defaulted bool) *wiringapi.Switch {
		sw := &wiringapi.Switch{
			ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: name},
			Spec:       wiringapi.SwitchSpec{Profile: "vs"},
		}
		if defaulted {
			sw.Default()
		}

		return sw
	}

	mu := sync.Mutex{}
	updates := map[string]int{}

	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&wiringapi.Fabric{ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric}},
			newSwitch("old", false),
			newSwitch("current", true),
			newSwitch("rejected", false),
			newSwitch("stale", false),
			newSwitch("flaky", false),
		).
		WithStatusSubresource(&fcintapi.FabricController{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
				if _, ok := obj.(*wiringapi.Switch); !ok {
					return c.Update(ctx, obj, opts...)
				}

				mu.Lock()
				updates[obj.GetName()]++
				attempt := updates[obj.GetName()]
				mu.Unlock()

				switch obj.GetName() {
				case "rejected":
					return kapierrors.NewInvalid(schema.GroupKind{Group: "wiring.githedgehog.com", Kind: "Switch"}, obj.GetName(), nil)
				case "flaky":
					if attempt <= 2 {
						return kapierrors.NewInternalError(context.DeadlineExceeded)
					}
				case "stale":
					// as if another version's webhook defaulted it
					obj.SetLabels(nil)
				}

				return c.Update(ctx, obj, opts...)
			},
		}).
		Build()

	profiles := switchprofile.NewDefaultSwitchProfiles()
	i := &FabricControllerInitializer{
		Client:    kube,
		apiReader: kube,
		key:       ktypes.NamespacedName{Namespace: ns, Name: fcintapi.FabricControllerName},
		version:   "v1.2.3",
		cfg:       &meta.FabricConfig{AllowExtraSwitchProfiles: true},
		profiles:  profiles,
	}

	require.NoError(t, i.Start(t.Context()))

	for _, name := range []string{"old", "current", "flaky"} {
		sw := &wiringapi.Switch{}
		require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: name}, sw))

		want := sw.DeepCopy()
		want.Default()
		require.True(t, defaultedEqual(sw, want), "switch %s should carry the current defaults", name)
		require.Equal(t, wiringapi.DefaultFabric, sw.Spec.Topology.Fabric)
	}

	require.Zero(t, updates["current"], "an object with the current defaults isn't written")
	require.Equal(t, 1, updates["old"])
	require.Equal(t, 1, updates["rejected"], "a rejected object isn't retried")
	require.Equal(t, refreshMaxAttempts, updates["stale"])
	require.Equal(t, 3, updates["flaky"], "a failed write is retried by the next pass")

	fc := &fcintapi.FabricController{}
	require.NoError(t, kube.Get(t.Context(), i.key, fc))
	require.Equal(t, "v1.2.3", fc.Status.InitializedVersion)
	require.True(t, kmeta.IsStatusConditionTrue(fc.Status.Conditions, fcintapi.ConditionInitialized))
	require.True(t, kmeta.IsStatusConditionFalse(fc.Status.Conditions, fcintapi.ConditionRefreshing))
	require.Equal(t, "v1.2.3", fc.Status.Refresh.Version)
	require.False(t, fc.Status.Refresh.FinishedAt.IsZero())
	require.Greater(t, fc.Status.Refresh.Passes, 1)

	require.Equal(t, fcintapi.FabricControllerRefreshKind{
		Kind:     "Switch",
		Total:    5,
		Updated:  2,
		Rejected: 1,
		Stale:    1,
		// none of the switches has an ASN or IPs
		Invalid: 5,
	}, refreshKindStatus(t, fc, "Switch"))
}

func refreshKindStatus(t *testing.T, fc *fcintapi.FabricController, kind string) fcintapi.FabricControllerRefreshKind {
	t.Helper()

	for _, k := range fc.Status.Refresh.Kinds {
		if k.Kind == kind {
			return k
		}
	}
	require.Fail(t, "no refresh status", "kind %s", kind)

	return fcintapi.FabricControllerRefreshKind{}
}

// a stored object that fails the current validation is reported and still gets the current defaults
func TestFabricControllerRefreshInvalid(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, fcintapi.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	// the fabric label is an outdated default, it's removed by the refresh
	server := func(name string) *wiringapi.Server {
		return &wiringapi.Server{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
			Name:      name,
			Labels:    map[string]string{wiringapi.LabelName("outdated"): "true"},
		}}
	}
	tooLong := strings.Repeat("s", meta.MaxNameLength+1)

	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&wiringapi.Fabric{ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric}},
			server("server-1"),
			server(tooLong),
		).
		WithStatusSubresource(&fcintapi.FabricController{}).
		Build()

	i := &FabricControllerInitializer{
		Client:    kube,
		apiReader: kube,
		key:       ktypes.NamespacedName{Namespace: "fab", Name: fcintapi.FabricControllerName},
		version:   "v1.2.3",
		cfg:       &meta.FabricConfig{AllowExtraSwitchProfiles: true},
		profiles:  switchprofile.NewDefaultSwitchProfiles(),
	}
	require.NoError(t, i.Start(t.Context()))

	for _, name := range []string{"server-1", tooLong} {
		srv := &wiringapi.Server{}
		require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: name}, srv))
		require.Empty(t, srv.Labels, "server %s should carry the current defaults", name)
	}

	fc := &fcintapi.FabricController{}
	require.NoError(t, kube.Get(t.Context(), i.key, fc))
	require.Equal(t, "v1.2.3", fc.Status.InitializedVersion)
	require.Equal(t, fcintapi.FabricControllerRefreshKind{
		Kind:    "Server",
		Total:   2,
		Updated: 2,
		Invalid: 1,
	}, refreshKindStatus(t, fc, "Server"))
}
