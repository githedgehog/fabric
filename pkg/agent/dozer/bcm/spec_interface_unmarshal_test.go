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

package bcm

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

func TestUnmarshalOCInterfacesSubinterfaces(t *testing.T) {
	type ocSub = oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface
	type ocV6 = oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6
	linkLocal := "fe80::e20:12ff:feff:209"
	linkLocalAddrs := &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Addresses{
		Address: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Addresses_Address{
			linkLocal: {
				Ip: new(linkLocal),
				State: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Addresses_Address_State{
					Ip: new(linkLocal), PrefixLength: new(uint8(64)),
				},
			},
		},
	}

	ocIfaces := &oc.OpenconfigInterfaces_Interfaces{
		Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
			"Ethernet4": {
				Name:   new("Ethernet4"),
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Config{Description: new("Fabric")},
				Subinterfaces: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces{
					Subinterface: map[uint32]*ocSub{
						0: {
							Index:  new(uint32(0)),
							Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Config{Index: new(uint32(0))},
						},
						// config-less subinterface whose IPv6 enable only shows up in state, see 46f8f0b91
						5: {
							Index: new(uint32(5)),
							Ipv6: &ocV6{
								State: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_State{Enabled: new(true)},
							},
						},
						6: {
							Index: new(uint32(6)),
							Ipv6: &ocV6{
								Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_Config{Enabled: new(true)},
							},
						},
						// phantoms from SONiC's bulk read, borrowing the index of Ethernet0.10/.20
						10: {
							Index: new(uint32(10)),
							Ipv6:  &ocV6{Addresses: linkLocalAddrs},
						},
						20: {
							Index: new(uint32(20)),
							Ipv6: &ocV6{
								Addresses: linkLocalAddrs,
								State:     &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_State{Enabled: new(false)},
							},
						},
						// skipped before anything that can fail to parse
						21: {
							Index: new(uint32(21)),
							Ipv4: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4{
								ProxyArp: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_ProxyArp{
									Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_ProxyArp_Config{Mode: 99},
								},
							},
						},
						// a phantom merged into a real subinterface with the same index
						30: {
							Index:  new(uint32(30)),
							Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Config{Index: new(uint32(30))},
							Ipv6:   &ocV6{Addresses: linkLocalAddrs},
						},
					},
				},
			},
		},
	}

	agent := &agentapi.Agent{Spec: agentapi.AgentSpec{SwitchProfile: &wiringapi.SwitchProfileSpec{}}}
	ifaces, err := unmarshalOCInterfaces(agent, ocIfaces)
	require.NoError(t, err)
	require.Contains(t, ifaces, "Ethernet4")
	subs := ifaces["Ethernet4"].Subinterfaces
	require.Equal(t, []uint32{0, 5, 6, 30}, slices.Sorted(maps.Keys(subs)))
	require.NotNil(t, subs[5].IPv6)
	require.NotNil(t, subs[6].IPv6)
	require.Nil(t, subs[30].IPv6)
}
