// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// gateways in two fabrics, the default one with two domains:
//   - gw-1, gw-2: default/default, group g1
//   - gw-3: default/d2, group g2
//   - gw-4: other/default, group g3
func gatewayTestKube(t *testing.T, extra ...kclient.Object) kclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	objMeta := func(name string) kmetav1.ObjectMeta {
		return kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}
	}

	gw := func(name, fabric, domain, group string) *gwapi.Gateway {
		gw := &gwapi.Gateway{
			ObjectMeta: objMeta(name),
			Spec: gwapi.GatewaySpec{
				Topology: gwapi.GatewayTopology{Fabric: fabric, Domain: domain},
				Groups:   []gwapi.GatewayGroupMembership{{Name: group}},
			},
		}
		gw.Default()

		return gw
	}

	vpcInfo := func(name, fabric string, domains ...string) *gwapi.VPCInfo {
		vpc := &gwapi.VPCInfo{
			ObjectMeta: objMeta(name),
			Spec: gwapi.VPCInfoSpec{
				Topology: gwapi.VPCInfoTopology{Fabric: fabric, Domains: domains},
				VNI:      100,
			},
			Status: gwapi.VPCInfoStatus{InternalID: name},
		}
		vpc.Default()

		return vpc
	}

	peering := func(name, fabric, vpc1, vpc2 string) *gwapi.GatewayPeering {
		p := &gwapi.GatewayPeering{
			ObjectMeta: objMeta(name),
			Spec: gwapi.PeeringSpec{
				Topology: gwapi.GatewayPeeringTopology{Fabric: fabric},
				Peering:  map[string]*gwapi.PeeringEntry{vpc1: {}, vpc2: {}},
			},
		}
		p.Default()

		return p
	}

	objs := []kclient.Object{
		&wiringapi.Fabric{
			ObjectMeta: objMeta(wiringapi.DefaultFabric),
			Spec:       wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {}, "d2": {}}},
		},
		&wiringapi.Fabric{
			ObjectMeta: objMeta("other"),
			Spec:       wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {}}},
		},

		gw("gw-1", "", "", "g1"),
		gw("gw-2", "", "", "g1"),
		gw("gw-3", "", "d2", "g2"),
		gw("gw-4", "other", "", "g3"),

		vpcInfo("vpc-1", "", wiringapi.DefaultFabricDomain),
		vpcInfo("vpc-2", "", wiringapi.DefaultFabricDomain),
		vpcInfo("vpc-3", "", "d2"),
		// in both domains of the default fabric
		vpcInfo("vpc-4", "", wiringapi.DefaultFabricDomain, "d2"),
		vpcInfo("vpc-5", "other", wiringapi.DefaultFabricDomain),

		peering("vpc-1--vpc-2", "", "vpc-1", "vpc-2"),
		peering("vpc-1--vpc-4", "", "vpc-1", "vpc-4"),
		peering("vpc-3--vpc-4", "", "vpc-3", "vpc-4"),
		peering("vpc-5--vpc-6", "other", "vpc-5", "vpc-6"),
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(objs, extra...)...).Build()
}

func TestBuildGatewayAgentScope(t *testing.T) {
	cfg := &meta.FabricConfig{}

	for _, tt := range []struct {
		name     string
		gw       string
		vpcs     []string
		peerings []string
		groups   map[string][]string
	}{
		{
			name:     "default domain",
			gw:       "gw-1",
			vpcs:     []string{"vpc-1", "vpc-2", "vpc-4"},
			peerings: []string{"vpc-1--vpc-2", "vpc-1--vpc-4"},
			groups:   map[string][]string{"g1": {"gw-1", "gw-2"}},
		},
		{
			name:     "another domain",
			gw:       "gw-3",
			vpcs:     []string{"vpc-3", "vpc-4"},
			peerings: []string{"vpc-3--vpc-4"},
			groups:   map[string][]string{"g2": {"gw-3"}},
		},
		{
			// the peering of the other fabric has a VPC that doesn't exist, so it's skipped there too
			name:   "another fabric",
			gw:     "gw-4",
			vpcs:   []string{"vpc-5"},
			groups: map[string][]string{"g3": {"gw-4"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := gatewayTestKube(t)

			gw := &gwapi.Gateway{}
			require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: tt.gw, Namespace: kmetav1.NamespaceDefault}, gw))

			ag, err := BuildGatewayAgent(t.Context(), kube, cfg, gw)
			require.NoError(t, err)

			require.Equal(t, tt.vpcs, slices.Sorted(maps.Keys(ag.Spec.VPCs)))
			require.Equal(t, tt.peerings, slices.Sorted(maps.Keys(ag.Spec.Peerings)))

			groups := map[string][]string{}
			for name, info := range ag.Spec.Groups {
				for _, member := range info.Members {
					groups[name] = append(groups[name], member.Name)
				}
			}
			require.Equal(t, tt.groups, groups)
		})
	}
}

func TestBuildGatewayAgentNotReadyElsewhere(t *testing.T) {
	notReady := &gwapi.VPCInfo{
		ObjectMeta: kmetav1.ObjectMeta{Name: "vpc-6", Namespace: kmetav1.NamespaceDefault},
		Spec: gwapi.VPCInfoSpec{
			Topology: gwapi.VPCInfoTopology{Domains: []string{"d2"}},
			VNI:      100,
		},
	}
	notReady.Default()

	kube := gatewayTestKube(t, notReady)
	cfg := &meta.FabricConfig{}

	gw := &gwapi.Gateway{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: "gw-1", Namespace: kmetav1.NamespaceDefault}, gw))
	_, err := BuildGatewayAgent(t.Context(), kube, cfg, gw)
	require.NoError(t, err, "a VPCInfo of another domain doesn't block the gateway")

	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: "gw-3", Namespace: kmetav1.NamespaceDefault}, gw))
	_, err = BuildGatewayAgent(t.Context(), kube, cfg, gw)
	require.ErrorIs(t, err, ErrRetryLater)
}

func TestGatewayEnqueue(t *testing.T) {
	kube := gatewayTestKube(t)
	r := &GatewayReconciler{Client: kube}

	get := func(obj kclient.Object, name string) kclient.Object {
		require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, obj))

		return obj
	}

	for _, tt := range []struct {
		name    string
		obj     kclient.Object
		enqueue func(context.Context, kclient.Object) []reconcile.Request
		gws     []string
	}{
		{name: "gateway", obj: get(&gwapi.Gateway{}, "gw-1"), enqueue: r.enqueueForGateway, gws: []string{"gw-1", "gw-2"}},
		{name: "peering", obj: get(&gwapi.GatewayPeering{}, "vpc-1--vpc-2"), enqueue: r.enqueueForPeering, gws: []string{"gw-1", "gw-2", "gw-3"}},
		{name: "vpcinfo in a domain", obj: get(&gwapi.VPCInfo{}, "vpc-3"), enqueue: r.enqueueForVPCInfo, gws: []string{"gw-3"}},
		{name: "vpcinfo in two domains", obj: get(&gwapi.VPCInfo{}, "vpc-4"), enqueue: r.enqueueForVPCInfo, gws: []string{"gw-1", "gw-2", "gw-3"}},
		{
			// written before the topology existed, the same as the default one
			name:    "vpcinfo without topology",
			obj:     &gwapi.VPCInfo{ObjectMeta: kmetav1.ObjectMeta{Name: "vpc-7", Namespace: kmetav1.NamespaceDefault}},
			enqueue: r.enqueueForVPCInfo,
			gws:     []string{"gw-1", "gw-2"},
		},
		{name: "fabric", obj: get(&wiringapi.Fabric{}, "other"), enqueue: r.enqueueForFabric, gws: []string{"gw-4"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gws := []string{}
			for _, req := range tt.enqueue(t.Context(), tt.obj) {
				gws = append(gws, req.Name)
			}
			slices.Sort(gws)

			require.Equal(t, tt.gws, gws)
		})
	}
}
