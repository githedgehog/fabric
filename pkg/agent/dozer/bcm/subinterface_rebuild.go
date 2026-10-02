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
	"fmt"
	"maps"
	"slices"

	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

// When a port loses its last switched (access or trunk) VLAN, SONiC sets the port's CPU host interface to strip
// VLAN tags, and only sets it back to keep them when the first VLAN subinterface of the port is created. A VLAN
// subinterface that already exists at that point stays in place but no longer receives any tagged traffic.
// withoutRebuiltSubinterfaces returns the desired spec without those subinterfaces and everything referencing
// them, so they can be deleted together with the switched VLAN and created again afterwards; it returns nil if
// no subinterface needs a rebuild.
func withoutRebuiltSubinterfaces(actual, desired *dozer.Spec) *dozer.Spec {
	if actual == nil || desired == nil {
		return nil
	}

	rebuild := map[string][]uint32{}
	for name, desiredIface := range desired.Interfaces {
		actualIface := actual.Interfaces[name]
		if !isPhysical(name) || actualIface == nil || desiredIface == nil {
			continue
		}
		if !hasSwitchedVLAN(actualIface) || hasSwitchedVLAN(desiredIface) {
			continue
		}
		for idx, sub := range desiredIface.Subinterfaces {
			if idx == 0 || sub == nil || sub.VLAN == nil {
				continue
			}
			if _, ok := actualIface.Subinterfaces[idx]; ok {
				rebuild[name] = append(rebuild[name], idx)
			}
		}
	}
	if len(rebuild) == 0 {
		return nil
	}

	intermediate := *desired
	intermediate.Interfaces = maps.Clone(desired.Interfaces)
	intermediate.VRFs = maps.Clone(desired.VRFs)
	intermediate.ACLInterfaces = maps.Clone(desired.ACLInterfaces)

	for _, name := range slices.Sorted(maps.Keys(rebuild)) {
		iface := *desired.Interfaces[name]
		iface.Subinterfaces = maps.Clone(iface.Subinterfaces)
		for _, idx := range rebuild[name] {
			delete(iface.Subinterfaces, idx)

			subName := fmt.Sprintf("%s.%d", name, idx)
			delete(intermediate.ACLInterfaces, subName)
			for vrfName, vrf := range intermediate.VRFs {
				if vrf == nil {
					continue
				}
				_, inVRF := vrf.Interfaces[subName]
				isNeighbor := false
				if vrf.BGP != nil {
					_, isNeighbor = vrf.BGP.Neighbors[subName]
				}
				if !inVRF && !isNeighbor {
					continue
				}

				vrfCopy := *vrf
				vrfCopy.Interfaces = maps.Clone(vrf.Interfaces)
				delete(vrfCopy.Interfaces, subName)
				if vrf.BGP != nil {
					bgp := *vrf.BGP
					bgp.Neighbors = maps.Clone(vrf.BGP.Neighbors)
					delete(bgp.Neighbors, subName)
					vrfCopy.BGP = &bgp
				}
				intermediate.VRFs[vrfName] = &vrfCopy
			}
		}
		intermediate.Interfaces[name] = &iface
	}

	return &intermediate
}

func hasSwitchedVLAN(iface *dozer.SpecInterface) bool {
	return iface.AccessVLAN != nil || len(iface.TrunkVLANs) > 0
}
