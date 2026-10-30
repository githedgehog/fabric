// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMACFilter(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, dhcpapi.AddToScheme(scheme))

	const mac = "0c:20:12:fe:00:01"

	fabricGen := func(name string, domains ...string) kclient.Object {
		fab := &wiringapi.Fabric{Spec: wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{}}}
		fab.Name, fab.Namespace = name, kmetav1.NamespaceDefault
		for _, domain := range domains {
			fab.Spec.Domains[domain] = wiringapi.FabricDomainSpec{}
		}

		return fab
	}
	// the agent ctrl copies the defaulted switch labels to the agent
	agentGen := func(name, fabricName, domain string) kclient.Object {
		sw := &wiringapi.Switch{Spec: wiringapi.SwitchSpec{Topology: wiringapi.SwitchTopology{Fabric: fabricName, Domains: []string{domain}}}}
		sw.Default()

		ag := &agentapi.Agent{}
		ag.Name, ag.Namespace, ag.Labels = name, kmetav1.NamespaceDefault, sw.Labels
		ag.Status.State.Interfaces = map[string]agentapi.SwitchStateInterface{"E1/1": {MAC: mac}}

		return ag
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		fabricGen("default", "default", "east"),
		fabricGen("backend", "default"),
		agentGen("leaf-1", "default", "default"),
		agentGen("leaf-2", "default", "east"),
		agentGen("leaf-3", "backend", "default"),
	).Build()

	for name, tt := range map[string]struct {
		in    MACIn
		ports []string
		err   string
	}{
		"all":            {in: MACIn{}, ports: []string{"leaf-1/E1/1", "leaf-2/E1/1", "leaf-3/E1/1"}},
		"fabric":         {in: MACIn{Fabric: "default"}, ports: []string{"leaf-1/E1/1", "leaf-2/E1/1"}},
		"domain":         {in: MACIn{Fabric: "default", Domain: "east"}, ports: []string{"leaf-2/E1/1"}},
		"unknown fabric": {in: MACIn{Fabric: "frontend"}, err: "fabric frontend not found"},
		"unknown domain": {in: MACIn{Fabric: "backend", Domain: "east"}, err: "domain east not found in fabric backend"},
		"domain only":    {in: MACIn{Domain: "east"}, err: "domain can only be used together with fabric"},
	} {
		t.Run(name, func(t *testing.T) {
			tt.in.Value = mac
			out, err := MAC(t.Context(), kube, tt.in)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.ports, out.Ports)
		})
	}
}

func TestCompareMACPorts(t *testing.T) {
	// MAC ports are "agentName/ifaceName". Sorting must group by switch
	// (natural order on the agent name) and, within a switch, keep management
	// ports first and order data ports naturally (Ethernet2 before Ethernet10).
	// A plain lexical sort would put leaf-10 before leaf-2 and Ethernet10
	// before Ethernet2.
	got := []string{
		"leaf-10/Ethernet2",
		"leaf-2/Ethernet1",
		"leaf-2/Ethernet10",
		"leaf-2/Ethernet2",
		"leaf-2/M1",
		"leaf-10/Ethernet1",
	}

	slices.SortFunc(got, compareMACPorts)

	require.Equal(t, []string{
		"leaf-2/M1",
		"leaf-2/Ethernet1",
		"leaf-2/Ethernet2",
		"leaf-2/Ethernet10",
		"leaf-10/Ethernet1",
		"leaf-10/Ethernet2",
	}, got)
}
