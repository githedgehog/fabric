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

package apiutil_test

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	gwintapi "go.githedgehog.com/fabric/api/gwint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestGetLLDPNeighbors covers the mapping from the agent reported neighbors to the expected wiring, in particular the
// derived port/MAC handling: VS maps E1/1 to Ethernet0, E1/2 to Ethernet1, E1/3 to Ethernet2 and so on.
func TestGetLLDPNeighbors(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	sw := func(name string) *wiringapi.Switch {
		return &wiringapi.Switch{
			Name: name, Namespace: kmetav1.NamespaceDefault,
			Spec: wiringapi.SwitchSpec{Profile: meta.SwitchProfileVS},
		}
	}

	conn := func(name string, spec wiringapi.ConnectionSpec) *wiringapi.Connection {
		c := &wiringapi.Connection{
			Name: name, Namespace: kmetav1.NamespaceDefault,
			Spec: spec,
		}
		// populates the switch/server list labels GetLLDPNeighbors selects on
		c.Default()

		return c
	}

	profile := switchprofile.VS.DeepCopy()
	profile.Namespace = kmetav1.NamespaceDefault

	updated := kmetav1.Time{Time: time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)}

	agent := &agentapi.Agent{
		Name: "leaf-1", Namespace: kmetav1.NamespaceDefault,
		Status: agentapi.AgentStatus{
			State: agentapi.SwitchState{
				Interfaces: map[string]agentapi.SwitchStateInterface{
					// server with a MAC port ID: the port name is only in the description
					"E1/1": {LLDPNeighbors: []agentapi.SwitchStateLLDPNeighbor{{
						SystemName:      "server-1",
						PortID:          "0a:00:00:00:1a:01",
						PortDescription: "enp2s1",
						MAC:             "0a:00:00:00:1a:01",
						Port:            "enp2s1",
						TTL:             120,
						LastUpdate:      &updated,
					}}},
					// fabric neighbor: the NOS port name has to be mapped to the peer API port name
					"E1/2": {LLDPNeighbors: []agentapi.SwitchStateLLDPNeighbor{{
						SystemName:        "spine-1",
						SystemDescription: "Hedgehog Fabric",
						PortID:            "Ethernet2",
						MAC:               "0a:00:00:00:2a:01",
						Port:              "Ethernet2",
					}}},
					// gateway neighbor
					"E1/3": {LLDPNeighbors: []agentapi.SwitchStateLLDPNeighbor{{
						SystemName: "gw-1",
						PortID:     "enp1s0",
						Port:       "enp1s0",
					}}},
					// older agent that doesn't report the derived port yet
					"E1/4": {LLDPNeighbors: []agentapi.SwitchStateLLDPNeighbor{{
						SystemName: "server-2",
						PortID:     "enp2s2",
					}}},
					// management port neighbors are never reported, even if an older agent collected them
					"M1": {LLDPNeighbors: []agentapi.SwitchStateLLDPNeighbor{{
						SystemName: "leaf-2",
						PortID:     "0a:00:00:00:3a:01",
						Port:       "eth0",
					}}},
				},
			},
		},
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		profile,
		sw("leaf-1"),
		sw("spine-1"),
		agent,
		conn("server-1--unbundled--leaf-1", wiringapi.ConnectionSpec{
			Unbundled: &wiringapi.ConnUnbundled{
				Link: wiringapi.ServerToSwitchLink{
					Server: wiringapi.NewBasePortName("server-1/enp2s1"),
					Switch: wiringapi.NewBasePortName("leaf-1/E1/1"),
				},
			},
		}),
		conn("spine-1--fabric--leaf-1", wiringapi.ConnectionSpec{
			Fabric: &wiringapi.ConnFabric{
				Links: []wiringapi.FabricLink{{
					Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("spine-1/E1/3")},
					Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("leaf-1/E1/2")},
				}},
			},
		}),
		conn("gw-1--gateway--leaf-1", wiringapi.ConnectionSpec{
			Gateway: &wiringapi.ConnGateway{
				Links: []wiringapi.GatewayLink{{
					Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("leaf-1/E1/3")},
					Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.NewBasePortName("gw-1/enp1s0")},
				}},
			},
		}),
	).Build()

	all, err := apiutil.GetLLDPNeighbors(t.Context(), kube, apiutil.SwitchFilter{Names: []string{"leaf-1"}}, apiutil.LLDPNeighborsOpts{})
	require.NoError(t, err)
	require.Len(t, all, 1)
	neighbors := all["leaf-1"]

	// server: derived port is compared to the wiring, so it matches instead of showing a MAC
	server := neighbors["E1/1"]
	require.Equal(t, apiutil.LLDPNeighborTypeServer, server.Type)
	require.Equal(t, apiutil.LLDPNeighbor{Name: "server-1", Port: "enp2s1"}, server.Expected)
	require.Len(t, server.Actual, 1)
	require.Equal(t, "server-1", server.Actual[0].Name)
	require.Equal(t, "enp2s1", server.Actual[0].Port)
	require.Equal(t, "0a:00:00:00:1a:01", server.Actual[0].MAC)
	require.Equal(t, uint16(120), server.Actual[0].TTL)
	require.NotNil(t, server.Actual[0].LastUpdate)
	// the client round trips the object, so only the instant is preserved, not the location
	require.True(t, server.Actual[0].LastUpdate.Time.Equal(updated.Time))

	// fabric: NOS port name mapped to the peer API port name, description filled in from the deployment ID
	fabric := neighbors["E1/2"]
	require.Equal(t, apiutil.LLDPNeighborTypeFabric, fabric.Type)
	require.Equal(t, "spine-1", fabric.Expected.Name)
	require.Equal(t, "E1/3", fabric.Expected.Port)
	require.Equal(t, wiringapi.SwitchLLDPDescription(""), fabric.Expected.Description)
	require.Len(t, fabric.Actual, 1)
	require.Equal(t, "E1/3", fabric.Actual[0].Port)
	require.Equal(t, "0a:00:00:00:2a:01", fabric.Actual[0].MAC)

	gateway := neighbors["E1/3"]
	require.Equal(t, apiutil.LLDPNeighborTypeGateway, gateway.Type)
	require.Equal(t, "gw-1", gateway.Expected.Name)
	require.Equal(t, "enp1s0", gateway.Actual[0].Port)

	// older agent: falls back to the raw port ID
	require.Equal(t, "enp2s2", neighbors["E1/4"].Actual[0].Port)

	require.NotContains(t, neighbors, "M1")

	// nothing in the wiring can provide these, so they stay unset on the expected side
	for port, n := range neighbors {
		require.Empty(t, n.Expected.MAC, port)
		require.Zero(t, n.Expected.TTL, port)
		require.Nil(t, n.Expected.LastUpdate, port)
	}
}

// TestGetLLDPNeighborsServerExpectedSystemName covers the server system name override: the wiring expects whatever the
// server object says it advertises, falling back to the object name.
func TestGetLLDPNeighborsServerExpectedSystemName(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	profile := switchprofile.VS.DeepCopy()
	profile.Namespace = kmetav1.NamespaceDefault

	srv := func(name, expectedSysName string) *wiringapi.Server {
		return &wiringapi.Server{
			Name: name, Namespace: kmetav1.NamespaceDefault,
			Spec: wiringapi.ServerSpec{
				Inspect: wiringapi.ServerInspect{ExpectedSystemName: expectedSysName},
			},
		}
	}

	conn := func(server, swPort string) *wiringapi.Connection {
		c := &wiringapi.Connection{
			Name:      server + "--unbundled--leaf-1",
			Namespace: kmetav1.NamespaceDefault,
			Spec: wiringapi.ConnectionSpec{
				Unbundled: &wiringapi.ConnUnbundled{
					Link: wiringapi.ServerToSwitchLink{
						Server: wiringapi.NewBasePortName(server + "/enp2s1"),
						Switch: wiringapi.NewBasePortName("leaf-1/" + swPort),
					},
				},
			},
		}
		c.Default()

		return c
	}

	neighbor := func(sysName string) agentapi.SwitchStateInterface {
		return agentapi.SwitchStateInterface{
			LLDPNeighbors: []agentapi.SwitchStateLLDPNeighbor{{
				SystemName: sysName,
				PortID:     "0a:00:00:00:1a:01",
				Port:       "enp2s1",
			}},
		}
	}

	agent := &agentapi.Agent{
		Name: "leaf-1", Namespace: kmetav1.NamespaceDefault,
		Status: agentapi.AgentStatus{
			State: agentapi.SwitchState{
				Interfaces: map[string]agentapi.SwitchStateInterface{
					"E1/1": neighbor("hh-server-1"),
					"E1/2": neighbor("server-2"),
					"E1/3": neighbor("server-3"),
					"E1/4": neighbor("hh-server-4.lan"),
				},
			},
		},
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		profile,
		&wiringapi.Switch{
			Name: "leaf-1", Namespace: kmetav1.NamespaceDefault,
			Spec: wiringapi.SwitchSpec{Profile: meta.SwitchProfileVS},
		},
		agent,
		srv("server-1", "hh-server-1"),
		srv("server-2", ""),
		// server-3 has no object at all, which is what the wiring looked like before the field existed
		srv("server-4", "hh-server-4"),
		conn("server-1", "E1/1"),
		conn("server-2", "E1/2"),
		conn("server-3", "E1/3"),
		conn("server-4", "E1/4"),
	).Build()

	all, err := apiutil.GetLLDPNeighbors(t.Context(), kube, apiutil.SwitchFilter{Names: []string{"leaf-1"}},
		apiutil.LLDPNeighborsOpts{IgnoreSuffixes: apiutil.DefaultLLDPIgnoreSuffixes})
	require.NoError(t, err)
	neighbors := all["leaf-1"]

	for _, tt := range []struct {
		port     string
		expected string
	}{
		{port: "E1/1", expected: "hh-server-1"},
		// no override and no object at all both fall back to the object name
		{port: "E1/2", expected: "server-2"},
		{port: "E1/3", expected: "server-3"},
		// the override is what the ignored suffixes are cut down to
		{port: "E1/4", expected: "hh-server-4"},
	} {
		status := neighbors[tt.port]
		require.Equal(t, tt.expected, status.Expected.Name, tt.port)
		require.Len(t, status.Actual, 1, tt.port)
		require.True(t, status.Actual[0].Matches(status.Expected), tt.port)
	}

	require.Equal(t, ".lan", neighbors["E1/4"].Actual[0].IgnoredSuffix)
}

// TestGetLLDPNeighborsFabricMACPortID covers a fabric port whose neighbor reports a MAC port ID: the derived port is
// then the port description, which must not be looked up in the NOS ports mapping as it isn't a NOS port name.
func TestGetLLDPNeighborsFabricMACPortID(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	profile := switchprofile.VS.DeepCopy()
	profile.Namespace = kmetav1.NamespaceDefault

	sw := func(name string) *wiringapi.Switch {
		return &wiringapi.Switch{
			Name: name, Namespace: kmetav1.NamespaceDefault,
			Spec: wiringapi.SwitchSpec{Profile: meta.SwitchProfileVS},
		}
	}

	conn := &wiringapi.Connection{
		Name: "spine-1--fabric--leaf-1", Namespace: kmetav1.NamespaceDefault,
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

	agent := &agentapi.Agent{
		Name: "leaf-1", Namespace: kmetav1.NamespaceDefault,
		Status: agentapi.AgentStatus{
			State: agentapi.SwitchState{
				Interfaces: map[string]agentapi.SwitchStateInterface{
					"E1/2": {LLDPNeighbors: []agentapi.SwitchStateLLDPNeighbor{{
						SystemName:      "spine-1",
						PortID:          "0a:00:00:00:2a:01",
						PortDescription: "Fabric leaf-1/E1/2 spine-1--fabric--leaf-1",
						MAC:             "0a:00:00:00:2a:01",
						Port:            "Fabric leaf-1/E1/2 spine-1--fabric--leaf-1",
					}}},
				},
			},
		},
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(profile, sw("leaf-1"), sw("spine-1"), agent, conn).Build()

	all, err := apiutil.GetLLDPNeighbors(t.Context(), kube, apiutil.SwitchFilter{Names: []string{"leaf-1"}}, apiutil.LLDPNeighborsOpts{})
	require.NoError(t, err)
	neighbors := all["leaf-1"]

	// the description is reported as is, it's never mapped, and it doesn't accidentally match the expected port
	actual := neighbors["E1/2"].Actual
	require.Len(t, actual, 1)
	require.Equal(t, "Fabric leaf-1/E1/2 spine-1--fabric--leaf-1", actual[0].Port)
	require.NotEqual(t, neighbors["E1/2"].Expected.Port, actual[0].Port)
}

// TestGetGatewayLLDPNeighbors covers the gateway side: the switch advertises its NOS port name, which is mapped back to
// the API one (VS: E1/3 is Ethernet2), and a gateway wired to a selected switch gets all of its links.
func TestGetGatewayLLDPNeighbors(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, gwintapi.AddToScheme(scheme))

	sw := func(name string) *wiringapi.Switch {
		return &wiringapi.Switch{
			Name: name, Namespace: kmetav1.NamespaceDefault,
			Spec: wiringapi.SwitchSpec{Profile: meta.SwitchProfileVS},
		}
	}

	gwConn := func(swPort, gwPort string) *wiringapi.Connection {
		swName, _, _ := strings.Cut(swPort, "/")
		gwName, _, _ := strings.Cut(gwPort, "/")
		c := &wiringapi.Connection{
			Name: gwName + "--gateway--" + swName, Namespace: kmetav1.NamespaceDefault,
			Spec: wiringapi.ConnectionSpec{Gateway: &wiringapi.ConnGateway{Links: []wiringapi.GatewayLink{{
				Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName(swPort)},
				Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.NewBasePortName(gwPort)},
			}}}},
		}
		// populates the connection type label the gateway connections are listed by
		c.Default()

		return c
	}

	gwAgent := func(name string, ifaces map[string]gwintapi.GatewayStateInterface) *gwintapi.GatewayAgent {
		return &gwintapi.GatewayAgent{
			Name: name, Namespace: kmetav1.NamespaceDefault,
			Status: gwintapi.GatewayAgentStatus{State: gwintapi.GatewayState{Interfaces: ifaces}},
		}
	}

	profile := switchprofile.VS.DeepCopy()
	profile.Namespace = kmetav1.NamespaceDefault

	missingGets := 0
	kube := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, kube kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			if _, ok := obj.(*wiringapi.Switch); ok && key.Name == "spine-3" {
				missingGets++
			}

			return kube.Get(ctx, key, obj, opts...)
		},
	}).WithObjects(
		profile,
		sw("spine-1"),
		sw("spine-2"),
		gwConn("spine-1/E1/3", "gw-1/port1"),
		gwConn("spine-2/E1/1", "gw-1/port2"),
		gwConn("spine-2/E1/2", "gw-2/port1"),
		gwConn("spine-1/E1/4", "gw-3/port1"),
		// the switch object is gone, which must not fail the gateway's other links
		gwConn("spine-3/E1/1", "gw-1/port4"),
		&agentapi.Agent{
			Name: "spine-1", Namespace: kmetav1.NamespaceDefault,
			Spec: agentapi.AgentSpec{Config: agentapi.AgentSpecConfig{DeploymentID: "1234"}},
		},
		gwAgent("gw-1", map[string]gwintapi.GatewayStateInterface{
			"port1": {LLDPNeighbors: []gwintapi.GatewayStateLLDPNeighbor{{
				SystemName:        "spine-1",
				SystemDescription: "Hedgehog Fabric 1234",
				PortID:            "Ethernet2",
				Port:              "Ethernet2",
				MAC:               "0a:00:00:00:2a:01",
			}}},
			// the wired switch is gone, so its neighbors are reported as advertised
			"port4": {LLDPNeighbors: []gwintapi.GatewayStateLLDPNeighbor{
				{SystemName: "spine-3", PortID: "Ethernet0"},
				{SystemName: "spine-3", PortID: "Ethernet1"},
			}},
			// not wired, still reported
			"port3": {LLDPNeighbors: []gwintapi.GatewayStateLLDPNeighbor{{
				SystemName: "server-1",
				PortID:     "enp2s1",
			}}},
		}),
		gwAgent("gw-2", map[string]gwintapi.GatewayStateInterface{"port1": {}}),
		// a dataplane that doesn't report interfaces yet
		gwAgent("gw-3", nil),
	).Build()

	all, err := apiutil.GetGatewayLLDPNeighbors(t.Context(), kube, apiutil.SwitchFilter{Names: []string{"spine-1"}}, apiutil.LLDPNeighborsOpts{})
	require.NoError(t, err)
	require.Equal(t, []string{"gw-1"}, slices.Sorted(maps.Keys(all)))
	neighbors := all["gw-1"]

	port1 := neighbors["port1"]
	require.Equal(t, apiutil.LLDPNeighborTypeGateway, port1.Type)
	require.Equal(t, "gw-1--gateway--spine-1", port1.ConnectionName)
	require.Equal(t, wiringapi.ConnectionTypeGateway, port1.ConnectionType)
	require.Equal(t, apiutil.LLDPNeighbor{Name: "spine-1", Port: "E1/3", Description: "Hedgehog Fabric 1234"}, port1.Expected)
	require.Len(t, port1.Actual, 1)
	require.Equal(t, "E1/3", port1.Actual[0].Port)
	require.True(t, port1.Actual[0].Matches(port1.Expected))

	// the link to the switch that isn't selected is still the gateway's, its agent is missing so there is no description
	port2 := neighbors["port2"]
	require.Equal(t, apiutil.LLDPNeighbor{Name: "spine-2", Port: "E1/1"}, port2.Expected)
	require.Empty(t, port2.Actual)

	// the port name is left as wired when the switch can't be looked up
	require.Equal(t, apiutil.LLDPNeighbor{Name: "spine-3", Port: "E1/1"}, neighbors["port4"].Expected)
	require.Len(t, neighbors["port4"].Actual, 2)
	require.ElementsMatch(t, []string{"Ethernet0", "Ethernet1"}, []string{neighbors["port4"].Actual[0].Port, neighbors["port4"].Actual[1].Port})
	// once for the wired port name and once for the neighbor port mapping, not per neighbor
	require.Equal(t, 2, missingGets)

	port3 := neighbors["port3"]
	require.Empty(t, port3.Expected.Name)
	require.Equal(t, "enp2s1", port3.Actual[0].Port)

	all, err = apiutil.GetGatewayLLDPNeighbors(t.Context(), kube, apiutil.SwitchFilter{}, apiutil.LLDPNeighborsOpts{})
	require.NoError(t, err)
	require.Equal(t, []string{"gw-1", "gw-2"}, slices.Sorted(maps.Keys(all)))
}
