// Copyright 2023 Hedgehog
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
	"context"
	"log/slog"
	"slices"
	"sort"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

var specAsPathListsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecAsPathList]{
	Summary:      "AS Path List",
	ValueHandler: specAsPathListEnforcer,
}

var specAsPathListEnforcer = &DefaultValueEnforcer[string, *dozer.SpecAsPathList]{
	Summary:      "AS Path Lists %s",
	Path:         "/routing-policy/defined-sets/bgp-defined-sets/as-path-sets/as-path-set[as-path-set-name=%s]",
	UpdateWeight: ActionWeightAsPathListUpdate,
	DeleteWeight: ActionWeightAsPathListDelete,
	Marshal: func(name string, value *dozer.SpecAsPathList) (ygot.ValidatedGoStruct, error) {
		memberStrs := slices.Clone(value.Members)
		sort.Strings(memberStrs)
		memberStrs = slices.Compact(memberStrs)

		return &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_AsPathSets{
			AsPathSet: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_AsPathSets_AsPathSet{
				name: {
					AsPathSetName: pointer.To(name),
					Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_AsPathSets_AsPathSet_Config{
						AsPathSetName:   pointer.To(name),
						AsPathSetMember: memberStrs,
						Action:          oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_PERMIT,
					},
				},
			},
		}, nil
	},
}

func loadActualAsPathLists(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocAsPathLists := &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets{}
	err := client.Get(ctx, "/routing-policy/defined-sets/bgp-defined-sets/as-path-sets", ocAsPathLists)
	if err != nil {
		return errors.Wrapf(err, "failed to read as-path lists")
	}
	spec.AsPathLists = unmarshalOCAsPathLists(ocAsPathLists)

	return nil
}

func unmarshalOCAsPathLists(ocVal *oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets) map[string]*dozer.SpecAsPathList {
	lists := map[string]*dozer.SpecAsPathList{}

	if ocVal.AsPathSets == nil {
		return lists
	}

	for name, ocList := range ocVal.AsPathSets.AsPathSet {
		if ocList.Config == nil {
			continue
		}

		if ocList.Config.Action != oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_PERMIT &&
			ocList.Config.Action != oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_UNSET {
			slog.Warn("unsupported as-path list action", "name", name, "action", ocList.Config.Action)

			continue
		}

		list := &dozer.SpecAsPathList{}
		for _, member := range ocList.Config.AsPathSetMember {
			if len(member) == 0 {
				continue
			}
			list.Members = append(list.Members, member)
		}

		lists[name] = list
	}

	return lists
}
