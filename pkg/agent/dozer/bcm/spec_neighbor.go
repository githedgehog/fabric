// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"context"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

// neighborGlobalName is the fixed key of the single NEIGH_GLOBAL_LIST entry as
// observed via sonic-cli ("Values").
const neighborGlobalName = "Values"

var specNeighborGlobalEnforcer = &DefaultValueEnforcer[string, *dozer.SpecNeighborGlobal]{
	Summary:      "Neighbor Global",
	CreatePath:   "/openconfig-neighbor:neighbor-globals/neighbor-global",
	Path:         "/openconfig-neighbor:neighbor-globals/neighbor-global[name=" + neighborGlobalName + "]",
	UpdateWeight: ActionWeightNeighborGlobalUpdate,
	DeleteWeight: ActionWeightNeighborGlobalDelete,
	Marshal: func(_ string, value *dozer.SpecNeighborGlobal) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNeighbor_NeighborGlobals{
			NeighborGlobal: map[string]*oc.OpenconfigNeighbor_NeighborGlobals_NeighborGlobal{
				neighborGlobalName: {
					Name: pointer.To(neighborGlobalName),
					Config: &oc.OpenconfigNeighbor_NeighborGlobals_NeighborGlobal_Config{
						Name:                      pointer.To(neighborGlobalName),
						Ipv4DropNeighborAgingTime: value.IPv4DropNeighborAgingTime,
					},
				},
			},
		}, nil
	},
}

func loadActualNeighborGlobal(ctx context.Context, client GNMICClient, spec *dozer.Spec) error {
	ocNeigh := &oc.OpenconfigNeighbor_NeighborGlobals{}
	err := client.Get(ctx, "/openconfig-neighbor:neighbor-globals/neighbor-global", ocNeigh)
	if err != nil {
		return errors.Wrapf(err, "failed to get neighbor global config")
	}

	spec.NeighborGlobal = unmarshalActualNeighborGlobal(ocNeigh)

	return nil
}

func unmarshalActualNeighborGlobal(ocVal *oc.OpenconfigNeighbor_NeighborGlobals) *dozer.SpecNeighborGlobal {
	if ocVal == nil {
		return nil
	}

	ng, ok := ocVal.NeighborGlobal[neighborGlobalName]
	if !ok || ng.Config == nil || ng.Config.Ipv4DropNeighborAgingTime == nil {
		return nil
	}

	return &dozer.SpecNeighborGlobal{
		IPv4DropNeighborAgingTime: ng.Config.Ipv4DropNeighborAgingTime,
	}
}
