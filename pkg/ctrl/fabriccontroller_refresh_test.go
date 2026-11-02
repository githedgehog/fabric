// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"slices"
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
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestDefaultedEqual(t *testing.T) {
	sw := &wiringapi.Switch{
		Namespace: kmetav1.NamespaceDefault, Name: "leaf-01",
		Spec: wiringapi.SwitchSpec{Profile: "vs"},
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
			Namespace: kmetav1.NamespaceDefault, Name: name,
			Spec: wiringapi.SwitchSpec{Profile: "vs"},
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
			&wiringapi.Fabric{
				Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric,
				Spec: wiringapi.DefaultFabricSpec(initFabricConfig()),
			},
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
		cfg:       initFabricConfig(),
		profiles:  profiles,
	}

	res, err := reconcileInit(t, i)
	require.NoError(t, err, "objects the refresh couldn't fix aren't retried with backoff")
	require.Equal(t, kctrl.Result{}, res, "nor periodically, only on a spec change")

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
	require.Empty(t, fc.Status.InitializedVersion, "objects the refresh couldn't fix fail the initialization")
	require.False(t, fc.IsReady("v1.2.3"))
	ready := kmeta.FindStatusCondition(fc.Status.Conditions, fcintapi.ConditionReady)
	require.Equal(t, fcintapi.ReasonFailed, ready.Reason)
	require.Equal(t, "5 invalid, 1 rejected, 1 stale", ready.Message)
	require.Empty(t, fc.Status.Error)
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

	reasons := map[string][]string{}
	for _, p := range fc.Status.Problems {
		require.Equal(t, "Switch", p.Kind)
		require.Equal(t, kmetav1.NamespaceDefault, p.Namespace)
		require.NotEmpty(t, p.Message)
		reasons[p.Name] = append(reasons[p.Name], p.Reason)
	}
	require.Equal(t, map[string][]string{
		"current":  {fcintapi.ProblemInvalid},
		"flaky":    {fcintapi.ProblemInvalid},
		"old":      {fcintapi.ProblemInvalid},
		"rejected": {fcintapi.ProblemInvalid, fcintapi.ProblemRejected},
		"stale":    {fcintapi.ProblemInvalid, fcintapi.ProblemStale},
	}, reasons)
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

// stored objects that fail the current validation still get the current defaults, but fail the initialization until
// they're fixed, only the first ones are listed
func TestFabricControllerRefreshInvalid(t *testing.T) {
	// the label is an outdated default, it's removed by the refresh
	server := func(name string) *wiringapi.Server {
		return &wiringapi.Server{
			Namespace: kmetav1.NamespaceDefault,
			Name:      name,
			Labels:    map[string]string{wiringapi.LabelName("outdated"): "true"},
		}
	}

	invalid := make([]string, 0, refreshMaxProblems+1)
	for idx := range refreshMaxProblems + 1 {
		invalid = append(invalid, fmt.Sprintf("%s-%02d", strings.Repeat("s", meta.MaxNameLength), idx))
	}

	objs := make([]kclient.Object, 0, len(invalid)+2)
	objs = append(objs,
		&wiringapi.Fabric{
			Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric,
			Spec: wiringapi.DefaultFabricSpec(initFabricConfig()),
		},
		server("server-1"),
	)
	for _, name := range invalid {
		objs = append(objs, server(name))
	}

	kube := fake.NewClientBuilder().
		WithScheme(initScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&fcintapi.FabricController{}).
		Build()

	i := &FabricControllerInitializer{
		Client:    kube,
		apiReader: kube,
		key:       ktypes.NamespacedName{Namespace: "fab", Name: fcintapi.FabricControllerName},
		version:   "v1.2.3",
		cfg:       initFabricConfig(),
		profiles:  switchprofile.NewDefaultSwitchProfiles(),
	}
	_, err := reconcileInit(t, i)
	require.NoError(t, err)

	for _, name := range append([]string{"server-1"}, invalid...) {
		srv := &wiringapi.Server{}
		require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: name}, srv))
		require.Empty(t, srv.Labels, "server %s should carry the current defaults", name)
	}

	fc := &fcintapi.FabricController{}
	require.NoError(t, kube.Get(t.Context(), i.key, fc))
	require.False(t, fc.IsReady("v1.2.3"))
	ready := kmeta.FindStatusCondition(fc.Status.Conditions, fcintapi.ConditionReady)
	require.Equal(t, fcintapi.ReasonFailed, ready.Reason)
	require.Equal(t, fmt.Sprintf("%d invalid", len(invalid)), ready.Message)
	require.Equal(t, fcintapi.FabricControllerRefreshKind{
		Kind:    "Server",
		Total:   len(invalid) + 1,
		Updated: len(invalid) + 1,
		Invalid: len(invalid),
	}, refreshKindStatus(t, fc, "Server"))

	require.Len(t, fc.Status.Problems, refreshMaxProblems)
	require.True(t, slices.IsSortedFunc(fc.Status.Problems, func(a, b fcintapi.FabricControllerProblem) int {
		return strings.Compare(a.Name, b.Name)
	}))
	for _, p := range fc.Status.Problems {
		require.Equal(t, "Server", p.Kind)
		require.Equal(t, fcintapi.ProblemInvalid, p.Reason)
		require.Contains(t, invalid, p.Name)
	}

	// fixed, e.g. in maintenance
	for _, name := range invalid {
		require.NoError(t, kube.Delete(t.Context(), &wiringapi.Server{Namespace: kmetav1.NamespaceDefault, Name: name}))
	}
	_, err = reconcileInit(t, i)
	require.NoError(t, err)

	require.NoError(t, kube.Get(t.Context(), i.key, fc))
	require.True(t, fc.IsReady("v1.2.3"))
	require.Empty(t, fc.Status.Problems)
}
