// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAgentListByConnections(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, vpcapi.AddToScheme(scheme))

	attach := func(name, conn string) *vpcapi.VPCAttachment {
		a := &vpcapi.VPCAttachment{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec:       vpcapi.VPCAttachmentSpec{Subnet: "vpc-1/subnet-1", Connection: conn},
		}
		// sets the connection label the attachments are selected by
		a.Default()

		return a
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		attach("attach-1", "conn-1"),
		attach("attach-2", "conn-2"),
		attach("attach-3", "conn-3"),
	).Build()
	r := &AgentReconciler{Client: kube}

	list := func(conns ...string) ([]string, error) {
		specs := map[string]wiringapi.ConnectionSpec{}
		for _, conn := range conns {
			specs[conn] = wiringapi.ConnectionSpec{}
		}

		attaches := &vpcapi.VPCAttachmentList{}
		if err := r.listByConnections(t.Context(), kmetav1.NamespaceDefault, attaches, specs); err != nil {
			return nil, err
		}

		names := []string{}
		for _, a := range attaches.Items {
			names = append(names, a.Name)
		}
		slices.Sort(names)

		return names, nil
	}

	names, err := list("conn-1", "conn-3", "conn-4")
	require.NoError(t, err)
	require.Equal(t, []string{"attach-1", "attach-3"}, names)

	names, err = list()
	require.NoError(t, err)
	require.Empty(t, names)

	// a name nothing could ever be attached to by label is reported rather than skipped
	_, err = list("conn-1", strings.Repeat("c", 64))
	require.ErrorContains(t, err, "isn't a valid label value")
}

func TestAgentSwitchLookups(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	sw := func(name, group string, redundancy meta.RedundancyType, groups ...string) *wiringapi.Switch {
		s := &wiringapi.Switch{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.SwitchSpec{
				Groups:     groups,
				Redundancy: wiringapi.SwitchRedundancy{Group: group, Type: redundancy},
			},
		}
		// adds the redundancy group to the groups and sets their labels
		s.Default()

		return s
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		sw("leaf-3", "eslag-1", meta.RedundancyTypeESLAG),
		sw("leaf-1", "eslag-1", meta.RedundancyTypeESLAG),
		sw("leaf-2", "eslag-1", meta.RedundancyTypeESLAG),
		sw("leaf-4", "eslag-2", meta.RedundancyTypeESLAG),
		// in the group, but not as its redundancy group
		sw("leaf-5", "", meta.RedundancyTypeNone, "eslag-1"),
		sw("spine-1", "", meta.RedundancyTypeNone),
	).Build()
	r := &AgentReconciler{Client: kube}

	get := func(name string) *wiringapi.Switch {
		s := &wiringapi.Switch{}
		require.NoError(t, kube.Get(t.Context(), objKey(name), s))

		return s
	}

	peers, err := r.redundancyGroupPeers(t.Context(), get("leaf-1"))
	require.NoError(t, err)
	require.Equal(t, []string{"leaf-2", "leaf-3"}, peers)

	peers, err = r.redundancyGroupPeers(t.Context(), get("leaf-4"))
	require.NoError(t, err)
	require.Empty(t, peers)

	peers, err = r.redundancyGroupPeers(t.Context(), get("spine-1"))
	require.NoError(t, err)
	require.Empty(t, peers)

	// a peer of another redundancy type is reported
	leaf2 := get("leaf-2")
	leaf2.Spec.Redundancy.Type = meta.RedundancyTypeMCLAG
	require.NoError(t, kube.Update(t.Context(), leaf2))
	_, err = r.redundancyGroupPeers(t.Context(), get("leaf-1"))
	require.ErrorContains(t, err, "different redundancy types")

	switches, err := r.getSwitches(t.Context(), kmetav1.NamespaceDefault, map[string]bool{"leaf-1": true, "spine-1": true})
	require.NoError(t, err)
	require.Len(t, switches, 2)
	require.Equal(t, "eslag-1", switches["leaf-1"].Spec.Redundancy.Group)

	// a neighbor that doesn't exist is reported rather than left out
	_, err = r.getSwitches(t.Context(), kmetav1.NamespaceDefault, map[string]bool{"leaf-1": true, "leaf-9": true})
	require.ErrorContains(t, err, "getting switch leaf-9")
}

// TestAgentKubeconfigSecret covers the kubeconfig secret the switches get at install: the kubeconfig is stored as is
// and the secret isn't rewritten when nothing changed
func TestAgentKubeconfigSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&wiringapi.Switch{ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault}},
	).Build()
	r := &AgentReconciler{Client: kube, cfg: &meta.FabricConfig{APIServer: "172.30.0.1:6443"}, lock: unlockedLock()}

	sw := &wiringapi.Switch{}
	require.NoError(t, kube.Get(t.Context(), objKey("leaf-1"), sw))

	// waits for the service account token to be filled in
	res, err := r.prepareAgentInfra(t.Context(), sw)
	require.NoError(t, err)
	require.NotNil(t, res)

	tokenSecret := &corev1.Secret{}
	require.NoError(t, kube.Get(t.Context(), objKey(AgentServiceAccount("leaf-1")+"-satoken"), tokenSecret))
	tokenSecret.Data = map[string][]byte{
		corev1.ServiceAccountRootCAKey:    []byte("ca-data"),
		corev1.ServiceAccountTokenKey:     []byte("token-data"),
		corev1.ServiceAccountNamespaceKey: []byte(kmetav1.NamespaceDefault),
	}
	require.NoError(t, kube.Update(t.Context(), tokenSecret))

	res, err = r.prepareAgentInfra(t.Context(), sw)
	require.NoError(t, err)
	require.Nil(t, res)

	expected, err := r.genKubeconfig(tokenSecret)
	require.NoError(t, err)
	require.Contains(t, expected, "token-data")

	secret := &corev1.Secret{}
	require.NoError(t, kube.Get(t.Context(), objKey(AgentKubeconfigSecret("leaf-1")), secret))
	require.Equal(t, expected, string(secret.Data[AgentKubeconfigKey]))

	// nothing changed, nothing written
	_, err = r.prepareAgentInfra(t.Context(), sw)
	require.NoError(t, err)
	again := &corev1.Secret{}
	require.NoError(t, kube.Get(t.Context(), objKey(AgentKubeconfigSecret("leaf-1")), again))
	require.Equal(t, secret.ResourceVersion, again.ResourceVersion)
}
