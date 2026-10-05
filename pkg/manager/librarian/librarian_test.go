// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package librarian_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureVNIs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	vpc := func(name string, subnets ...string) *vpcapi.VPC {
		v := &vpcapi.VPC{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec:       vpcapi.VPCSpec{Subnets: map[string]*vpcapi.VPCSubnet{}},
		}
		for _, subnet := range subnets {
			v.Spec.Subnets[subnet] = &vpcapi.VPCSubnet{}
		}

		return v
	}
	specs := func(vpcs ...*vpcapi.VPC) map[string]vpcapi.VPCSpec {
		res := map[string]vpcapi.VPCSpec{}
		for _, v := range vpcs {
			res[v.Name] = v.Spec
		}

		return res
	}

	vpc1 := vpc("vpc-1", "subnet-1")
	ext1 := &vpcapi.External{ObjectMeta: kmetav1.ObjectMeta{Name: "ext-1", Namespace: kmetav1.NamespaceDefault}}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		vpc1, ext1,
		// left over from a VPC deleted earlier
		&agentapi.Catalog{
			ObjectMeta: kmetav1.ObjectMeta{Name: librarian.CatVNIs, Namespace: librarian.Namespace},
			Spec:       agentapi.CatalogSpec{VPCVNIs: map[string]uint32{"gone": 1000}},
		},
	).Build()
	libr := librarian.NewManager(&meta.FabricConfig{})

	catalog := func() *agentapi.Catalog {
		t.Helper()

		cat := &agentapi.Catalog{}
		require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Name: librarian.CatVNIs, Namespace: librarian.Namespace}, cat))

		return cat
	}

	update := func(vpcs map[string]vpcapi.VPCSpec, externals map[string]bool) bool {
		t.Helper()

		updated, err := libr.EnsureVNIs(t.Context(), kube, vpcs, externals)
		require.NoError(t, err)

		return updated
	}

	// allocating also releases the VNIs of the deleted VPCs and covers all VPCs and externals
	require.True(t, update(specs(vpc1), nil))
	cat := catalog()
	require.Contains(t, cat.Spec.VPCVNIs, "vpc-1")
	require.Contains(t, cat.Spec.VPCVNIs, librarian.ReqForExt("ext-1"))
	require.NotContains(t, cat.Spec.VPCVNIs, "gone")
	require.Contains(t, cat.Spec.VPCSubnetVNIs["vpc-1"], "subnet-1")

	// everything requested is already allocated, the catalog isn't touched
	require.False(t, update(specs(vpc1), map[string]bool{"ext-1": true}))
	require.False(t, update(nil, nil))
	require.Equal(t, cat.ResourceVersion, catalog().ResourceVersion)

	// a new subnet of an existing VPC gets a VNI, the existing ones keep theirs
	require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Name: "vpc-1", Namespace: kmetav1.NamespaceDefault}, vpc1))
	vpc1.Spec.Subnets["subnet-2"] = &vpcapi.VPCSubnet{}
	require.NoError(t, kube.Update(t.Context(), vpc1))
	require.True(t, update(specs(vpc1), nil))
	vnis := catalog().Spec
	require.Equal(t, cat.Spec.VPCVNIs["vpc-1"], vnis.VPCVNIs["vpc-1"])
	require.Equal(t, cat.Spec.VPCSubnetVNIs["vpc-1"]["subnet-1"], vnis.VPCSubnetVNIs["vpc-1"]["subnet-1"])
	require.Contains(t, vnis.VPCSubnetVNIs["vpc-1"], "subnet-2")

	// a new external gets a VNI
	require.NoError(t, kube.Create(t.Context(), &vpcapi.External{ObjectMeta: kmetav1.ObjectMeta{Name: "ext-2", Namespace: kmetav1.NamespaceDefault}}))
	require.True(t, update(nil, map[string]bool{"ext-2": true}))
	require.Contains(t, catalog().Spec.VPCVNIs, librarian.ReqForExt("ext-2"))

	// getting an allocated VNI doesn't touch the catalog
	cat = catalog()
	vni, err := libr.GetOrEnsureExternalVNI(t.Context(), kube, "ext-1")
	require.NoError(t, err)
	require.Equal(t, cat.Spec.VPCVNIs[librarian.ReqForExt("ext-1")], vni)
	vni, err = libr.GetOrEnsureVPCVNI(t.Context(), kube, vpc1)
	require.NoError(t, err)
	require.Equal(t, cat.Spec.VPCVNIs["vpc-1"], vni)
	require.Equal(t, cat.ResourceVersion, catalog().ResourceVersion)

	// getting a missing one allocates it, incl. the subnets of the VPC
	vpc2 := vpc("vpc-2", "subnet-1")
	require.NoError(t, kube.Create(t.Context(), vpc2))
	vni, err = libr.GetOrEnsureVPCVNI(t.Context(), kube, vpc2)
	require.NoError(t, err)
	vnis = catalog().Spec
	require.Equal(t, vnis.VPCVNIs["vpc-2"], vni)
	require.Contains(t, vnis.VPCSubnetVNIs["vpc-2"], "subnet-1")

	// a VPC that doesn't exist can't get a VNI
	_, err = libr.GetOrEnsureVPCVNI(t.Context(), kube, vpc("vpc-3"))
	require.ErrorContains(t, err, "failed to find VPC VNI for vpc vpc-3")
}

// TestCatalogOwners covers what the switch and redundancy group catalogs are garbage collected with
func TestCatalogOwners(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	objMeta := func(name string) kmetav1.ObjectMeta {
		return kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}
	}
	sw := func(name, group string) *wiringapi.Switch {
		s := &wiringapi.Switch{ObjectMeta: objMeta(name)}
		if group != "" {
			s.Spec.Redundancy = wiringapi.SwitchRedundancy{Type: meta.RedundancyTypeESLAG, Group: group}
		}

		return s
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		sw("leaf-1", ""),
		sw("leaf-2", "eslag-1"),
		// its group was deleted while still in use, which used to be allowed
		sw("leaf-3", "eslag-2"),
		&wiringapi.SwitchGroup{ObjectMeta: objMeta("eslag-1")},
	).Build()
	libr := librarian.NewManager(&meta.FabricConfig{})

	get := func(obj kclient.Object, name string) kclient.Object {
		t.Helper()

		require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Name: name, Namespace: kmetav1.NamespaceDefault}, obj))

		return obj
	}
	owners := func(key string) []string {
		t.Helper()

		res := []string{}
		for _, ref := range get(&agentapi.Catalog{}, key).GetOwnerReferences() {
			// plain owner references, the catalogs aren't reconciled from their owners
			require.Nil(t, ref.Controller, key)
			require.Nil(t, ref.BlockOwnerDeletion, key)
			res = append(res, ref.Kind+"/"+ref.Name)
		}

		return res
	}

	for _, name := range []string{"leaf-1", "leaf-2", "leaf-3"} {
		s := get(&wiringapi.Switch{}, name).(*wiringapi.Switch)
		require.NoError(t, libr.CatalogForRedundancyGroup(t.Context(), kube, &agentapi.CatalogSpec{}, s, nil, nil, nil, nil))
		require.NoError(t, libr.CatalogForSwitch(t.Context(), kube, &agentapi.CatalogSpec{}, s, nil, nil, nil, nil, nil, nil))
	}

	// without a redundancy group both catalogs are the same one, owned by the switch
	require.Equal(t, []string{"Switch/leaf-1"}, owners("switch.leaf-1"))
	require.Equal(t, []string{"Switch/leaf-2"}, owners("switch.leaf-2"))
	require.Equal(t, []string{"SwitchGroup/eslag-1"}, owners("redundancy.eslag-1"))
	require.Empty(t, owners("redundancy.eslag-2"))
}

func TestGetOrEnsureVPCInfoID(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	vpcInfo := func(name string) *gwapi.VPCInfo {
		return &gwapi.VPCInfo{ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}}
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		vpcInfo("vpc-1"),
		// left over from a VPCInfo deleted earlier
		&agentapi.Catalog{
			ObjectMeta: kmetav1.ObjectMeta{Name: librarian.CatVPCInfos, Namespace: librarian.Namespace},
			Spec:       agentapi.CatalogSpec{VPCInfoIDs: map[string]uint32{"gone": 1}},
		},
	).Build()
	libr := librarian.NewManager(&meta.FabricConfig{})

	catalog := func() *agentapi.Catalog {
		t.Helper()

		cat := &agentapi.Catalog{}
		require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Name: librarian.CatVPCInfos, Namespace: librarian.Namespace}, cat))

		return cat
	}

	// allocating also releases the IDs of the deleted VPCInfos
	id, err := libr.GetOrEnsureVPCInfoID(t.Context(), kube, 100, "vpc-1")
	require.NoError(t, err)
	cat := catalog()
	require.Equal(t, map[string]uint32{"vpc-1": id}, cat.Spec.VPCInfoIDs)

	// already allocated, the catalog isn't touched
	again, err := libr.GetOrEnsureVPCInfoID(t.Context(), kube, 100, "vpc-1")
	require.NoError(t, err)
	require.Equal(t, id, again)
	require.Equal(t, cat.ResourceVersion, catalog().ResourceVersion)

	// a new VPCInfo gets its own ID
	require.NoError(t, kube.Create(t.Context(), vpcInfo("vpc-2")))
	id2, err := libr.GetOrEnsureVPCInfoID(t.Context(), kube, 100, "vpc-2")
	require.NoError(t, err)
	require.NotEqual(t, id, id2)

	// a VPCInfo that doesn't exist can't get an ID
	_, err = libr.GetOrEnsureVPCInfoID(t.Context(), kube, 100, "vpc-3")
	require.ErrorContains(t, err, "failed to find VPCInfo ID for vpcInfo vpc-3")
}
