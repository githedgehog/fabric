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

func TestIRBVLANReqs(t *testing.T) {
	vpc := func(name, relayVPC string) vpcapi.VPC {
		return vpcapi.VPC{
			ObjectMeta: kmetav1.ObjectMeta{Name: name},
			Spec:       vpcapi.VPCSpec{Subnets: map[string]*vpcapi.VPCSubnet{"subnet-1": {DHCP: vpcapi.VPCDHCP{RelayVPC: relayVPC}}}},
		}
	}
	attach := func(conn, subnet string) vpcapi.VPCAttachment {
		return vpcapi.VPCAttachment{Spec: vpcapi.VPCAttachmentSpec{Connection: conn, Subnet: subnet}}
	}
	peering := func(vpc1, vpc2 string) vpcapi.VPCPeering {
		return vpcapi.VPCPeering{Spec: vpcapi.VPCPeeringSpec{Permit: []map[string]vpcapi.VPCPeer{{vpc1: {}, vpc2: {}}}}}
	}
	ext := func(name, fabric string) vpcapi.External {
		return vpcapi.External{
			ObjectMeta: kmetav1.ObjectMeta{Name: name},
			Spec:       vpcapi.ExternalSpec{Topology: vpcapi.ExternalTopology{Fabric: fabric}},
		}
	}
	extAttach := func(conn, ext string) vpcapi.ExternalAttachment {
		return vpcapi.ExternalAttachment{Spec: vpcapi.ExternalAttachmentSpec{Connection: conn, External: ext}}
	}
	extPeering := func(ext, vpc string) vpcapi.ExternalPeering {
		return vpcapi.ExternalPeering{Spec: vpcapi.ExternalPeeringSpec{Permit: vpcapi.ExternalPeeringSpecPermit{
			External: vpcapi.ExternalPeeringSpecExternal{Name: ext},
			VPC:      vpcapi.ExternalPeeringSpecVPC{Name: vpc},
		}}}
	}

	conns := map[string]wiringapi.ConnectionSpec{
		"conn-1":      {},
		"conn-static": {StaticExternal: &wiringapi.ConnStaticExternal{WithinVPC: "vpc-static"}},
	}

	reqs := irbVLANReqs("fabric-1", conns,
		[]vpcapi.VPCAttachment{attach("conn-1", "vpc-1/subnet-1"), attach("conn-1", "vpc-missing/subnet-1"), attach("conn-other", "vpc-other/subnet-1")},
		[]vpcapi.VPC{
			vpc("vpc-1", "vpc-relay"), vpc("vpc-relay", ""), vpc("vpc-static", ""), vpc("vpc-peer", ""), vpc("vpc-far", ""),
			vpc("vpc-ext", ""), vpc("vpc-other", ""), vpc("vpc-other-ext", ""),
		},
		[]vpcapi.VPCPeering{peering("vpc-1", "vpc-peer"), peering("vpc-peer", "vpc-far"), peering("vpc-missing", "vpc-far")},
		[]vpcapi.ExternalAttachment{extAttach("conn-1", "ext-1"), extAttach("conn-1", "ext-other-fabric"), extAttach("conn-other", "ext-other")},
		[]vpcapi.External{ext("ext-1", "fabric-1"), ext("ext-other-fabric", "fabric-2"), ext("ext-other", "fabric-1")},
		[]vpcapi.ExternalPeering{extPeering("ext-1", "vpc-ext"), extPeering("ext-other", "vpc-other-ext")},
	)

	// only the direct peerings of the attached VPCs that exist count, as for the switch's own config
	require.Equal(t, map[string]bool{
		"vpc-1": true, "vpc-relay": true, "vpc-static": true, "vpc-peer": true, "vpc-ext": true, "ext@ext-1": true,
	}, reqs)
}
