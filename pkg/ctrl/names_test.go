// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"strings"
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
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	kapivalidation "k8s.io/apimachinery/pkg/api/validation"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmetav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// maxName is a valid name of the given length
func maxName(prefix string, length int) string {
	return prefix + strings.Repeat("x", length-len(prefix))
}

// requireValidObjects checks that the objects listed are accepted by the API server: their names and labels, and
// the labels of the pods of daemonsets, which the fake client doesn't check
func requireValidObjects(t *testing.T, kube kclient.Client, lists ...kclient.ObjectList) {
	t.Helper()

	validLabels := func(what string, labels map[string]string) {
		t.Helper()

		require.Empty(t, kmetav1validation.ValidateLabels(labels, field.NewPath(what)))
	}

	for _, list := range lists {
		require.NoError(t, kube.List(t.Context(), list))
		items, err := kmeta.ExtractList(list)
		require.NoError(t, err)
		require.NotEmpty(t, items, "%T", list)

		for _, item := range items {
			obj, ok := item.(kclient.Object)
			require.True(t, ok, "%T", item)

			require.Empty(t, kapivalidation.NameIsDNSSubdomain(obj.GetName(), false), "%T %s", obj, obj.GetName())
			validLabels("metadata.labels", obj.GetLabels())

			if ds, ok := obj.(*appv1.DaemonSet); ok {
				validLabels("spec.selector.matchLabels", ds.Spec.Selector.MatchLabels)
				validLabels("spec.template.metadata.labels", ds.Spec.Template.Labels)
				validLabels("spec.template.spec.nodeSelector", ds.Spec.Template.Spec.NodeSelector)
			}
		}
	}
}

// TestProducedObjectNames checks that the objects produced for others are valid with the longest names these
// others can have
func TestProducedObjectNames(t *testing.T) {
	t.Run("agent", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, wiringapi.AddToScheme(scheme))
		require.NoError(t, agentapi.AddToScheme(scheme))
		require.NoError(t, corev1.AddToScheme(scheme))
		require.NoError(t, rbacv1.AddToScheme(scheme))

		name := maxName("leaf-", meta.MaxNameLength)
		kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&wiringapi.Switch{ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}},
		).Build()
		r := &AgentReconciler{Client: kube, cfg: &meta.FabricConfig{APIServer: "172.30.0.1:6443"}, lock: unlockedLock()}

		sw := &wiringapi.Switch{}
		require.NoError(t, kube.Get(t.Context(), objKey(name), sw))

		// stops at the token secret, which the fake client never fills
		_, err := r.prepareAgentInfra(t.Context(), sw)
		require.NoError(t, err)

		tokenSecret := &corev1.Secret{}
		require.NoError(t, kube.Get(t.Context(), objKey(AgentServiceAccount(name)+"-satoken"), tokenSecret))
		tokenSecret.Data = map[string][]byte{
			corev1.ServiceAccountRootCAKey:    []byte("ca-data"),
			corev1.ServiceAccountTokenKey:     []byte("token-data"),
			corev1.ServiceAccountNamespaceKey: []byte(kmetav1.NamespaceDefault),
		}
		require.NoError(t, kube.Update(t.Context(), tokenSecret))

		// now gets the kubeconfig secret as well
		_, err = r.prepareAgentInfra(t.Context(), sw)
		require.NoError(t, err)

		requireValidObjects(t, kube, &corev1.ServiceAccountList{}, &rbacv1.RoleList{}, &rbacv1.RoleBindingList{}, &corev1.SecretList{})
	})

	t.Run("gateway", func(t *testing.T) {
		name := maxName("gw-", gwapi.MaxGatewayNameLength)
		gw := &gwapi.Gateway{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec:       gwapi.GatewaySpec{Groups: []gwapi.GatewayGroupMembership{{Name: "g1"}}},
		}
		gw.Default()

		kube := gatewayTestKube(t, gw)
		r := &GatewayReconciler{Client: kube, cfg: &meta.FabricConfig{GatewayNamespace: "fab"}, lock: unlockedLock()}

		_, err := r.Reconcile(t.Context(), reconcileReq(name))
		require.NoError(t, err)

		requireValidObjects(t, kube, &gwintapi.GatewayAgentList{}, &corev1.ServiceAccountList{}, &rbacv1.RoleList{},
			&rbacv1.RoleBindingList{}, &appv1.DaemonSetList{})
	})

	t.Run("vpc", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, vpcapi.AddToScheme(scheme))
		require.NoError(t, gwapi.AddToScheme(scheme))
		require.NoError(t, agentapi.AddToScheme(scheme))
		require.NoError(t, dhcpapi.AddToScheme(scheme))

		// VPC and External names are capped at 11 characters
		vpc := &vpcapi.VPC{
			ObjectMeta: kmetav1.ObjectMeta{Name: maxName("vpc-", 11), Namespace: kmetav1.NamespaceDefault},
			Spec: vpcapi.VPCSpec{Subnets: map[string]*vpcapi.VPCSubnet{
				maxName("subnet-", meta.MaxNameLength): {
					Subnet: "10.0.1.0/24",
					VLAN:   1001,
					DHCP:   vpcapi.VPCDHCP{Enable: true, Range: &vpcapi.VPCDHCPRange{Start: "10.0.1.10", End: "10.0.1.99"}},
				},
			}},
		}
		vpc.Default()
		ext := &vpcapi.External{ObjectMeta: kmetav1.ObjectMeta{Name: maxName("ext-", 11), Namespace: kmetav1.NamespaceDefault}}
		ext.Default()

		kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vpc, ext).Build()
		cfg := &meta.FabricConfig{}
		libr := librarian.NewManager(cfg)

		_, err := (&VPCReconciler{Client: kube, cfg: cfg, libr: libr, lock: unlockedLock()}).Reconcile(t.Context(), reconcileReq(vpc.Name))
		require.NoError(t, err)
		_, err = (&GwVPCSync{Client: kube, cfg: cfg, libr: libr, lock: unlockedLock()}).Reconcile(t.Context(), reconcileReq(vpc.Name))
		require.NoError(t, err)
		_, err = (&GwExternalSync{Client: kube, cfg: cfg, libr: libr, lock: unlockedLock()}).Reconcile(t.Context(), reconcileReq(ext.Name))
		require.NoError(t, err)

		requireValidObjects(t, kube, &dhcpapi.DHCPSubnetList{}, &gwapi.VPCInfoList{})
	})

	t.Run("vpc interconnect", func(t *testing.T) {
		// capped at 11 characters as its External, with a connection name making the longest attachment name
		ic := &vpcapi.VPCInterconnect{
			ObjectMeta: kmetav1.ObjectMeta{Name: maxName("ic-", 11), Namespace: kmetav1.NamespaceDefault},
			Spec: vpcapi.VPCInterconnectSpec{
				Links: []vpcapi.VPCInterconnectLink{{Connection: maxName("leaf-01--", meta.MaxNameLength-11-2-2-4), VLAN: 4094}},
				Local: map[string]vpcapi.VPCInterconnectVPC{maxName("vpc-", 11): {Subnets: []string{"subnet-01"}}},
			},
		}
		ic.Default()
		require.Len(t, vpcapi.VPCInterconnectAttachmentName(ic.Name, ic.Spec.Links[0]), meta.MaxNameLength)

		kube := vpcInterconnectTestKube(t, ic)
		cfg := &meta.FabricConfig{BaseVPCCommunity: "50000:0"}
		r := &VPCInterconnectReconciler{Client: kube, cfg: cfg, libr: librarian.NewManager(cfg), lock: unlockedLock()}
		for range 2 {
			_, err := r.Reconcile(t.Context(), reconcileReq(ic.Name))
			require.NoError(t, err)
		}

		requireValidObjects(t, kube, &vpcapi.ExternalList{}, &vpcapi.ExternalAttachmentList{}, &vpcapi.ExternalPeeringList{})
	})
}
