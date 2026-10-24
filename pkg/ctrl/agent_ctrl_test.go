// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAgentListByConnections(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, vpcapi.AddToScheme(scheme))

	attach := func(name, conn string) *vpcapi.VPCAttachment {
		a := &vpcapi.VPCAttachment{
			Name: name, Namespace: kmetav1.NamespaceDefault,
			Spec: vpcapi.VPCAttachmentSpec{Subnet: "vpc-1/subnet-1", Connection: conn},
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
			Name: name, Namespace: kmetav1.NamespaceDefault,
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
		&wiringapi.Switch{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
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
			Name: name,
			Spec: vpcapi.VPCSpec{Subnets: map[string]*vpcapi.VPCSubnet{"subnet-1": {DHCP: vpcapi.VPCDHCP{RelayVPC: relayVPC}}}},
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
			Name: name,
			Spec: vpcapi.ExternalSpec{Topology: vpcapi.ExternalTopology{Fabric: fabric}},
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

// TestIRBVLANReqsMatchReconcile covers that irbVLANReqs picks the same IRB VLAN requests as Reconcile does for the
// switch's own config, as one it misses for a peer gets released and renumbered by the peer's reconcile (#1649)
func TestIRBVLANReqsMatchReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))

	objMeta := func(name string) kmetav1.ObjectMeta {
		return kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}
	}
	leaf := func(name string) *wiringapi.Switch {
		sw := &wiringapi.Switch{
			ObjectMeta: objMeta(name),
			Spec:       wiringapi.SwitchSpec{Redundancy: wiringapi.SwitchRedundancy{Type: meta.RedundancyTypeESLAG, Group: "eslag-1"}},
		}
		// sets the fabric, domain and group labels
		sw.Default()

		return sw
	}
	// the agent kubeconfig is generated from it, the fake client never fills it
	token := func(sw string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: objMeta(AgentServiceAccount(sw) + "-satoken"),
			Data: map[string][]byte{
				corev1.ServiceAccountRootCAKey:    []byte("ca"),
				corev1.ServiceAccountTokenKey:     []byte("token"),
				corev1.ServiceAccountNamespaceKey: []byte(kmetav1.NamespaceDefault),
			},
		}
	}
	conn := func(name string, spec wiringapi.ConnectionSpec) *wiringapi.Connection {
		c := &wiringapi.Connection{ObjectMeta: objMeta(name), Spec: spec}
		// sets the switch labels the connections are listed by
		c.Default()

		return c
	}
	link := func(server, sw string) wiringapi.ServerToSwitchLink {
		return wiringapi.ServerToSwitchLink{Server: wiringapi.BasePortName{Port: server}, Switch: wiringapi.BasePortName{Port: sw}}
	}
	vpc := func(name, relayVPC string) *vpcapi.VPC {
		return &vpcapi.VPC{ObjectMeta: objMeta(name), Spec: vpcapi.VPCSpec{
			IPv4Namespace: "default",
			VLANNamespace: "default",
			Subnets:       map[string]*vpcapi.VPCSubnet{"subnet-1": {Subnet: "10.0.1.0/24", DHCP: vpcapi.VPCDHCP{RelayVPC: relayVPC}}},
		}}
	}
	attach := func(conn, vpc string) *vpcapi.VPCAttachment {
		a := &vpcapi.VPCAttachment{
			ObjectMeta: objMeta(conn + "--" + vpc),
			Spec:       vpcapi.VPCAttachmentSpec{Connection: conn, Subnet: vpc + "/subnet-1"},
		}
		// sets the connection label the attachments are listed by
		a.Default()

		return a
	}
	peering := func(vpc1, vpc2 string) *vpcapi.VPCPeering {
		return &vpcapi.VPCPeering{
			ObjectMeta: objMeta(vpc1 + "--" + vpc2),
			Spec:       vpcapi.VPCPeeringSpec{Permit: []map[string]vpcapi.VPCPeer{{vpc1: {}, vpc2: {}}}},
		}
	}
	ext := func(name, fabric string) *vpcapi.External {
		return &vpcapi.External{ObjectMeta: objMeta(name), Spec: vpcapi.ExternalSpec{Topology: vpcapi.ExternalTopology{Fabric: fabric}}}
	}
	extAttach := func(conn, ext string) *vpcapi.ExternalAttachment {
		a := &vpcapi.ExternalAttachment{
			ObjectMeta: objMeta(conn + "--" + ext),
			Spec:       vpcapi.ExternalAttachmentSpec{Connection: conn, External: ext},
		}
		// sets the connection label the attachments are listed by
		a.Default()

		return a
	}
	extPeering := func(ext, vpc string) *vpcapi.ExternalPeering {
		return &vpcapi.ExternalPeering{ObjectMeta: objMeta(ext + "--" + vpc), Spec: vpcapi.ExternalPeeringSpec{Permit: vpcapi.ExternalPeeringSpecPermit{
			External: vpcapi.ExternalPeeringSpecExternal{Name: ext},
			VPC:      vpcapi.ExternalPeeringSpecVPC{Name: vpc},
		}}}
	}

	const eslagConn = "server-1--eslag--leaf-1--leaf-2"
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&wiringapi.Fabric{
			ObjectMeta: objMeta(wiringapi.DefaultFabric),
			Spec:       wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {SpineASN: 65100}}},
		},
		leaf("leaf-1"), leaf("leaf-2"), token("leaf-1"), token("leaf-2"),
		&agentapi.Catalog{
			Name: librarian.CatConns, Namespace: librarian.Namespace,
			Spec: agentapi.CatalogSpec{ConnectionIDs: map[string]uint32{eslagConn: 1}},
		},

		// shared by both leaves
		conn(eslagConn, wiringapi.ConnectionSpec{ESLAG: &wiringapi.ConnESLAG{Links: []wiringapi.ServerToSwitchLink{
			link("server-1/enp2s1", "leaf-1/E1/1"), link("server-1/enp2s2", "leaf-2/E1/1"),
		}}}),
		attach(eslagConn, "vpc-shared"),

		// leaf-1 only: a DHCP relay VPC, a peering, the peering of the peer that doesn't count, externals of this and
		// another fabric and the VPC peered with the external
		conn("server-2--unbundled--leaf-1", wiringapi.ConnectionSpec{Unbundled: &wiringapi.ConnUnbundled{Link: link("server-2/enp2s1", "leaf-1/E1/2")}}),
		attach("server-2--unbundled--leaf-1", "vpc-1"),
		peering("vpc-1", "vpc-peer"),
		peering("vpc-peer", "vpc-far"),
		conn("leaf-1--external", wiringapi.ConnectionSpec{External: &wiringapi.ConnExternal{Link: wiringapi.ConnExternalLink{
			Switch: wiringapi.BasePortName{Port: "leaf-1/E1/10"},
		}}}),
		extAttach("leaf-1--external", "ext-1"),
		extAttach("leaf-1--external", "ext-other-fabric"),
		extPeering("ext-1", "vpc-ext"),

		// leaf-2 only: a static external within a VPC and an attachment to a VPC that doesn't exist, with a peering
		conn("server-3--unbundled--leaf-2", wiringapi.ConnectionSpec{Unbundled: &wiringapi.ConnUnbundled{Link: link("server-3/enp2s1", "leaf-2/E1/2")}}),
		attach("server-3--unbundled--leaf-2", "vpc-2"),
		attach("server-3--unbundled--leaf-2", "vpc-gone"),
		peering("vpc-gone", "vpc-far"),
		conn("leaf-2--static-external", wiringapi.ConnectionSpec{StaticExternal: &wiringapi.ConnStaticExternal{
			Link: wiringapi.ConnStaticExternalLink{Switch: wiringapi.ConnStaticExternalLinkSwitch{
				BasePortName: wiringapi.BasePortName{Port: "leaf-2/E1/10"},
				IP:           "172.31.0.1/24",
				NextHop:      "172.31.0.2",
			}},
			WithinVPC: "vpc-static",
		}}),

		vpc("vpc-shared", ""), vpc("vpc-1", "vpc-relay"), vpc("vpc-relay", ""), vpc("vpc-peer", ""), vpc("vpc-far", ""),
		vpc("vpc-ext", ""), vpc("vpc-2", ""), vpc("vpc-static", ""), vpc("vpc-unused", ""),
		ext("ext-1", wiringapi.DefaultFabric), ext("ext-other-fabric", "fabric-2"),
	).Build()

	cfg := &meta.FabricConfig{VPCIRBVLANRanges: []meta.VLANRange{{From: 3000, To: 3099}}}
	r := &AgentReconciler{Client: kube, cfg: cfg, libr: librarian.NewManager(cfg), lock: unlockedLock()}

	// irbVLANReqs for the connections the switch got, from everything there is
	reqs := func(conns map[string]wiringapi.ConnectionSpec) map[string]bool {
		t.Helper()

		attaches, vpcs, peerings := &vpcapi.VPCAttachmentList{}, &vpcapi.VPCList{}, &vpcapi.VPCPeeringList{}
		extAttaches, exts, extPeerings := &vpcapi.ExternalAttachmentList{}, &vpcapi.ExternalList{}, &vpcapi.ExternalPeeringList{}
		for _, list := range []kclient.ObjectList{attaches, vpcs, peerings, extAttaches, exts, extPeerings} {
			require.NoError(t, kube.List(t.Context(), list))
		}

		return irbVLANReqs(wiringapi.DefaultFabric, conns, attaches.Items, vpcs.Items, peerings.Items, extAttaches.Items, exts.Items, extPeerings.Items)
	}
	reconcile := func(name string) *agentapi.Agent {
		t.Helper()

		res, err := r.Reconcile(t.Context(), reconcileReq(name))
		require.NoError(t, err)
		require.Zero(t, res)

		agent := &agentapi.Agent{}
		require.NoError(t, kube.Get(t.Context(), objKey(name), agent))

		return agent
	}
	keys := func(vlans map[string]uint16) map[string]bool {
		res := map[string]bool{}
		for name := range vlans {
			res[name] = true
		}

		return res
	}

	agent1, agent2 := reconcile("leaf-1"), reconcile("leaf-2")

	// the agent gets the IRB VLANs of what Reconcile configures on the switch
	require.Equal(t, map[string]bool{
		"vpc-shared": true, "vpc-1": true, "vpc-relay": true, "vpc-peer": true, "vpc-ext": true, "ext@ext-1": true,
	}, keys(agent1.Spec.Catalog.IRBVLANs))
	require.Equal(t, map[string]bool{"vpc-shared": true, "vpc-2": true, "vpc-static": true}, keys(agent2.Spec.Catalog.IRBVLANs))

	require.Equal(t, keys(agent1.Spec.Catalog.IRBVLANs), reqs(agent1.Spec.Connections))
	require.Equal(t, keys(agent2.Spec.Catalog.IRBVLANs), reqs(agent2.Spec.Connections))

	// the group catalog holds both and reconciling either again keeps the values
	group := keys(agent1.Spec.Catalog.IRBVLANs)
	maps.Copy(group, keys(agent2.Spec.Catalog.IRBVLANs))
	cat := &agentapi.Catalog{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: "redundancy.eslag-1", Namespace: librarian.Namespace}, cat))
	require.Equal(t, group, keys(cat.Spec.IRBVLANs))

	require.Equal(t, agent1.Spec.Catalog.IRBVLANs, reconcile("leaf-1").Spec.Catalog.IRBVLANs)
	require.Equal(t, agent2.Spec.Catalog.IRBVLANs, reconcile("leaf-2").Spec.Catalog.IRBVLANs)
}
