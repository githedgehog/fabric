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

const (
	rebuildIface   = IfacePrefixPhysical + "0"
	rebuildSubName = rebuildIface + ".2024"
	rebuildVRF     = "VrfVvpc-01"
	rebuildSubPath = "/interfaces/interface[name=" + rebuildIface + "]/subinterfaces/subinterface[index=2024]"
)

func rebuildSpec(trunkVLANs []string, withSub bool) *dozer.Spec {
	iface := &dozer.SpecInterface{
		TrunkVLANs:    trunkVLANs,
		Subinterfaces: map[uint32]*dozer.SpecSubinterface{0: {}},
	}
	vrf := &dozer.SpecVRF{
		Enabled:    pointer.To(true),
		Interfaces: map[string]*dozer.SpecVRFInterface{},
		BGP: &dozer.SpecVRFBGP{
			AS:        pointer.To(uint32(65101)),
			Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{},
		},
	}
	spec := &dozer.Spec{
		Interfaces:    map[string]*dozer.SpecInterface{rebuildIface: iface},
		VRFs:          map[string]*dozer.SpecVRF{rebuildVRF: vrf},
		ACLInterfaces: map[string]*dozer.SpecACLInterface{},
	}
	if withSub {
		iface.Subinterfaces[2024] = &dozer.SpecSubinterface{
			VLAN: pointer.To(uint16(2024)),
			IPv6: &dozer.SpecInterfaceIPv6{Enabled: pointer.To(true)},
		}
		vrf.Interfaces[rebuildSubName] = &dozer.SpecVRFInterface{}
		vrf.BGP.Neighbors[rebuildSubName] = &dozer.SpecVRFBGPNeighbor{
			Enabled:  pointer.To(true),
			PeerType: pointer.To(string(dozer.SpecVRFBGPNeighborPeerTypeExternal)),
		}
		spec.ACLInterfaces[rebuildSubName] = &dozer.SpecACLInterface{Ingress: pointer.To("acl-vpc-01")}
	}

	return spec
}

// index of the first action with the given weight whose path starts with the given prefix, -1 if none
func actionIndex(actions []dozer.Action, weight ActionWeight, pathPrefix string) int {
	for idx, a := range actions {
		act := a.(*Action)
		if act.Weight == weight && len(act.Path) >= len(pathPrefix) && act.Path[:len(pathPrefix)] == pathPrefix {
			return idx
		}
	}

	return -1
}

func TestCalculateActionsRebuildsSubinterfaceAfterLastSwitchedVLAN(t *testing.T) {
	trunkPath := "/interfaces/interface[name=" + rebuildIface + "]/ethernet/switched-vlan/config/trunk-vlans"

	for _, tt := range []struct {
		name        string
		actual      *dozer.Spec
		desired     *dozer.Spec
		wantRebuild bool
	}{
		{
			name:        "last trunk VLAN removed, subinterface stays",
			actual:      rebuildSpec([]string{"1501"}, true),
			desired:     rebuildSpec(nil, true),
			wantRebuild: true,
		},
		{
			name:    "one of two trunk VLANs removed, subinterface stays",
			actual:  rebuildSpec([]string{"1501", "1502"}, true),
			desired: rebuildSpec([]string{"1502"}, true),
		},
		{
			name:    "last trunk VLAN removed, no subinterface",
			actual:  rebuildSpec([]string{"1501"}, false),
			desired: rebuildSpec(nil, false),
		},
		{
			name:    "subinterface created after the last trunk VLAN is removed",
			actual:  rebuildSpec([]string{"1501"}, false),
			desired: rebuildSpec(nil, true),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actions, err := Processor().CalculateActions(t.Context(), tt.actual, tt.desired)
			require.NoError(t, err)

			subDelete := actionIndex(actions, ActionWeightInterfaceSubinterfaceDelete, rebuildSubPath)
			subCreate := actionIndex(actions, ActionWeightInterfaceSubinterfaceUpdate, rebuildSubPath)

			if !tt.wantRebuild {
				require.Equal(t, -1, subDelete, "unexpected subinterface delete")

				return
			}

			trunkDelete := actionIndex(actions, ActionWeightInterfaceEthernetSwitchedTrunkDelete, trunkPath)
			vrfDelete := actionIndex(actions, ActionWeightVRFInterfaceDelete, "")
			vrfCreate := actionIndex(actions, ActionWeightVRFInterfaceUpdate, "")
			neighDelete := actionIndex(actions, ActionWeightVRFBGPNeighborDelete, "")
			neighCreate := actionIndex(actions, ActionWeightVRFBGPNeighborUpdate, "")

			require.NotEqual(t, -1, trunkDelete, "trunk VLAN delete")
			require.NotEqual(t, -1, subDelete, "subinterface delete")
			require.NotEqual(t, -1, subCreate, "subinterface create")
			require.NotEqual(t, -1, vrfDelete, "VRF interface delete")
			require.NotEqual(t, -1, vrfCreate, "VRF interface create")
			require.NotEqual(t, -1, neighDelete, "BGP neighbor delete")
			require.NotEqual(t, -1, neighCreate, "BGP neighbor create")

			require.Less(t, vrfDelete, subDelete)
			require.Less(t, neighDelete, subDelete)
			require.Less(t, subDelete, trunkDelete)
			require.Less(t, trunkDelete, subCreate)
			require.Less(t, subCreate, vrfCreate)
			require.Less(t, subCreate, neighCreate)
		})
	}
}

func TestWithoutRebuiltSubinterfacesKeepsDesired(t *testing.T) {
	actual := rebuildSpec([]string{"1501"}, true)
	desired := rebuildSpec(nil, true)

	intermediate := withoutRebuiltSubinterfaces(actual, desired)
	require.NotNil(t, intermediate)

	require.NotContains(t, intermediate.Interfaces[rebuildIface].Subinterfaces, uint32(2024))
	require.Contains(t, intermediate.Interfaces[rebuildIface].Subinterfaces, uint32(0))
	require.NotContains(t, intermediate.VRFs[rebuildVRF].Interfaces, rebuildSubName)
	require.NotContains(t, intermediate.VRFs[rebuildVRF].BGP.Neighbors, rebuildSubName)
	require.NotContains(t, intermediate.ACLInterfaces, rebuildSubName)

	// the desired spec itself stays untouched
	require.Equal(t, rebuildSpec(nil, true), desired)
}

func TestCalculateActionsRebuildsAllSubinterfacesOfPort(t *testing.T) {
	const secondSubName = rebuildIface + ".2025"
	const secondSubPath = "/interfaces/interface[name=" + rebuildIface + "]/subinterfaces/subinterface[index=2025]"
	withSecond := func(spec *dozer.Spec) *dozer.Spec {
		spec.Interfaces[rebuildIface].Subinterfaces[2025] = &dozer.SpecSubinterface{
			VLAN: pointer.To(uint16(2025)),
			IPv6: &dozer.SpecInterfaceIPv6{Enabled: pointer.To(true)},
		}
		spec.VRFs[rebuildVRF].Interfaces[secondSubName] = &dozer.SpecVRFInterface{}
		spec.VRFs[rebuildVRF].BGP.Neighbors[secondSubName] = &dozer.SpecVRFBGPNeighbor{
			Enabled:  pointer.To(true),
			PeerType: pointer.To(string(dozer.SpecVRFBGPNeighborPeerTypeExternal)),
		}

		return spec
	}

	actions, err := Processor().CalculateActions(t.Context(), withSecond(rebuildSpec([]string{"1501"}, true)), withSecond(rebuildSpec(nil, true)))
	require.NoError(t, err)

	trunkDelete := actionIndex(actions, ActionWeightInterfaceEthernetSwitchedTrunkDelete, "/interfaces/interface[name="+rebuildIface+"]/ethernet/switched-vlan")
	require.NotEqual(t, -1, trunkDelete, "trunk VLAN delete")
	for _, path := range []string{rebuildSubPath, secondSubPath} {
		subDelete := actionIndex(actions, ActionWeightInterfaceSubinterfaceDelete, path)
		subCreate := actionIndex(actions, ActionWeightInterfaceSubinterfaceUpdate, path)
		require.NotEqual(t, -1, subDelete, "subinterface delete %s", path)
		require.NotEqual(t, -1, subCreate, "subinterface create %s", path)
		require.Less(t, subDelete, trunkDelete, path)
		require.Less(t, trunkDelete, subCreate, path)
	}
}
