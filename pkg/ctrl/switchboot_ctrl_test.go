// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/version"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func switchBootTestKube(t *testing.T, objs ...kclient.Object) kclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, fcintapi.AddToScheme(scheme))

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func switchBootTestSwitch(name, profile string, boot wiringapi.SwitchBoot) *wiringapi.Switch {
	return &wiringapi.Switch{
		Name: name, Namespace: kmetav1.NamespaceDefault,
		Spec: wiringapi.SwitchSpec{Profile: profile, Boot: boot},
	}
}

func switchBootTestProfile(name string, nos meta.NOSType) *wiringapi.SwitchProfile {
	return &wiringapi.SwitchProfile{
		Name: name, Namespace: kmetav1.NamespaceDefault,
		Spec: wiringapi.SwitchProfileSpec{NOSType: nos},
	}
}

func TestSwitchBootReconcile(t *testing.T) {
	kube := switchBootTestKube(t,
		switchBootTestProfile("prof-1", meta.NOSTypeSONiCBCMBase),
		switchBootTestProfile("prof-2", meta.NOSTypeCumulusMlx),
		switchBootTestSwitch("leaf-1", "prof-1", wiringapi.SwitchBoot{Serial: " SN-0001 ", MAC: "0C:20:12:FE:01:00"}),
		switchBootTestSwitch("leaf-2", "prof-2", wiringapi.SwitchBoot{}),
		// stored before the MAC validation existed
		switchBootTestSwitch("leaf-3", "prof-2", wiringapi.SwitchBoot{Serial: "SN-0003", MAC: "not-a-mac"}),
	)
	r := &SwitchBootReconciler{Client: kube, lock: unlockedLock()}

	for _, name := range []string{"leaf-1", "leaf-2", "leaf-3"} {
		_, err := r.Reconcile(t.Context(), reconcileReq(name))
		require.NoError(t, err)
	}

	sb := &fcintapi.SwitchBoot{}
	requireOwnedBy(t, kube, sb, "leaf-1", "Switch", "leaf-1")
	require.Equal(t, fcintapi.SwitchBootSpec{
		Serial:       "sn-0001",
		MAC:          "0c:20:12:fe:01:00",
		NOSType:      meta.NOSTypeSONiCBCMBase,
		AgentVersion: version.Version,
	}, sb.Spec)

	// also produced without serial and MAC, it can't be booted but carries the status later
	requireOwnedBy(t, kube, sb, "leaf-2", "Switch", "leaf-2")
	require.Equal(t, fcintapi.SwitchBootSpec{NOSType: meta.NOSTypeCumulusMlx, AgentVersion: version.Version}, sb.Spec)

	// an invalid MAC is left out, it's still found by serial
	requireOwnedBy(t, kube, sb, "leaf-3", "Switch", "leaf-3")
	require.Equal(t, fcintapi.SwitchBootSpec{
		Serial:       "sn-0003",
		NOSType:      meta.NOSTypeCumulusMlx,
		AgentVersion: version.Version,
	}, sb.Spec)

	// a profile change reconciles the switches using it
	sp := &wiringapi.SwitchProfile{}
	require.NoError(t, kube.Get(t.Context(), objKey("prof-1"), sp))
	sp.Spec.NOSType = meta.NOSTypeSONiCCLSPlusBroadcom
	require.NoError(t, kube.Update(t.Context(), sp))
	require.Equal(t, []string{"leaf-1"}, enqueuedNames(t, enqueueSwitchesByProfile(kube, r.lock), sp))

	_, err := r.Reconcile(t.Context(), reconcileReq("leaf-1"))
	require.NoError(t, err)
	require.NoError(t, kube.Get(t.Context(), objKey("leaf-1"), sb))
	require.Equal(t, meta.NOSTypeSONiCCLSPlusBroadcom, sb.Spec.NOSType)

	// a switch change too
	sw := &wiringapi.Switch{}
	require.NoError(t, kube.Get(t.Context(), objKey("leaf-2"), sw))
	sw.Spec.Boot.MAC = "0C:20:12:FE:02:00"
	require.NoError(t, kube.Update(t.Context(), sw))

	_, err = r.Reconcile(t.Context(), reconcileReq("leaf-2"))
	require.NoError(t, err)
	require.NoError(t, kube.Get(t.Context(), objKey("leaf-2"), sb))
	require.Equal(t, "0c:20:12:fe:02:00", sb.Spec.MAC)
}

func TestSwitchBootReconcileErrors(t *testing.T) {
	kube := switchBootTestKube(t,
		switchBootTestSwitch("leaf-1", "missing", wiringapi.SwitchBoot{Serial: "sn-0001"}),
		switchBootTestSwitch("leaf-2", "", wiringapi.SwitchBoot{Serial: "sn-0002"}),
		switchBootTestSwitch("leaf-3", "prof-1", wiringapi.SwitchBoot{Serial: "sn-0003"}),
		switchBootTestProfile("prof-1", meta.NOSTypeSONiCBCMBase),
	)
	r := &SwitchBootReconciler{Client: kube, lock: unlockedLock()}

	_, err := r.Reconcile(t.Context(), reconcileReq("leaf-1"))
	require.ErrorContains(t, err, "getting switch profile missing")
	requireNotFound(t, kube, &fcintapi.SwitchBoot{}, kmetav1.NamespaceDefault, "leaf-1")

	_, err = r.Reconcile(t.Context(), reconcileReq("leaf-2"))
	require.ErrorContains(t, err, "switch has no profile")
	requireNotFound(t, kube, &fcintapi.SwitchBoot{}, kmetav1.NamespaceDefault, "leaf-2")

	// nothing is produced for a switch being deleted
	sw := &wiringapi.Switch{}
	require.NoError(t, kube.Get(t.Context(), objKey("leaf-3"), sw))
	markDeleted(t, kube, sw)
	_, err = r.Reconcile(t.Context(), reconcileReq("leaf-3"))
	require.NoError(t, err)
	requireNotFound(t, kube, &fcintapi.SwitchBoot{}, kmetav1.NamespaceDefault, "leaf-3")

	// nor for one that's gone
	_, err = r.Reconcile(t.Context(), reconcileReq("leaf-4"))
	require.NoError(t, err)
}
