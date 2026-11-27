// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	gwintapi "go.githedgehog.com/fabric/api/gwint/v1alpha1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// gatewayTestKube is env-1's gateway-1: a DPDK gateway with one port wired to a leaf and a passive DAC in it, plus a
// second spec interface that isn't wired.
func gatewayTestKube(t *testing.T, now time.Time) kclient.Reader {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))
	require.NoError(t, gwintapi.AddToScheme(scheme))

	conn := &wiringapi.Connection{
		Name: "s5248-03--gateway--gateway-1", Namespace: kmetav1.NamespaceDefault,
		Spec: wiringapi.ConnectionSpec{Gateway: &wiringapi.ConnGateway{Links: []wiringapi.GatewayLink{{
			Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("s5248-03/E1/3")},
			Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.NewBasePortName("gateway-1/port1")},
		}}}},
	}
	conn.Default()

	// never valid, it must not break inspecting the other gateways
	invalid := &wiringapi.Connection{
		Name: "s5248-04--gateway--two-gateways", Namespace: kmetav1.NamespaceDefault,
		Labels: map[string]string{wiringapi.LabelConnectionType: wiringapi.ConnectionTypeGateway},
		Spec: wiringapi.ConnectionSpec{Gateway: &wiringapi.ConnGateway{Links: []wiringapi.GatewayLink{
			{
				Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("s5248-04/E1/1")},
				Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.NewBasePortName("gateway-2/port1")},
			},
			{
				Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName("s5248-04/E1/2")},
				Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.NewBasePortName("gateway-3/port1")},
			},
		}}},
	}

	changed := kmetav1.NewTime(now.Add(-2 * time.Hour))
	flaps := uint32(0)

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		conn,
		invalid,
		&gwapi.Gateway{
			Name: "gateway-1", Namespace: kmetav1.NamespaceDefault,
			Spec: gwapi.GatewaySpec{
				Driver: gwapi.GatewayDriverDPDK,
				Groups: []gwapi.GatewayGroupMembership{{Name: "default"}},
				Interfaces: map[string]gwapi.GatewayInterface{
					"port1": {PCI: "0000:05:00.0"},
					"port2": {PCI: "0000:05:00.1"},
				},
			},
		},
		&gwintapi.GatewayAgent{
			Name: "gateway-1", Namespace: kmetav1.NamespaceDefault,
			Status: gwintapi.GatewayAgentStatus{State: gwintapi.GatewayState{
				Dataplane: gwintapi.DataplaneStatus{Version: "v0.39.0"},
				Interfaces: map[string]gwintapi.GatewayStateInterface{
					"port1": {
						OperStatus:    gwintapi.InterfaceOperStatusUp,
						MAC:           "b4:96:91:3c:7a:10",
						LastChange:    &changed,
						Speed:         "10G",
						AutoNegotiate: true,
						LinkFlapCount: &flaps,
						Counters:      &gwintapi.GatewayStateInterfaceCounters{InBitsPerSecond: 2110, OutBitsPerSecond: 1582, InErrors: 1},
						LLDPNeighbors: []gwintapi.GatewayStateLLDPNeighbor{{
							SystemName:      "s5248-03",
							PortID:          "Ethernet2",
							Port:            "Ethernet2",
							PortDescription: "Gateway gateway-1/port1 s5248-03--gateway--gateway-1",
							MAC:             "3c:2c:30:71:1b:04",
							Model:           "S5248F-ON",
						}},
					},
				},
				Transceivers: map[string]gwintapi.GatewayStateTransceiver{
					"port1": {
						Description:   "SFP28 25GBASE-CR-DAC-1.0M",
						CableClass:    "DAC",
						FormFactor:    "SFP28",
						ConnectorType: "COPPER_PIGTAIL_CONNECTOR",
						CableLength:   1,
						SerialNumber:  "CSC230804260009",
						VendorPart:    "SFP-H25G-CU1M",
					},
				},
			}},
		},
	).Build()
}

func TestGatewayPort(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	kube := gatewayTestKube(t, now)

	out, err := Port(t.Context(), kube, PortIn{Port: "gateway-1/port1"})
	require.NoError(t, err)
	require.NotNil(t, out.ConnectionName)
	require.Equal(t, "s5248-03--gateway--gateway-1", *out.ConnectionName)
	require.NotNil(t, out.GatewayInterfaceState)
	require.Equal(t, "10G", out.GatewayInterfaceState.Speed)
	require.NotNil(t, out.GatewayTransceiverState)
	require.Equal(t, "DAC", out.GatewayTransceiverState.CableClass)

	text, err := out.MarshalText(PortIn{}, now)
	require.NoError(t, err)
	for _, want := range []string{"Used in Connection s5248-03--gateway--gateway-1:", "10G/auto", "2 hours ago", "↓ 2,110", "Ethernet2", "S5248F-ON", "CableClass: DAC", "CableLength: 1m"} {
		require.Contains(t, text, want)
	}

	// spec interface that isn't wired and has nothing reported
	out, err = Port(t.Context(), kube, PortIn{Port: "gateway-1/port2"})
	require.NoError(t, err)
	require.Nil(t, out.ConnectionName)
	require.Nil(t, out.GatewayInterfaceState)
	require.Nil(t, out.GatewayTransceiverState)
}

func TestGatewayConnection(t *testing.T) {
	t.Parallel()

	kube := gatewayTestKube(t, time.Now())

	out, err := Connection(t.Context(), kube, ConnectionIn{Name: "s5248-03--gateway--gateway-1"})
	require.NoError(t, err)

	// the switch has no agent here, so only the gateway side is reported
	require.Len(t, out.Ports, 1)
	require.Equal(t, "gateway-1/port1", out.Ports[0].Name)
	require.NotNil(t, out.Ports[0].GatewayState)
	require.Equal(t, gwintapi.InterfaceOperStatusUp, out.Ports[0].GatewayState.OperStatus)
}

func TestGateway(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	kube := gatewayTestKube(t, now)

	out, err := Gateway(t.Context(), kube, GatewayIn{Name: "gateway-1"})
	require.NoError(t, err)
	require.Equal(t, "v0.39.0", out.Dataplane)
	require.Len(t, out.Ports, 2)
	require.Equal(t, "port1", out.Ports[0].PortName)
	require.Equal(t, "s5248-03--gateway--gateway-1", out.Ports[0].ConnectionName)
	require.NotNil(t, out.Ports[0].InterfaceState)
	require.NotNil(t, out.Ports[0].TransceiverState)
	require.Equal(t, "port2", out.Ports[1].PortName)
	require.Empty(t, out.Ports[1].ConnectionName)
	require.Nil(t, out.Ports[1].InterfaceState)

	text, err := out.MarshalText(GatewayIn{Ports: true, Transceivers: true, Counters: true, Lasers: true}, now)
	require.NoError(t, err)
	for _, want := range []string{"Ready", "v0.39.0", "0000:05:00.0", "0000:05:00.1", "SFP28 25GBASE-CR-DAC-1.0M", "COPPER_PIGTAIL", "1m", "↑ 1,582", "Laser Status:"} {
		require.Contains(t, text, want)
	}
}

func TestLLDPOutMarshalTextGateway(t *testing.T) {
	t.Parallel()

	out := &LLDPOut{
		Neighbors: map[string]map[string]apiutil.LLDPNeighborStatus{},
		GatewayNeighbors: map[string]map[string]apiutil.LLDPNeighborStatus{
			"gateway-1": {
				"port1": {
					ConnectionName: "s5248-03--gateway--gateway-1",
					ConnectionType: wiringapi.ConnectionTypeGateway,
					Type:           apiutil.LLDPNeighborTypeGateway,
					Expected:       apiutil.LLDPNeighbor{Name: "s5248-03", Port: "E1/3"},
					Actual:         []apiutil.LLDPNeighbor{{Name: "s5248-03", Port: "E1/4"}},
				},
				// gateway interface names are free-form, this one isn't a management port
				"MLX0": {
					ConnectionName: "s5248-04--gateway--gateway-1",
					ConnectionType: wiringapi.ConnectionTypeGateway,
					Type:           apiutil.LLDPNeighborTypeGateway,
					Expected:       apiutil.LLDPNeighbor{Name: "s5248-04", Port: "E1/3"},
					Actual:         []apiutil.LLDPNeighbor{{Name: "s5248-04", Port: "E1/3"}},
				},
			},
		},
	}

	res, err := out.MarshalText(LLDPIn{}, time.Now())
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(res), "\n")
	require.Equal(t, "Gateway: gateway-1", lines[0])
	require.Len(t, lines, 4)
	require.True(t, strings.HasPrefix(lines[2], "MLX0"))
	require.Contains(t, lines[3], "E1/4 (want E1/3)")

	errs := lldpStrictErrs("gateway gateway-1", "port1", out.GatewayNeighbors["gateway-1"]["port1"])
	require.Len(t, errs, 1)
	require.Equal(t, `gateway gateway-1: port1: expected neighbor port "E1/3", got "E1/4"`, errs[0].Error())
}
