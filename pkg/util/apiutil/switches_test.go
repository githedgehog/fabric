// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package apiutil_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSwitchFilterValidate(t *testing.T) {
	for _, tt := range []struct {
		name   string
		filter apiutil.SwitchFilter
		err    string
	}{
		{name: "empty"},
		{name: "names", filter: apiutil.SwitchFilter{Names: []string{"leaf-1"}}},
		{name: "fabric", filter: apiutil.SwitchFilter{Fabric: "default"}},
		{name: "fabric and domain", filter: apiutil.SwitchFilter{Fabric: "default", Domain: "default"}},
		{name: "names and fabric", filter: apiutil.SwitchFilter{Names: []string{"leaf-1"}, Fabric: "default"}, err: "together with switch names"},
		{name: "names and domain", filter: apiutil.SwitchFilter{Names: []string{"leaf-1"}, Domain: "default"}, err: "together with switch names"},
		{name: "domain only", filter: apiutil.SwitchFilter{Domain: "default"}, err: "together with fabric"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.err)
			}
		})
	}
}

// TestSwitchFilterSelection covers which switches the bulk helpers load for a filter and that the connections loaded
// in bulk end up with the switches they're wired to
func TestSwitchFilterSelection(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	profile := switchprofile.VS.DeepCopy()
	profile.Namespace = kmetav1.NamespaceDefault

	objs := []kclient.Object{
		profile,
		&wiringapi.Fabric{
			ObjectMeta: kmetav1.ObjectMeta{Name: wiringapi.DefaultFabric, Namespace: kmetav1.NamespaceDefault},
			Spec:       wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {}}},
		},
		&wiringapi.Fabric{
			ObjectMeta: kmetav1.ObjectMeta{Name: "backend", Namespace: kmetav1.NamespaceDefault},
			Spec:       wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{"d1": {}, "d2": {}}},
		},
	}

	// switch and agent with the same labels, as the agent controller keeps them
	addSwitch := func(name, fabric, domain string, withAgent bool) {
		sw := &wiringapi.Switch{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.SwitchSpec{
				Profile:  meta.SwitchProfileVS,
				Topology: wiringapi.SwitchTopology{Fabric: fabric, Domains: []string{domain}},
			},
		}
		sw.Default()
		objs = append(objs, sw)

		if withAgent {
			objs = append(objs, &agentapi.Agent{
				ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault, Labels: sw.Labels},
			})
		}
	}

	addSwitch("leaf-1", wiringapi.DefaultFabric, wiringapi.DefaultFabricDomain, true)
	addSwitch("spine-1", wiringapi.DefaultFabric, wiringapi.DefaultFabricDomain, true)
	addSwitch("leaf-2", "backend", "d1", true)
	addSwitch("leaf-3", "backend", "d2", true)

	conn := &wiringapi.Connection{
		ObjectMeta: kmetav1.ObjectMeta{Name: "spine-1--fabric--leaf-1", Namespace: kmetav1.NamespaceDefault},
		Spec: wiringapi.ConnectionSpec{
			Fabric: &wiringapi.ConnFabric{
				Links: []wiringapi.FabricLink{{
					Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("spine-1/E1/3")},
					Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("leaf-1/E1/2")},
				}},
			},
		},
	}
	conn.Default()
	objs = append(objs, conn)

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

	for _, tt := range []struct {
		name     string
		filter   apiutil.SwitchFilter
		switches []string
		err      string
	}{
		{name: "all", switches: []string{"leaf-1", "leaf-2", "leaf-3", "spine-1"}},
		{name: "names", filter: apiutil.SwitchFilter{Names: []string{"leaf-2", "leaf-1", "leaf-2"}}, switches: []string{"leaf-1", "leaf-2"}},
		{name: "default fabric", filter: apiutil.SwitchFilter{Fabric: wiringapi.DefaultFabric}, switches: []string{"leaf-1", "spine-1"}},
		{name: "default fabric and domain", filter: apiutil.SwitchFilter{Fabric: wiringapi.DefaultFabric, Domain: wiringapi.DefaultFabricDomain}, switches: []string{"leaf-1", "spine-1"}},
		{name: "fabric", filter: apiutil.SwitchFilter{Fabric: "backend"}, switches: []string{"leaf-2", "leaf-3"}},
		{name: "fabric and domain", filter: apiutil.SwitchFilter{Fabric: "backend", Domain: "d1"}, switches: []string{"leaf-2"}},
		{name: "unknown domain", filter: apiutil.SwitchFilter{Fabric: "backend", Domain: "d3"}, err: "domain d3 not found in fabric backend, available: d1, d2"},
		{name: "domain of another fabric", filter: apiutil.SwitchFilter{Fabric: wiringapi.DefaultFabric, Domain: "d1"}, err: "domain d1 not found in fabric default, available: default"},
		{name: "unknown fabric", filter: apiutil.SwitchFilter{Fabric: "frontend"}, err: "fabric frontend not found, available: backend, default"},
		{name: "unknown name", filter: apiutil.SwitchFilter{Names: []string{"leaf-4"}}, err: "switch leaf-4 not found"},
		{name: "invalid", filter: apiutil.SwitchFilter{Domain: "d1"}, err: "together with fabric"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			neighbors, err := apiutil.GetLLDPNeighbors(t.Context(), kube, tt.filter, apiutil.LLDPNeighborsOpts{})
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)

				return
			}
			require.NoError(t, err)

			// every selected switch is reported, even without any neighbors
			require.Equal(t, tt.switches, slices.Sorted(maps.Keys(neighbors)))

			if slices.Contains(tt.switches, "leaf-1") {
				require.Equal(t, "spine-1", neighbors["leaf-1"]["E1/2"].Expected.Name)
				require.Equal(t, "E1/3", neighbors["leaf-1"]["E1/2"].Expected.Port)
			}
			if slices.Contains(tt.switches, "spine-1") {
				require.Equal(t, "leaf-1", neighbors["spine-1"]["E1/3"].Expected.Name)
				require.Equal(t, "E1/2", neighbors["spine-1"]["E1/3"].Expected.Port)
			}
			if slices.Contains(tt.switches, "leaf-2") {
				require.Empty(t, neighbors["leaf-2"])
			}
		})
	}
}

func TestSwitchFilterMissingAgent(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	sw := &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.SwitchSpec{Profile: meta.SwitchProfileVS},
	}
	sw.Default()

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sw).Build()

	_, err := apiutil.GetLLDPNeighbors(t.Context(), kube, apiutil.SwitchFilter{}, apiutil.LLDPNeighborsOpts{})
	require.ErrorContains(t, err, "agent leaf-1 not found")

	_, err = apiutil.GetLLDPNeighbors(t.Context(), kube, apiutil.SwitchFilter{Names: []string{"leaf-1"}}, apiutil.LLDPNeighborsOpts{})
	require.ErrorContains(t, err, "agent leaf-1 not found")
}

// TestSwitchFilterNoFabricObject covers a fabric filter without any Fabric objects: there's no fallback to the
// default one, only the unfiltered inspect works
func TestSwitchFilterNoFabricObject(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	sw := &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.SwitchSpec{Profile: meta.SwitchProfileVS},
	}
	sw.Default()

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		sw,
		&agentapi.Agent{ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault, Labels: sw.Labels}},
	).Build()

	neighbors, err := apiutil.GetLLDPNeighbors(t.Context(), kube, apiutil.SwitchFilter{}, apiutil.LLDPNeighborsOpts{})
	require.NoError(t, err)
	require.Equal(t, []string{"leaf-1"}, slices.Sorted(maps.Keys(neighbors)))

	_, err = apiutil.GetLLDPNeighbors(t.Context(), kube, apiutil.SwitchFilter{Fabric: wiringapi.DefaultFabric}, apiutil.LLDPNeighborsOpts{})
	require.ErrorContains(t, err, "fabric default not found, available: none")

	_, err = apiutil.GetLLDPNeighbors(t.Context(), kube,
		apiutil.SwitchFilter{Fabric: wiringapi.DefaultFabric, Domain: wiringapi.DefaultFabricDomain}, apiutil.LLDPNeighborsOpts{})
	require.ErrorContains(t, err, "fabric default not found, available: none")
}
