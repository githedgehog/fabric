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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec:       wiringapi.SwitchSpec{Profile: meta.SwitchProfileVS},
		}
	}

	conn := func(name string, spec wiringapi.ConnectionSpec) *wiringapi.Connection {
		c := &wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec:       spec,
		}
		// populates the switch/server list labels GetLLDPNeighbors selects on
		c.Default()

		return c
	}

	profile := switchprofile.VS.DeepCopy()
	profile.Namespace = kmetav1.NamespaceDefault

	updated := kmetav1.Time{Time: time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)}

	agent := &agentapi.Agent{
		ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
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

	neighbors, err := apiutil.GetLLDPNeighbors(context.Background(), kube, &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
	}, apiutil.LLDPNeighborsOpts{})
	require.NoError(t, err)

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
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec:       wiringapi.SwitchSpec{Profile: meta.SwitchProfileVS},
		}
	}

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

	agent := &agentapi.Agent{
		ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
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

	neighbors, err := apiutil.GetLLDPNeighbors(context.Background(), kube, &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
	}, apiutil.LLDPNeighborsOpts{})
	require.NoError(t, err)

	// the description is reported as is, it's never mapped, and it doesn't accidentally match the expected port
	actual := neighbors["E1/2"].Actual
	require.Len(t, actual, 1)
	require.Equal(t, "Fabric leaf-1/E1/2 spine-1--fabric--leaf-1", actual[0].Port)
	require.NotEqual(t, neighbors["E1/2"].Expected.Port, actual[0].Port)
}
