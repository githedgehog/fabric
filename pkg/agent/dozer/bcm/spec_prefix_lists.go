package bcm

import (
	"context"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

var specPrefixListsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecPrefixList]{
	Summary:      "Prefix List",
	ValueHandler: specPrefixListEnforcer,
}

var specPrefixListEnforcer = &DefaultValueEnforcer[string, *dozer.SpecPrefixList]{
	Summary:      "Prefix Lists %s",
	Path:         "/routing-policy/defined-sets/prefix-sets[name=%s]",
	UpdateWeight: ActionWeightPrefixListUpdate,
	DeleteWeight: ActionWeightPrefixListDelete,
	Marshal: func(name string, value *dozer.SpecPrefixList) (ygot.ValidatedGoStruct, error) {
		prefixes := map[oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes_ExtendedPrefix_Key]*oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes_ExtendedPrefix{}

		for seq, prefix := range value.Prefixes {
			action := oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_UNSET
			if prefix.Action == dozer.SpecPrefixListActionPermit {
				action = oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_PERMIT
			} else if prefix.Action == dozer.SpecPrefixListActionDeny {
				action = oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_DENY
			}

			prefixes[oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes_ExtendedPrefix_Key{
				SequenceNumber:  seq,
				IpPrefix:        prefix.Prefix,
				MasklengthRange: "exact",
			}] = &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes_ExtendedPrefix{
				SequenceNumber:  ygot.Uint32(seq),
				IpPrefix:        ygot.String(prefix.Prefix),
				MasklengthRange: ygot.String("exact"),
				Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes_ExtendedPrefix_Config{
					SequenceNumber:  ygot.Uint32(seq),
					IpPrefix:        ygot.String(prefix.Prefix),
					MasklengthRange: ygot.String("exact"),
					Action:          action,
				},
			}
		}

		return &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets{
			PrefixSets: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets{
				PrefixSet: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet{
					name: {
						Name: ygot.String(name),
						Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config{
							Name: ygot.String(name),
							Mode: oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_IPV4,
						},
						// TODO handle separately to be able to update prefix lists
						ExtendedPrefixes: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes{
							ExtendedPrefix: prefixes,
						},
					},
				},
			},
		}, nil
	},
}

func loadActualPrefixLists(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocPrefixLists := &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets{}
	err := client.Get(ctx, "/routing-policy/defined-sets/prefix-sets", ocPrefixLists, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read prefix lists")
	}
	spec.PrefixLists, err = unmarshalOCPrefixLists(ocPrefixLists)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal prefix lists")
	}

	return nil
}

func unmarshalOCPrefixLists(ocVal *oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets) (map[string]*dozer.SpecPrefixList, error) {
	prefixLists := map[string]*dozer.SpecPrefixList{}

	if ocVal == nil || ocVal.PrefixSets == nil {
		return prefixLists, nil
	}

	for name, ocPrefixList := range ocVal.PrefixSets.PrefixSet {
		if ocPrefixList.Config == nil || ocPrefixList.Config.Mode != oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_IPV4 {
			continue
		}

		prefixList := &dozer.SpecPrefixList{
			Prefixes: map[uint32]*dozer.SpecPrefixListPrefix{},
		}

		if ocPrefixList.ExtendedPrefixes != nil {
			for key, ocPrefix := range ocPrefixList.ExtendedPrefixes.ExtendedPrefix {
				if ocPrefix.Config == nil {
					continue
				}

				if ocPrefix.Config.MasklengthRange == nil || ocPrefix.Config.MasklengthRange != nil && *ocPrefix.Config.MasklengthRange != "exact" {
					continue
				}

				action := dozer.SpecPrefixListActionUnset
				if ocPrefix.Config.Action == oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_PERMIT {
					action = dozer.SpecPrefixListActionPermit
				} else if ocPrefix.Config.Action == oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_DENY {
					action = dozer.SpecPrefixListActionDeny
				}

				prefixList.Prefixes[key.SequenceNumber] = &dozer.SpecPrefixListPrefix{
					Prefix: key.IpPrefix,
					Action: action,
				}
			}
		}

		prefixLists[name] = prefixList
	}

	return prefixLists, nil
}
