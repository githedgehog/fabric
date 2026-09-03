// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package apiutil_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Unnumbered sessions are keyed by the port they run over, so the expected neighbors have to line
// up with what the agent reports for them: one entry per link, rather than every unnumbered link
// on the switch sharing a single key that matches nothing.
func TestGetBGPNeighborsUnnumbered(t *testing.T) {
	const (
		self     = "leaf-01"
		spine    = "spine-01"
		spineASN = uint32(65100)
		wVLAN    = uint16(3123)
	)

	sw := &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Name: self, Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.SwitchSpec{Role: wiringapi.SwitchRoleServerLeaf},
	}

	fabricConn := &wiringapi.Connection{
		ObjectMeta: kmetav1.ObjectMeta{Name: self + "--fabric--" + spine, Namespace: kmetav1.NamespaceDefault},
		Spec: wiringapi.ConnectionSpec{
			Fabric: &wiringapi.ConnFabric{
				Links: []wiringapi.FabricLink{
					{
						Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName(spine + "/E1/1")},
						Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName(self + "/E1/1")},
					},
					{
						Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName(spine + "/E1/2")},
						Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName(self + "/E1/2")},
					},
					// a breakout-capable port, which the agent reports as E1/53/1
					{
						Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName(spine + "/E1/53")},
						Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.NewBasePortName(self + "/E1/53")},
					},
				},
			},
		},
	}

	// E1/1 and E1/2 are plain SFP28 ports on the S5248F, E1/53 a breakout-capable QSFP28
	newAgent := func(silicon string) *agentapi.Agent {
		ag := &agentapi.Agent{ObjectMeta: kmetav1.ObjectMeta{Name: self, Namespace: kmetav1.NamespaceDefault}}
		ag.Spec.SwitchProfile = switchprofile.DellS5248FON.Spec.DeepCopy()
		ag.Spec.SwitchProfile.SwitchSilicon = silicon
		ag.Spec.Switches = map[string]wiringapi.SwitchSpec{
			spine: {ASN: spineASN, ProtocolIP: "172.30.11.2/32"},
		}
		ag.Spec.Catalog.TH5WorkaroundVLANs = map[string]uint16{"E1/1": wVLAN, "E1/2": wVLAN + 1, "E1/53": wVLAN + 2}

		return ag
	}

	// what the agent reports once the NOS interface names are translated back to API ports
	reported := func(keys ...string) map[string]map[string]agentapi.SwitchStateBGPNeighbor {
		neighs := map[string]agentapi.SwitchStateBGPNeighbor{}
		for _, key := range keys {
			neighs[key] = agentapi.SwitchStateBGPNeighbor{SessionState: agentapi.BGPNeighborSessionStateEstablished}
		}

		return map[string]map[string]agentapi.SwitchStateBGPNeighbor{"default": neighs}
	}

	type neighbor struct {
		conn string
		port string
	}

	for _, tt := range []struct {
		name     string
		silicon  string
		reported []string
		expected map[string]neighbor // neighbor key -> connection and port it belongs to
	}{
		{
			name:     "fabric links",
			silicon:  switchprofile.SiliconVS,
			reported: []string{"E1/1", "E1/2", "E1/53/1", "172.30.11.2"},
			expected: map[string]neighbor{
				"E1/1":        {conn: fabricConn.Name, port: "E1/1"},
				"E1/2":        {conn: fabricConn.Name, port: "E1/2"},
				"E1/53/1":     {conn: fabricConn.Name, port: "E1/53/1"},
				"172.30.11.2": {port: "Lo"},
			},
		},
		{
			// the peering runs over the workaround SVI, one per link, named after the port the
			// VLAN was allocated for
			name:     "on TH5",
			silicon:  switchprofile.SiliconBroadcomTH5,
			reported: []string{"E1/1.3123", "E1/2.3124", "E1/53.3125", "172.30.11.2"},
			expected: map[string]neighbor{
				"E1/1.3123":   {conn: fabricConn.Name, port: "E1/1"},
				"E1/2.3124":   {conn: fabricConn.Name, port: "E1/2"},
				"E1/53.3125":  {conn: fabricConn.Name, port: "E1/53/1"},
				"172.30.11.2": {port: "Lo"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, wiringapi.AddToScheme(scheme))
			require.NoError(t, vpcapi.AddToScheme(scheme))
			require.NoError(t, agentapi.AddToScheme(scheme))

			ag := newAgent(tt.silicon)
			ag.Status.State.BGPNeighbors = reported(tt.reported...)

			objs := []kclient.Object{sw, ag}
			for _, conn := range []*wiringapi.Connection{fabricConn} {
				conn := conn.DeepCopy()
				conn.Default()
				objs = append(objs, conn)
			}

			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
				WithStatusSubresource(ag).Build()
			require.NoError(t, kube.Status().Update(t.Context(), ag))

			neighs, err := apiutil.GetBGPNeighbors(t.Context(), kube, &meta.FabricConfig{}, sw)
			require.NoError(t, err)

			// an expected key that does not match what the agent reports shows up as an extra
			// entry, which is what keying every unnumbered session by "unnum" used to do
			require.Len(t, neighs["default"], len(tt.expected))

			for key, want := range tt.expected {
				neigh, ok := neighs["default"][key]
				require.True(t, ok, "neighbor %s must be present", key)
				require.True(t, neigh.Expected, "neighbor %s must be expected", key)
				require.Equal(t, want.conn, neigh.ConnectionName)
				require.Equal(t, want.port, neigh.Port)
				require.Equal(t, agentapi.BGPNeighborSessionStateEstablished, neigh.SessionState,
					"neighbor %s must join the state the agent reports", key)
			}
		})
	}
}
