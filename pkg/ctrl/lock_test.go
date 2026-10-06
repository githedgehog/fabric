// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// unlockedLock is the lock of an initialized fabric controller, for tests of what's gated by it
func unlockedLock() *Lock {
	lock := NewLock("system:serviceaccount:fab:fabric-ctrl")
	lock.unlocked.Store(true)

	return lock
}

func TestLockedReconcilers(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	sw := &wiringapi.Switch{ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"}}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sw).Build()

	lock := NewLock("system:serviceaccount:fab:fabric-ctrl")
	cfg := &meta.FabricConfig{}
	libr := librarian.NewManager(cfg)
	req := kctrl.Request{NamespacedName: ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: "any"}}

	// none of them has a client, they'd panic if they got past the lock
	for name, reconcile := range map[string]func() (kctrl.Result, error){
		"agent":       func() (kctrl.Result, error) { return (&AgentReconciler{lock: lock}).Reconcile(t.Context(), req) },
		"vpc":         func() (kctrl.Result, error) { return (&VPCReconciler{lock: lock}).Reconcile(t.Context(), req) },
		"connection":  func() (kctrl.Result, error) { return (&ConnectionReconciler{lock: lock}).Reconcile(t.Context(), req) },
		"gateway":     func() (kctrl.Result, error) { return (&GatewayReconciler{lock: lock}).Reconcile(t.Context(), req) },
		"vpcinfo":     func() (kctrl.Result, error) { return (&VPCInfoReconciler{lock: lock}).Reconcile(t.Context(), req) },
		"gw-vpc-sync": func() (kctrl.Result, error) { return (&GwVPCSync{lock: lock}).Reconcile(t.Context(), req) },
		"gw-ext-sync": func() (kctrl.Result, error) { return (&GwExternalSync{lock: lock}).Reconcile(t.Context(), req) },
		"switch-profiles": func() (kctrl.Result, error) {
			return (&SwitchProfileReconciler{lock: lock, profiles: switchprofile.NewDefaultSwitchProfiles()}).Reconcile(t.Context(), req)
		},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := reconcile()
			require.NoError(t, err)
			require.Equal(t, kctrl.Result{RequeueAfter: lockedRequeueAfter}, res)
		})
	}

	agent := &AgentReconciler{Client: kube, cfg: cfg, libr: libr, lock: lock}
	require.Empty(t, agent.enqueueAllSwitches(t.Context(), sw), "fan-out is dropped while locked")
	require.Empty(t, agent.enqueueByFabric(t.Context(), &wiringapi.Fabric{ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric}}))

	lock.unlocked.Store(true)
	require.Len(t, agent.enqueueAllSwitches(t.Context(), sw), 1, "fan-out is back once unlocked")
}
