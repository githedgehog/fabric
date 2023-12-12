package bcm

import (
	"context"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

var specCommunityListsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecCommunityList]{
	Summary:      "Community List",
	ValueHandler: specCommunityListEnforcer,
}

var specCommunityListEnforcer = &DefaultValueEnforcer[string, *dozer.SpecCommunityList]{
	Summary:      "Community Lists %s",
	Path:         "/routing-policy/defined-sets/bgp-defined-sets/community-sets[community-set-name=%s]",
	UpdateWeight: ActionWeightCommunityListUpdate,
	DeleteWeight: ActionWeightCommunityListDelete,
	Marshal: func(name string, value *dozer.SpecCommunityList) (ygot.ValidatedGoStruct, error) {
		members := []oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config_CommunityMember_Union{}
		for _, member := range value.Members {
			members = append(members, oc.UnionString(member))
		}

		return &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets{
			CommunitySets: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets{
				CommunitySet: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet{
					name: {
						CommunitySetName: ygot.String(name),
						Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config{
							CommunitySetName: ygot.String(name),
							CommunityMember:  members,
						},
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

		list := &dozer.SpecCommunityList{}
		for _, member := range ocList.Config.CommunityMember {
			if str, ok := member.(oc.UnionString); ok {
				list.Members = append(list.Members, string(str))
			} else {
				return nil, errors.Errorf("unexpected community member type: %T", member)
			}
		}

		lists[name] = list
	}

	return lists, nil
}
