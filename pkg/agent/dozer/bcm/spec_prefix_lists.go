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
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

var specPrefixListsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecPrefixList]{
	Summary:      "Prefix List",
	ValueHandler: specPrefixListEnforcer,
}

var specPrefixListEnforcer = &DefaultValueEnforcer[string, *dozer.SpecPrefixList]{
	Summary: "Prefix List %s",
	CustomHandler: func(basePath, name string, actual, desired *dozer.SpecPrefixList, actions *ActionQueue) error {
		basePath += fmt.Sprintf("/routing-policy/defined-sets/prefix-sets/prefix-set[name=%s]", name)

		if err := specPrefixListBaseEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrapf(err, "failed to enforce prefix list base")
		}

		actualEntries, desiredEntries := ValueOrNil(actual, desired,
			func(value *dozer.SpecPrefixList) map[uint32]*dozer.SpecPrefixListEntry { return value.Prefixes })

		if err := specPrefixListEntriesEnforcer.Handle(basePath, actualEntries, desiredEntries, actions); err != nil {
			return errors.Wrapf(err, "failed to enforce prefix list entries")
		}

		return nil
	},
}

var specPrefixListBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecPrefixList]{
	Summary:   "Prefix List Base %s",
	NoReplace: true, // we don't want to replace the whole prefix list, just update the entries
	Getter: func(name string, value *dozer.SpecPrefixList) any {
		return name // we do only care about the name of the prefix list
	},
	UpdateWeight: ActionWeightPrefixListUpdate,
	DeleteWeight: ActionWeightPrefixListDelete,
	Marshal: func(name string, _ *dozer.SpecPrefixList) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets{
			PrefixSet: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet{
				name: {
					Name: pointer.To(name),
					Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config{
						Name: pointer.To(name),
						Mode: oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_IPV4,
					},
				},
			},
		}, nil
	},
}

var specPrefixListEntriesEnforcer = &DefaultMapEnforcer[uint32, *dozer.SpecPrefixListEntry]{
	Summary:      "Prefix List Entries",
	ValueHandler: specPrefixListEntryEnforcer,
}

func getMaskLenRange(entry *dozer.SpecPrefixListEntry) (string, error) {
	maskLenRange := "exact"
	if entry.Prefix.Ge > 0 || entry.Prefix.Le > 0 {
		prefixParts := strings.Split(entry.Prefix.Prefix, "/")
		if len(prefixParts) != 2 {
			return "", errors.Errorf("invalid prefix %s", entry.Prefix.Prefix)
		}

		ge := fmt.Sprintf("%d", entry.Prefix.Ge)
		le := fmt.Sprintf("%d", entry.Prefix.Le)

		if entry.Prefix.Ge == 0 {
			ge = prefixParts[1]
		}
		if entry.Prefix.Le == 0 {
			le = "32"
		}

		maskLenRange = fmt.Sprintf("%s..%s", ge, le)
	}

	return maskLenRange, nil
}

var specPrefixListEntryEnforcer = &DefaultValueEnforcer[uint32, *dozer.SpecPrefixListEntry]{
	Summary: "Prefix Lists Entry %d",
	PathFunc: func(seq uint32, value *dozer.SpecPrefixListEntry) string {
		maskLenRange, err := getMaskLenRange(value)
		if err != nil {
			maskLenRange = "invalid"
		}

		return fmt.Sprintf("/extended-prefixes/extended-prefix[sequence-number=%d][ip-prefix=%s][masklength-range=%s]", seq, value.Prefix.Prefix, maskLenRange)
	},
	RecreateOnUpdate: true, // TODO validate
	UpdateWeight:     ActionWeightPrefixListEntryUpdate,
	DeleteWeight:     ActionWeightPrefixListEntryDelete,
	Marshal: func(seq uint32, entry *dozer.SpecPrefixListEntry) (ygot.ValidatedGoStruct, error) {
		action := oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_UNSET
		if entry.Action == dozer.SpecPrefixListActionPermit {
			action = oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_PERMIT
		} else if entry.Action == dozer.SpecPrefixListActionDeny {
			action = oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_DENY
		}

		maskLenRange, err := getMaskLenRange(entry)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get mask length range")
		}

		return &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes{
			ExtendedPrefix: map[oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes_ExtendedPrefix_Key]*oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes_ExtendedPrefix{
				{
					SequenceNumber:  seq,
					IpPrefix:        entry.Prefix.Prefix,
					MasklengthRange: maskLenRange,
				}: {
					SequenceNumber:  pointer.To(seq),
					IpPrefix:        pointer.To(entry.Prefix.Prefix),
					MasklengthRange: pointer.To(maskLenRange),
					Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_ExtendedPrefixes_ExtendedPrefix_Config{
						SequenceNumber:  pointer.To(seq),
						IpPrefix:        pointer.To(entry.Prefix.Prefix),
						MasklengthRange: pointer.To(maskLenRange),
						Action:          action,
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
			Prefixes: map[uint32]*dozer.SpecPrefixListEntry{},
		}

		if ocPrefixList.ExtendedPrefixes != nil {
			for key, ocPrefix := range ocPrefixList.ExtendedPrefixes.ExtendedPrefix {
				if ocPrefix.Config == nil {
					continue
				}

				if ocPrefix.Config.MasklengthRange == nil {
					continue
				}

				action := dozer.SpecPrefixListActionUnset
				if ocPrefix.Config.Action == oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_PERMIT {
					action = dozer.SpecPrefixListActionPermit
				} else if ocPrefix.Config.Action == oc.OpenconfigRoutingPolicyExt_RoutingPolicyExtActionType_DENY {
					action = dozer.SpecPrefixListActionDeny
				}

				le := uint8(0)
				ge := uint8(0)

				if *ocPrefix.Config.MasklengthRange != "exact" {
					parts := strings.Split(*ocPrefix.Config.MasklengthRange, "..")
					if len(parts) != 2 {
						return nil, errors.Errorf("invalid mask length range %s for prefix list %s", *ocPrefix.Config.MasklengthRange, name)
					}

					leR, err := strconv.ParseUint(parts[1], 10, 8)
					if err != nil {
						return nil, errors.Wrapf(err, "invalid mask length range %s for prefix list %s", *ocPrefix.Config.MasklengthRange, name)
					}
					le = uint8(leR)

					geR, err := strconv.ParseUint(parts[0], 10, 8)
					if err != nil {
						return nil, errors.Wrapf(err, "invalid mask length range %s for prefix list %s", *ocPrefix.Config.MasklengthRange, name)
					}
					ge = uint8(geR)
				}

				_, ipNet, err := net.ParseCIDR(key.IpPrefix)
				if err != nil {
					return nil, errors.Wrapf(err, "invalid prefix %s for prefix list %s", key.IpPrefix, name)
				}
				prefixLen, _ := ipNet.Mask.Size()

				if ge == uint8(prefixLen) { //nolint:gosec
					ge = 0
				}

				prefixList.Prefixes[key.SequenceNumber] = &dozer.SpecPrefixListEntry{
					Prefix: dozer.SpecPrefixListPrefix{
						Prefix: key.IpPrefix,
						Le:     le,
						Ge:     ge,
					},
					Action: action,
				}
			}
		}

		prefixLists[name] = prefixList
	}

	return prefixLists, nil
}
