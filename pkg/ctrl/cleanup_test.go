// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	gwintapi "go.githedgehog.com/fabric/api/gwint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// testFinalizer keeps a test object around with a deletion timestamp, the fake client deletes it right away otherwise
const testFinalizer = "test/hold"

func objKey(name string) kclient.ObjectKey {
	return kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}
}

func reconcileReq(name string) kctrl.Request {
	return kctrl.Request{NamespacedName: objKey(name)}
}

// requireOwnedBy checks that obj is garbage collected with the owner without blocking its deletion
func requireOwnedBy(t *testing.T, kube kclient.Client, obj kclient.Object, name, ownerKind, ownerName string) {
	t.Helper()

	require.NoError(t, kube.Get(t.Context(), objKey(name), obj))
	refs := obj.GetOwnerReferences()
	require.Len(t, refs, 1, "%T %s", obj, name)
	require.Equal(t, ownerKind, refs[0].Kind)
	require.Equal(t, ownerName, refs[0].Name)
	require.True(t, *refs[0].Controller)
	require.False(t, *refs[0].BlockOwnerDeletion)
}

func requireNotFound(t *testing.T, kube kclient.Client, obj kclient.Object, ns, name string) {
	t.Helper()

	err := kube.Get(t.Context(), kclient.ObjectKey{Name: name, Namespace: ns}, obj)
	require.True(t, kapierrors.IsNotFound(err), "%T %s: %v", obj, name, err)
}

// markDeleted gives the object a deletion timestamp, kept by a test finalizer
func markDeleted(t *testing.T, kube kclient.Client, obj kclient.Object) {
	t.Helper()

	obj.SetFinalizers(append(obj.GetFinalizers(), testFinalizer))
	require.NoError(t, kube.Update(t.Context(), obj))
	require.NoError(t, kube.Delete(t.Context(), obj))
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj))
	require.NotNil(t, obj.GetDeletionTimestamp())
}

func TestGatewayCleanup(t *testing.T) {
	kube := gatewayTestKube(t)
	r := &GatewayReconciler{Client: kube, cfg: &meta.FabricConfig{GatewayNamespace: "fab"}, lock: unlockedLock()}

	_, err := r.Reconcile(t.Context(), reconcileReq("gw-1"))
	require.NoError(t, err)

	gw := &gwapi.Gateway{}
	require.NoError(t, kube.Get(t.Context(), objKey("gw-1"), gw))
	require.Contains(t, gw.Finalizers, CleanupFinalizer)

	// garbage collected with the gateway
	requireOwnedBy(t, kube, &gwintapi.GatewayAgent{}, "gw-1", "Gateway", "gw-1")
	requireOwnedBy(t, kube, &rbacv1.Role{}, entityName("gw-1"), "Gateway", "gw-1")
	requireOwnedBy(t, kube, &rbacv1.RoleBinding{}, entityName("gw-1"), "Gateway", "gw-1")

	// in the gateway namespace, cleaned up by the reconciler and labeled so that their deletion reconciles the gateway
	for _, obj := range []kclient.Object{
		&corev1.ServiceAccount{ObjectMeta: kmetav1.ObjectMeta{Namespace: "fab", Name: entityName("gw-1")}},
		&appv1.DaemonSet{ObjectMeta: kmetav1.ObjectMeta{Namespace: "fab", Name: entityName("gw-1", "dataplane")}},
		&appv1.DaemonSet{ObjectMeta: kmetav1.ObjectMeta{Namespace: "fab", Name: entityName("gw-1", "frr")}},
	} {
		require.NoError(t, kube.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj))
		require.Equal(t, "gw-1", obj.GetLabels()[LabelGateway], "%T %s", obj, obj.GetName())

		reqs := r.enqueueForDeployed(t.Context(), obj)
		require.Equal(t, []kctrl.Request{reconcileReq("gw-1")}, reqs)
	}
	// the selector is immutable, so the label is only on the daemonset itself
	ds := &appv1.DaemonSet{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Namespace: "fab", Name: entityName("gw-1", "frr")}, ds))
	require.NotContains(t, ds.Spec.Selector.MatchLabels, LabelGateway)
	require.Empty(t, r.enqueueForDeployed(t.Context(), &appv1.DaemonSet{}))

	require.NoError(t, kube.Delete(t.Context(), gw))

	// a gateway being deleted is no longer a member of its groups
	gw2 := &gwapi.Gateway{}
	require.NoError(t, kube.Get(t.Context(), objKey("gw-2"), gw2))
	ag, err := BuildGatewayAgent(t.Context(), kube, r.cfg, gw2)
	require.NoError(t, err)
	require.Len(t, ag.Spec.Groups["g1"].Members, 1)
	require.Equal(t, "gw-2", ag.Spec.Groups["g1"].Members[0].Name)

	_, err = r.Reconcile(t.Context(), reconcileReq("gw-1"))
	require.NoError(t, err)

	requireNotFound(t, kube, &gwapi.Gateway{}, kmetav1.NamespaceDefault, "gw-1")
	requireNotFound(t, kube, &corev1.ServiceAccount{}, "fab", entityName("gw-1"))
	requireNotFound(t, kube, &appv1.DaemonSet{}, "fab", entityName("gw-1", "dataplane"))
	requireNotFound(t, kube, &appv1.DaemonSet{}, "fab", entityName("gw-1", "frr"))
}

func TestOnlyDeletes(t *testing.T) {
	obj := &agentapi.Agent{}

	require.True(t, onlyDeletes.Delete(event.DeleteEvent{Object: obj}))
	require.False(t, onlyDeletes.Create(event.CreateEvent{Object: obj}))
	require.False(t, onlyDeletes.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: obj}))
	require.False(t, onlyDeletes.Generic(event.GenericEvent{Object: obj}))
}

func TestGatewayWebhookTerminating(t *testing.T) {
	// no reader: nothing may be looked up for a gateway that's only getting its finalizer removed
	w := &GatewayWebhook{}

	gw := &gwapi.Gateway{ObjectMeta: kmetav1.ObjectMeta{Name: "gw-1", DeletionTimestamp: &kmetav1.Time{}}}
	_, err := w.ValidateUpdate(t.Context(), gw, gw)
	require.NoError(t, err)
}

func TestGatewayWebhookMetadataOnly(t *testing.T) {
	// gateways that no longer validate, here with the gateway support disabled, still get their finalizer added
	w := &GatewayWebhook{cfg: &meta.FabricConfig{EnableGateway: false}}

	oldGw := &gwapi.Gateway{
		ObjectMeta: kmetav1.ObjectMeta{Name: "gw-1"},
		Spec:       gwapi.GatewaySpec{Workers: 4},
	}
	newGw := oldGw.DeepCopy()
	newGw.Finalizers = []string{CleanupFinalizer}
	newGw.Labels = map[string]string{"some": "label"}

	_, err := w.ValidateUpdate(t.Context(), oldGw, newGw)
	require.NoError(t, err)

	// a spec change is still validated
	newGw.Spec.Workers = 8
	_, err = w.ValidateUpdate(t.Context(), oldGw, newGw)
	require.ErrorContains(t, err, "gateway support is not enabled")
}

func TestAgentCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&wiringapi.Switch{ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault}},
		&wiringapi.Switch{ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-2", Namespace: kmetav1.NamespaceDefault}},
	).Build()
	r := &AgentReconciler{Client: kube, cfg: &meta.FabricConfig{}, lock: unlockedLock()}

	sw := &wiringapi.Switch{}
	require.NoError(t, kube.Get(t.Context(), objKey("leaf-1"), sw))

	// stops at the token secret, which the fake client never fills
	res, err := r.prepareAgentInfra(t.Context(), sw)
	require.NoError(t, err)
	require.NotNil(t, res)

	saName := AgentServiceAccount("leaf-1")
	requireOwnedBy(t, kube, &corev1.ServiceAccount{}, saName, "Switch", "leaf-1")
	requireOwnedBy(t, kube, &rbacv1.Role{}, saName, "Switch", "leaf-1")
	requireOwnedBy(t, kube, &rbacv1.RoleBinding{}, saName, "Switch", "leaf-1")
	requireOwnedBy(t, kube, &corev1.Secret{}, saName+"-satoken", "Switch", "leaf-1")

	// nothing is produced for a switch being deleted
	require.NoError(t, kube.Get(t.Context(), objKey("leaf-2"), sw))
	markDeleted(t, kube, sw)
	_, err = r.Reconcile(t.Context(), reconcileReq("leaf-2"))
	require.NoError(t, err)
	requireNotFound(t, kube, &corev1.ServiceAccount{}, kmetav1.NamespaceDefault, AgentServiceAccount("leaf-2"))
	requireNotFound(t, kube, &agentapi.Agent{}, kmetav1.NamespaceDefault, "leaf-2")
}

func TestVPCCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, dhcpapi.AddToScheme(scheme))

	vpc := func(name string) *vpcapi.VPC {
		return &vpcapi.VPC{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec: vpcapi.VPCSpec{Subnets: map[string]*vpcapi.VPCSubnet{
				"subnet-1": {
					Subnet: "10.0.1.0/24",
					VLAN:   1001,
					DHCP:   vpcapi.VPCDHCP{Enable: true, Range: &vpcapi.VPCDHCPRange{Start: "10.0.1.10", End: "10.0.1.99"}},
				},
			}},
		}
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vpc("vpc-1"), vpc("vpc-2")).Build()
	cfg := &meta.FabricConfig{}
	r := &VPCReconciler{Client: kube, cfg: cfg, libr: librarian.NewManager(cfg), lock: unlockedLock()}

	_, err := r.Reconcile(t.Context(), reconcileReq("vpc-1"))
	require.NoError(t, err)
	requireOwnedBy(t, kube, &dhcpapi.DHCPSubnet{}, "vpc-1--subnet-1", "VPC", "vpc-1")

	// disabling DHCP on a subnet that still exists drops its DHCPSubnet
	v := &vpcapi.VPC{}
	require.NoError(t, kube.Get(t.Context(), objKey("vpc-1"), v))
	v.Spec.Subnets["subnet-1"].DHCP.Enable = false
	require.NoError(t, kube.Update(t.Context(), v))
	_, err = r.Reconcile(t.Context(), reconcileReq("vpc-1"))
	require.NoError(t, err)
	requireNotFound(t, kube, &dhcpapi.DHCPSubnet{}, kmetav1.NamespaceDefault, "vpc-1--subnet-1")

	// nothing is produced or allocated for a VPC being deleted
	catalog := func() *agentapi.Catalog {
		cat := &agentapi.Catalog{}
		require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: librarian.CatVNIs, Namespace: librarian.Namespace}, cat))

		return cat
	}
	cat := catalog()
	require.NoError(t, kube.Get(t.Context(), objKey("vpc-2"), v))
	markDeleted(t, kube, v)
	_, err = r.Reconcile(t.Context(), reconcileReq("vpc-2"))
	require.NoError(t, err)
	requireNotFound(t, kube, &dhcpapi.DHCPSubnet{}, kmetav1.NamespaceDefault, "vpc-2--subnet-1")
	require.Equal(t, cat.ResourceVersion, catalog().ResourceVersion)
}

func TestGwVPCSyncOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&vpcapi.VPC{ObjectMeta: kmetav1.ObjectMeta{Name: "vpc-1", Namespace: kmetav1.NamespaceDefault}},
		&vpcapi.External{ObjectMeta: kmetav1.ObjectMeta{Name: "ext-1", Namespace: kmetav1.NamespaceDefault}},
	).Build()
	cfg := &meta.FabricConfig{}
	libr := librarian.NewManager(cfg)

	_, err := (&GwVPCSync{Client: kube, cfg: cfg, libr: libr, lock: unlockedLock()}).Reconcile(t.Context(), reconcileReq("vpc-1"))
	require.NoError(t, err)
	requireOwnedBy(t, kube, &gwapi.VPCInfo{}, "vpc-1", "VPC", "vpc-1")

	_, err = (&GwExternalSync{Client: kube, cfg: cfg, libr: libr, lock: unlockedLock()}).Reconcile(t.Context(), reconcileReq("ext-1"))
	require.NoError(t, err)
	requireOwnedBy(t, kube, &gwapi.VPCInfo{}, vpcapi.VPCInfoExtPrefix+"ext-1", "External", "ext-1")
}

func TestSwitchGroupDeleteInUse(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&wiringapi.Switch{
			ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
			Spec:       wiringapi.SwitchSpec{Groups: []string{"mclag-1"}, Redundancy: wiringapi.SwitchRedundancy{Group: "eslag-1", Type: meta.RedundancyTypeESLAG}},
		},
	).Build()
	w := &SwitchGroupWebhook{KubeClient: kube}

	group := func(name string) *wiringapi.SwitchGroup {
		return &wiringapi.SwitchGroup{ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}}
	}

	_, err := w.ValidateDelete(t.Context(), group("eslag-1"))
	require.ErrorContains(t, err, "switch group is used by switch leaf-1")
	_, err = w.ValidateDelete(t.Context(), group("mclag-1"))
	require.ErrorContains(t, err, "switch group is used by switch leaf-1")
	_, err = w.ValidateDelete(t.Context(), group("unused"))
	require.NoError(t, err)
}

func TestConnectionTerminating(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	conn := &wiringapi.Connection{
		ObjectMeta: kmetav1.ObjectMeta{Name: "conn-1", Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.ConnectionSpec{ESLAG: &wiringapi.ConnESLAG{}},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(conn).Build()
	r := &ConnectionReconciler{Client: kube, libr: librarian.NewManager(&meta.FabricConfig{}), lock: unlockedLock()}

	markDeleted(t, kube, conn)
	_, err := r.Reconcile(t.Context(), reconcileReq("conn-1"))
	require.NoError(t, err)

	// nothing allocated, so the catalog was never even created
	requireNotFound(t, kube, &agentapi.Catalog{}, librarian.Namespace, librarian.CatConns)
}
