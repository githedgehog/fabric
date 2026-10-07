// Copyright 2026 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func vpcInterconnectTestKube(t *testing.T, objs ...kclient.Object) kclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestVPCInterconnectGenerates(t *testing.T) {
	ic := &vpcapi.VPCInterconnect{
		ObjectMeta: kmetav1.ObjectMeta{Name: "ic-01", Namespace: kmetav1.NamespaceDefault},
		Spec: vpcapi.VPCInterconnectSpec{
			Links: []vpcapi.VPCInterconnectLink{
				{Connection: "leaf-01--external", VLAN: 101, RemoteASN: 64801, BFD: &vpcapi.ExternalAttachmentBFD{}},
				{Connection: "leaf-02--external", VLAN: 101},
			},
			Local: map[string]vpcapi.VPCInterconnectVPC{
				"vpc-01": {Subnets: []string{"subnet-02", "subnet-01"}},
				"vpc-02": {Subnets: []string{"subnet-01"}},
			},
			Remote: vpcapi.VPCInterconnectRemote{Prefixes: []string{"10.1.1.0/24", "0.0.0.0/0"}},
		},
	}
	ic.Default()
	kube := vpcInterconnectTestKube(t, ic)
	cfg := &meta.FabricConfig{BaseVPCCommunity: "50000:0"}
	r := &VPCInterconnectReconciler{Client: kube, cfg: cfg, libr: librarian.NewManager(cfg), lock: unlockedLock()}

	// the External first, without the community, which comes from its VNI
	res, err := r.Reconcile(t.Context(), reconcileReq("ic-01"))
	require.NoError(t, err)
	require.NotZero(t, res.RequeueAfter)
	ext := &vpcapi.External{}
	require.NoError(t, kube.Get(t.Context(), objKey("ic-01"), ext))
	require.Empty(t, ext.Spec.OutboundCommunity)
	attaches := &vpcapi.ExternalAttachmentList{}
	require.NoError(t, kube.List(t.Context(), attaches))
	require.Empty(t, attaches.Items, "no attachment before the External has its community")

	res, err = r.Reconcile(t.Context(), reconcileReq("ic-01"))
	require.NoError(t, err)
	require.Zero(t, res.RequeueAfter)

	requireOwnedBy(t, kube, ext, "ic-01", vpcapi.KindVPCInterconnect, "ic-01")
	cat := &agentapi.Catalog{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Namespace: librarian.Namespace, Name: librarian.CatVNIs}, cat))
	comm, err := librarian.VNICommunity(cfg.BaseVPCCommunity, cat.Spec.VPCVNIs[librarian.ReqForExt("ic-01")])
	require.NoError(t, err)
	require.Equal(t, vpcapi.ExternalSpec{
		Topology:          vpcapi.ExternalTopology{Fabric: "default", Domain: "default"},
		IPv4Namespace:     "default",
		OutboundCommunity: comm,
	}, ext.Spec)

	attach := &vpcapi.ExternalAttachment{}
	requireOwnedBy(t, kube, attach, "ic-01--leaf-01--external--101", vpcapi.KindVPCInterconnect, "ic-01")
	require.Equal(t, vpcapi.ExternalAttachmentSpec{
		Topology:   vpcapi.ExternalAttachmentTopology{Fabric: "default"},
		External:   "ic-01",
		Connection: "leaf-01--external",
		Switch:     vpcapi.ExternalAttachmentSwitch{VLAN: 101},
		Neighbor:   vpcapi.ExternalAttachmentNeighbor{ASN: 64801},
		BFD:        &vpcapi.ExternalAttachmentBFD{},
	}, attach.Spec)
	requireOwnedBy(t, kube, &vpcapi.ExternalAttachment{}, "ic-01--leaf-02--external--101", vpcapi.KindVPCInterconnect, "ic-01")

	peering := &vpcapi.ExternalPeering{}
	requireOwnedBy(t, kube, peering, "ic-01--vpc-01", vpcapi.KindVPCInterconnect, "ic-01")
	require.Equal(t, vpcapi.ExternalPeeringSpec{
		Topology: vpcapi.ExternalPeeringTopology{Fabric: "default"},
		Permit: vpcapi.ExternalPeeringSpecPermit{
			VPC: vpcapi.ExternalPeeringSpecVPC{Name: "vpc-01", Subnets: []string{"subnet-01", "subnet-02"}},
			External: vpcapi.ExternalPeeringSpecExternal{Name: "ic-01", Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
				{Prefix: "0.0.0.0/0"}, {Prefix: "10.1.1.0/24"},
			}},
		},
	}, peering.Spec)
	requireOwnedBy(t, kube, &vpcapi.ExternalPeering{}, "ic-01--vpc-02", vpcapi.KindVPCInterconnect, "ic-01")

	// nothing changed, nothing written
	_, err = r.Reconcile(t.Context(), reconcileReq("ic-01"))
	require.NoError(t, err)
	unchanged := &vpcapi.ExternalPeering{}
	require.NoError(t, kube.Get(t.Context(), objKey("ic-01--vpc-01"), unchanged))
	require.Equal(t, peering.ResourceVersion, unchanged.ResourceVersion)

	// what a removed link or VPC generated goes away, and only that: the webhooks keep user objects off a generated
	// External, but one that got there anyway isn't deleted
	user := &vpcapi.ExternalPeering{
		ObjectMeta: kmetav1.ObjectMeta{Name: "user--vpc-03", Namespace: kmetav1.NamespaceDefault},
		Spec: vpcapi.ExternalPeeringSpec{Permit: vpcapi.ExternalPeeringSpecPermit{
			VPC:      vpcapi.ExternalPeeringSpecVPC{Name: "vpc-03"},
			External: vpcapi.ExternalPeeringSpecExternal{Name: "ic-01"},
		}},
	}
	user.Default()
	require.NoError(t, kube.Create(t.Context(), user))
	require.NoError(t, kube.Get(t.Context(), objKey("ic-01"), ic))
	ic.Spec.Links = ic.Spec.Links[:1]
	delete(ic.Spec.Local, "vpc-02")
	require.NoError(t, kube.Update(t.Context(), ic))
	_, err = r.Reconcile(t.Context(), reconcileReq("ic-01"))
	require.NoError(t, err)
	requireNotFound(t, kube, &vpcapi.ExternalAttachment{}, kmetav1.NamespaceDefault, "ic-01--leaf-02--external--101")
	requireNotFound(t, kube, &vpcapi.ExternalPeering{}, kmetav1.NamespaceDefault, "ic-01--vpc-02")
	require.NoError(t, kube.Get(t.Context(), objKey("ic-01--leaf-01--external--101"), &vpcapi.ExternalAttachment{}))
	require.NoError(t, kube.Get(t.Context(), objKey("user--vpc-03"), &vpcapi.ExternalPeering{}))
}

// a user External of the same name, which the webhooks refuse to create, is left alone
func TestVPCInterconnectUserExternal(t *testing.T) {
	ic := &vpcapi.VPCInterconnect{ObjectMeta: kmetav1.ObjectMeta{Name: "ic-01", Namespace: kmetav1.NamespaceDefault}}
	ic.Default()
	ext := &vpcapi.External{ObjectMeta: kmetav1.ObjectMeta{Name: "ic-01", Namespace: kmetav1.NamespaceDefault}}
	ext.Default()
	kube := vpcInterconnectTestKube(t, ic, ext)
	cfg := &meta.FabricConfig{BaseVPCCommunity: "50000:0"}
	r := &VPCInterconnectReconciler{Client: kube, cfg: cfg, libr: librarian.NewManager(cfg), lock: unlockedLock()}

	_, err := r.Reconcile(t.Context(), reconcileReq("ic-01"))
	require.ErrorContains(t, err, "isn't generated for VPC interconnect ic-01")
	require.NoError(t, kube.Get(t.Context(), objKey("ic-01"), ext))
	require.Empty(t, ext.OwnerReferences)
}
