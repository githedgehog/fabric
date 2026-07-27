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
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

// A hostBGP attachment on a subnet with VLAN 0 enables IPv6 on subinterface 0 of the port itself,
// and that subinterface stays around after the attachment is removed. The IPv6 enable must still be
// dropped, and it has to happen before the port is removed from the VRF, otherwise SONiC rejects the
// VRF interface delete with "L3 Configuration exists for Interface".
func TestSpecInterfaceSubinterfaceIPv6Enforcer(t *testing.T) {
	const iface = IfacePrefixPhysical + "0"

	for _, tt := range []struct {
		name        string
		actual      *dozer.SpecSubinterface
		desired     *dozer.SpecSubinterface
		wantActions []Action
	}{
		{
			name:    "ipv6 dropped, subinterface stays",
			actual:  &dozer.SpecSubinterface{IPv6: &dozer.SpecInterfaceIPv6{Enabled: pointer.To(true)}},
			desired: &dozer.SpecSubinterface{},
			wantActions: []Action{{
				Weight: ActionWeightInterfaceSubinterfaceIPv6Delete,
				Type:   ActionTypeDelete,
				Path:   "/interfaces/interface[name=Ethernet0]/subinterfaces/subinterface[index=0]/ipv6/config/enabled",
			}},
		},
		{
			name:    "ipv6 dropped along with the subinterface",
			actual:  &dozer.SpecSubinterface{IPv6: &dozer.SpecInterfaceIPv6{Enabled: pointer.To(true)}},
			desired: nil,
			wantActions: []Action{{
				Weight: ActionWeightInterfaceSubinterfaceIPv6Delete,
				Type:   ActionTypeDelete,
				Path:   "/interfaces/interface[name=Ethernet0]/subinterfaces/subinterface[index=0]/ipv6/config/enabled",
			}},
		},
		{
			name:    "ipv6 enabled",
			actual:  &dozer.SpecSubinterface{},
			desired: &dozer.SpecSubinterface{IPv6: &dozer.SpecInterfaceIPv6{Enabled: pointer.To(true)}},
			wantActions: []Action{{
				Weight: ActionWeightInterfaceSubinterfaceIPv6Update,
				Type:   ActionTypeUpdate,
				Path:   "/interfaces/interface[name=Ethernet0]/subinterfaces/subinterface[index=0]/ipv6/config/enabled",
			}},
		},
		{
			name:        "ipv6 unchanged",
			actual:      &dozer.SpecSubinterface{IPv6: &dozer.SpecInterfaceIPv6{Enabled: pointer.To(true)}},
			desired:     &dozer.SpecSubinterface{IPv6: &dozer.SpecInterfaceIPv6{Enabled: pointer.To(true)}},
			wantActions: nil,
		},
		{
			// unrelated subinterface changes must not produce spurious IPv6 actions
			name:        "only IPs changed",
			actual:      &dozer.SpecSubinterface{IPs: map[string]*dozer.SpecInterfaceIP{"10.0.0.1": {PrefixLen: pointer.To(uint8(24))}}},
			desired:     &dozer.SpecSubinterface{},
			wantActions: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actions := &ActionQueue{}
			err := specInterfaceSubinterfaceEnforcer.Handle("/interfaces/interface[name="+iface+"]", 0, tt.actual, tt.desired, actions)
			require.NoError(t, err)

			ipv6Actions := []Action{}
			for _, a := range actions.actions {
				act := a.(*Action)
				if act.Weight != ActionWeightInterfaceSubinterfaceIPv6Delete && act.Weight != ActionWeightInterfaceSubinterfaceIPv6Update {
					continue
				}
				ipv6Actions = append(ipv6Actions, Action{Weight: act.Weight, Type: act.Type, Path: act.Path})
			}

			require.Len(t, ipv6Actions, len(tt.wantActions))
			for idx, want := range tt.wantActions {
				require.Equal(t, want, ipv6Actions[idx])
			}
		})
	}

	// the IPv6 enable must be removed before the port leaves the VRF
	require.Less(t, ActionWeightInterfaceSubinterfaceIPv6Delete, ActionWeightVRFInterfaceDelete)
}
