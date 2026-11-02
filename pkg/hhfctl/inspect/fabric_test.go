// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func fabricScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, fcintapi.AddToScheme(scheme))

	return scheme
}

func testFabricController(ver string, ready bool) *fcintapi.FabricController {
	fc := &fcintapi.FabricController{
		Namespace: "fab", Name: fcintapi.FabricControllerName, Generation: 1,
		Status: fcintapi.FabricControllerStatus{Version: ver, InitializedVersion: ver},
	}
	if ready {
		fc.Status.Conditions = []kmetav1.Condition{{
			Type: fcintapi.ConditionReady, Status: kmetav1.ConditionTrue, ObservedGeneration: 1,
			Reason: fcintapi.ReasonInitialized, Message: "operational",
		}}
	}

	return fc
}

func TestFabricFilter(t *testing.T) {
	scheme := fabricScheme(t)

	meta := func(name string) kmetav1.ObjectMeta {
		return kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}
	}
	// stored switches are always defaulted
	switchGen := func(name, fabricName string) *wiringapi.Switch {
		sw := &wiringapi.Switch{ObjectMeta: meta(name), Spec: wiringapi.SwitchSpec{Topology: wiringapi.SwitchTopology{Fabric: fabricName}, Profile: "vs"}}
		sw.Default()

		return sw
	}
	// created without a fabric, so defaulted to the default one
	leaf01 := switchGen("leaf-01", "")
	// the agent ctrl copies the switch labels to the agent
	agent01 := &agentapi.Agent{ObjectMeta: meta("leaf-01")}
	agent01.Labels = leaf01.Labels

	profileGets, agentGets := 0, 0
	kube := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, kube kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			switch obj.(type) {
			case *wiringapi.SwitchProfile:
				profileGets++
			case *agentapi.Agent:
				agentGets++
			}

			return kube.Get(ctx, key, obj, opts...)
		},
	}).WithObjects(
		testFabricController("v1.2.3", true),
		&wiringapi.SwitchProfile{ObjectMeta: meta("vs")},
		&wiringapi.Fabric{ObjectMeta: meta("default")},
		&wiringapi.Fabric{ObjectMeta: meta("backend")},
		leaf01,
		switchGen("leaf-02", "backend"),
		// leaf-02 has no agent yet
		agent01,
	).Build()

	names := func(out *FabricOut) ([]string, []string) {
		fabrics, switches := []string{}, []string{} //nolint:prealloc
		for _, fab := range out.Fabrics {
			fabrics = append(fabrics, fab.Name)
		}
		for _, sw := range out.Switches {
			switches = append(switches, sw.Name+"@"+sw.Fabric)
		}

		return fabrics, switches
	}

	out, err := Fabric(t.Context(), kube, FabricIn{})
	require.NoError(t, err)
	fabrics, switches := names(out)
	require.Equal(t, []string{"backend", "default"}, fabrics)
	require.Equal(t, []string{"leaf-01@default", "leaf-02@backend"}, switches)
	require.Equal(t, 1, profileGets, "shared switch profile should be fetched once")
	require.Zero(t, agentGets, "agents should be listed, not fetched one by one")
	require.Equal(t, "Ready: 1/2 switches", out.Summary)

	out, err = Fabric(t.Context(), kube, FabricIn{Name: "default"})
	require.NoError(t, err)
	fabrics, switches = names(out)
	require.Equal(t, []string{"default"}, fabrics)
	require.Equal(t, []string{"leaf-01@default"}, switches)
	require.Equal(t, "Ready: 1/1 switches", out.Summary)

	out, err = Fabric(t.Context(), kube, FabricIn{Name: "backend"})
	require.NoError(t, err)
	_, switches = names(out)
	require.Equal(t, []string{"leaf-02@backend"}, switches)
	require.Equal(t, "Ready: 0/1 switches", out.Summary)

	_, err = Fabric(t.Context(), kube, FabricIn{Name: "frontend"})
	require.ErrorContains(t, err, "fabric frontend not found")
}

func TestFabricController(t *testing.T) {
	failed := testFabricController("v1.2.4", false)
	failed.Status.InitializedVersion = "v1.2.3"
	failed.Status.Conditions = []kmetav1.Condition{{
		Type: fcintapi.ConditionReady, Status: kmetav1.ConditionFalse, ObservedGeneration: 1,
		Reason: fcintapi.ReasonFailed, Message: "1 invalid",
	}}
	failed.Status.Problems = []fcintapi.FabricControllerProblem{{
		Kind: "Switch", Namespace: kmetav1.NamespaceDefault, Name: "leaf-01", Reason: fcintapi.ProblemInvalid, Message: "asn is required",
	}}

	retrying := testFabricController("v1.2.4", false)
	retrying.Spec.Maintenance = true
	retrying.Status.Error = "listing VPC: etcd is down"

	for _, tt := range []struct {
		name  string
		objs  []kclient.Object
		want  *FabricOutController
		shown []string
	}{
		{
			name: "ready",
			objs: []kclient.Object{testFabricController("v1.2.3", true)},
			want: &FabricOutController{Ready: true, Version: "v1.2.3", Initialized: "v1.2.3", State: fcintapi.ReasonInitialized, Message: "operational"},
		},
		{
			name:  "missing",
			want:  &FabricOutController{State: "Missing", Message: "not created yet"},
			shown: []string{"Fabric controller isn't ready", "Missing", "not created yet"},
		},
		{
			name: "failed",
			objs: []kclient.Object{failed},
			want: &FabricOutController{
				Version: "v1.2.4", Initialized: "v1.2.3", State: fcintapi.ReasonFailed, Message: "1 invalid",
				Problems: failed.Status.Problems,
			},
			shown: []string{"Fabric controller isn't ready", "v1.2.4", "v1.2.3", "Failed", "1 invalid", "leaf-01", "Invalid", "asn is required"},
		},
		{
			name:  "error in maintenance",
			objs:  []kclient.Object{retrying},
			want:  &FabricOutController{Version: "v1.2.4", Initialized: "v1.2.4", Maintenance: true, Error: "listing VPC: etcd is down"},
			shown: []string{"Fabric controller isn't ready", "true", "Error: listing VPC: etcd is down"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().WithScheme(fabricScheme(t)).WithObjects(tt.objs...).Build()

			out, err := Fabric(t.Context(), kube, FabricIn{})
			require.NoError(t, err)
			require.Equal(t, tt.want, out.Controller)

			text, err := out.MarshalText(FabricIn{}, time.Now())
			require.NoError(t, err)
			if len(tt.shown) == 0 {
				require.NotContains(t, text, "Fabric controller")
			}
			for _, s := range tt.shown {
				require.Contains(t, text, s)
			}
		})
	}

	t.Run("more than one", func(t *testing.T) {
		other := testFabricController("v1.2.3", true)
		other.Namespace = "other"
		kube := fake.NewClientBuilder().WithScheme(fabricScheme(t)).WithObjects(testFabricController("v1.2.3", true), other).Build()

		_, err := Fabric(t.Context(), kube, FabricIn{})
		require.ErrorContains(t, err, "fabric controller found in namespaces")
	})
}
