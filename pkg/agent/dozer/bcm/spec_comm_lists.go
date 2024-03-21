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

var specCommunityListsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecCommunityList]{
	Summary:      "Community List",
	ValueHandler: specCommunityListEnforcer,
}

var specCommunityListEnforcer = &DefaultValueEnforcer[string, *dozer.SpecCommunityList]{
	Summary:      "Community Lists %s",
	Path:         "/routing-policy/defined-sets/bgp-defined-sets/community-sets/community-set[community-set-name=%s]",
	UpdateWeight: ActionWeightCommunityListUpdate,
	DeleteWeight: ActionWeightCommunityListDelete,
	Marshal: func(name string, value *dozer.SpecCommunityList) (ygot.ValidatedGoStruct, error) {
		memberStrs := slices.Clone(value.Members)
		sort.Strings(memberStrs)

		members := []oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config_CommunityMember_Union{}
		for _, member := range memberStrs {
			members = append(members, oc.UnionString(member))
		}

		return &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets{
			CommunitySet: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet{
				name: {
					CommunitySetName: pointer.To(name),
					Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config{
						CommunitySetName: pointer.To(name),
						CommunityMember:  members,
						MatchSetOptions:  oc.OpenconfigRoutingPolicy_MatchSetOptionsType_ANY,
						Action:           oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_PERMIT,
					},
				},
			},
		}, nil
	},
}

func loadActualCommunityLists(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocCommLists := &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets{}
	err := client.Get(ctx, "/routing-policy/defined-sets/bgp-defined-sets/community-sets", ocCommLists)
	if err != nil {
		return errors.Wrapf(err, "failed to read community lists")
	}
	spec.CommunityLists, err = unmarshalOCCommunityLists(ocCommLists)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal community lists")
	}

	return nil
}

func unmarshalOCCommunityLists(ocVal *oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets) (map[string]*dozer.SpecCommunityList, error) {
	lists := map[string]*dozer.SpecCommunityList{}

	if ocVal.CommunitySets == nil {
		return lists, nil
	}

	for name, ocList := range ocVal.CommunitySets.CommunitySet {
		if ocList.Config == nil {
			continue
		}

		if ocList.Config.MatchSetOptions != oc.OpenconfigRoutingPolicy_MatchSetOptionsType_ANY {
			slog.Warn("unsupported community list match set options", "name", name, "match_set_options", ocList.Config.MatchSetOptions)
			continue
		}
		if ocList.Config.Action != oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_PERMIT {
			slog.Warn("unsupported community list action", "name", name, "action", ocList.Config.Action)
			continue
		}

		list := &dozer.SpecCommunityList{}
		for _, member := range ocList.Config.CommunityMember {
			if str, ok := member.(oc.UnionString); ok {
				if len(str) == 0 {
					continue
				}
				list.Members = append(list.Members, string(str))
			} else {
				return nil, errors.Errorf("unexpected community member type: %T", member)
			}
		}

		lists[name] = list
	}

	return lists, nil
}
