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
	"log/slog"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

var specRouteMapsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecRouteMap]{
	Summary:      "Route Map",
	ValueHandler: specRouteMapEnforcer,
}

var specRouteMapEnforcer = &DefaultValueEnforcer[string, *dozer.SpecRouteMap]{
	Summary: "Route Maps %s",
	CustomHandler: func(basePath, name string, actual, desired *dozer.SpecRouteMap, actions *ActionQueue) error {
		basePath += fmt.Sprintf("/routing-policy/policy-definitions/policy-definition[name=%s]", name)

		if err := specRouteMapBaseEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrapf(err, "failed to enforce route map base")
		}

		actualStatements, desiredStatements := ValueOrNil(actual, desired,
			func(value *dozer.SpecRouteMap) map[string]*dozer.SpecRouteMapStatement { return value.Statements })
		if err := specRouteMapStatementsEnforcer.Handle(basePath, actualStatements, desiredStatements, actions); err != nil {
			return errors.Wrapf(err, "failed to enforce route map statements")
		}

		return nil
	},
}

var specRouteMapBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecRouteMap]{
	Summary:      "Route Maps Base %s",
	UpdateWeight: ActionWeightRouteMapUpdate,
	DeleteWeight: ActionWeightRouteMapDelete,
	Marshal: func(name string, value *dozer.SpecRouteMap) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions{
			PolicyDefinition: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition{
				name: {
					Name: pointer.To(name),
					Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Config{
						Name: pointer.To(name),
					},
				},
			},
		}, nil
	},
}

var specRouteMapStatementsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecRouteMapStatement]{
	Summary:      "Route Map Statement",
	ValueHandler: specRouteMapStatementEnforcer,
}

var specRouteMapStatementEnforcer = &DefaultValueEnforcer[string, *dozer.SpecRouteMapStatement]{
	Summary:          "Route Map Statement %s",
	Path:             "/statements/statement[name=%s]",
	RecreateOnUpdate: true, // TODO validate
	UpdateWeight:     ActionWeightRouteMapStatementUpdate,
	DeleteWeight:     ActionWeightRouteMapStatementDelete,
	Marshal: func(seq string, statement *dozer.SpecRouteMapStatement) (ygot.ValidatedGoStruct, error) {
		conditions := &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions{}
		if statement.Conditions.DirectlyConnected != nil && *statement.Conditions.DirectlyConnected {
			if conditions.Config == nil {
				conditions.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_Config{}
			}

			conditions.Config.InstallProtocolEq = oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED
		}
		if statement.Conditions.MatchPrefixList != nil {
			if conditions.MatchPrefixSet == nil {
				conditions.MatchPrefixSet = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_MatchPrefixSet{}
			}
			if conditions.MatchPrefixSet.Config == nil {
				conditions.MatchPrefixSet.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_MatchPrefixSet_Config{}
			}

			conditions.MatchPrefixSet.Config.PrefixSet = statement.Conditions.MatchPrefixList
		}
		if statement.Conditions.MatchCommunityList != nil {
			if conditions.BgpConditions == nil {
				conditions.BgpConditions = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions{}
			}
			if conditions.BgpConditions.Config == nil {
				conditions.BgpConditions.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions_Config{}
			}
			conditions.BgpConditions.Config.CommunitySet = statement.Conditions.MatchCommunityList
		}
		if statement.Conditions.MatchNextHopPrefixList != nil {
			if conditions.BgpConditions == nil {
				conditions.BgpConditions = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions{}
			}
			if conditions.BgpConditions.Config == nil {
				conditions.BgpConditions.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions_Config{}
			}
			conditions.BgpConditions.Config.NextHopSet = statement.Conditions.MatchNextHopPrefixList
		}
		if statement.Conditions.MatchEVPNDefaultRoute != nil && *statement.Conditions.MatchEVPNDefaultRoute {
			if conditions.BgpConditions == nil {
				conditions.BgpConditions = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions{}
			}
			if conditions.BgpConditions.MatchEvpnSet == nil {
				conditions.BgpConditions.MatchEvpnSet = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions_MatchEvpnSet{}
			}
			if conditions.BgpConditions.MatchEvpnSet.Config == nil {
				conditions.BgpConditions.MatchEvpnSet.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions_MatchEvpnSet_Config{}
			}

			conditions.BgpConditions.MatchEvpnSet.Config.DefaultType5Route = pointer.To(true)
		}
		if statement.Conditions.MatchEVPNVNI != nil {
			if conditions.BgpConditions == nil {
				conditions.BgpConditions = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions{}
			}
			if conditions.BgpConditions.MatchEvpnSet == nil {
				conditions.BgpConditions.MatchEvpnSet = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions_MatchEvpnSet{}
			}
			if conditions.BgpConditions.MatchEvpnSet.Config == nil {
				conditions.BgpConditions.MatchEvpnSet.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions_MatchEvpnSet_Config{}
			}

			conditions.BgpConditions.MatchEvpnSet.Config.VniNumber = statement.Conditions.MatchEVPNVNI
		}
		if statement.Conditions.Call != nil {
			if conditions.Config == nil {
				conditions.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_Config{}
			}

			conditions.Config.CallPolicy = statement.Conditions.Call
		}
		if statement.Conditions.MatchSourceVRF != nil {
			if conditions.MatchSrcNetworkInstance == nil {
				conditions.MatchSrcNetworkInstance = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_MatchSrcNetworkInstance{}
			}
			if conditions.MatchSrcNetworkInstance.Config == nil {
				conditions.MatchSrcNetworkInstance.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_MatchSrcNetworkInstance_Config{}
			}

			conditions.MatchSrcNetworkInstance.Config.Name = statement.Conditions.MatchSourceVRF
		}

		result := oc.OpenconfigRoutingPolicy_PolicyResultType_REJECT_ROUTE
		if statement.Result == dozer.SpecRouteMapResultAccept {
			result = oc.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE
		}

		var bgpActions *oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions
		if len(statement.SetCommunities) > 0 {
			comms := []oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union{}
			for _, comm := range statement.SetCommunities {
				comms = append(comms, oc.UnionString(comm))
			}

			if bgpActions == nil {
				bgpActions = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions{}
			}

			bgpActions.SetCommunity = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity{
				Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config{
					Method:  oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE,
					Options: oc.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD,
				},
				Inline: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline{
					Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config{
						Communities: comms,
					},
				},
			}
		}
		if statement.SetLocalPreference != nil {
			if bgpActions == nil {
				bgpActions = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions{}
			}
			if bgpActions.Config == nil {
				bgpActions.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config{}
			}

			bgpActions.Config.SetLocalPref = statement.SetLocalPreference
		}

		statements := &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_OrderedMap{}
		statements.Append(&oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement{
			Name: pointer.To(seq),
			Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Config{
				Name: pointer.To(seq),
			},
			Conditions: conditions,
			Actions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions{
				BgpActions: bgpActions,
				Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_Config{
					PolicyResult: result,
				},
			},
		})

		return &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements{
			Statement: statements,
		}, nil
	},
}

func loadActualRouteMaps(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocRouteMaps := &oc.OpenconfigRoutingPolicy_RoutingPolicy{}
	err := client.Get(ctx, "/routing-policy/policy-definitions", ocRouteMaps, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read route maps")
	}
	spec.RouteMaps, err = unmarshalOCRouteMaps(ocRouteMaps)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal route maps")
	}

	return nil
}

func unmarshalOCRouteMaps(ocVal *oc.OpenconfigRoutingPolicy_RoutingPolicy) (map[string]*dozer.SpecRouteMap, error) {
	routeMaps := map[string]*dozer.SpecRouteMap{}

	if ocVal == nil || ocVal.PolicyDefinitions == nil {
		return routeMaps, nil
	}

	for name, ocRouteMap := range ocVal.PolicyDefinitions.PolicyDefinition {
		if ocRouteMap.Statements == nil || ocRouteMap.Statements.Statement == nil {
			continue
		}

		statements := map[string]*dozer.SpecRouteMapStatement{}

		for _, statement := range ocRouteMap.Statements.Statement.Values() {
			conditions := dozer.SpecRouteMapConditions{}
			if statement.Conditions != nil {
				if statement.Conditions.Config != nil && statement.Conditions.Config.InstallProtocolEq == oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED {
					conditions.DirectlyConnected = pointer.To(true)
				}
				if statement.Conditions.MatchPrefixSet != nil && statement.Conditions.MatchPrefixSet.Config != nil && statement.Conditions.MatchPrefixSet.Config.PrefixSet != nil {
					conditions.MatchPrefixList = statement.Conditions.MatchPrefixSet.Config.PrefixSet
				}
				if statement.Conditions.BgpConditions != nil {
					if statement.Conditions.BgpConditions.MatchEvpnSet != nil && statement.Conditions.BgpConditions.MatchEvpnSet.Config != nil {
						conditions.MatchEVPNDefaultRoute = statement.Conditions.BgpConditions.MatchEvpnSet.Config.DefaultType5Route
						conditions.MatchEVPNVNI = statement.Conditions.BgpConditions.MatchEvpnSet.Config.VniNumber
					}

					if statement.Conditions.BgpConditions.Config != nil {
						conditions.MatchCommunityList = statement.Conditions.BgpConditions.Config.CommunitySet
						conditions.MatchNextHopPrefixList = statement.Conditions.BgpConditions.Config.NextHopSet
					}
				}
				if statement.Conditions.Config != nil && statement.Conditions.Config.CallPolicy != nil {
					conditions.Call = statement.Conditions.Config.CallPolicy
				}
				if statement.Conditions.MatchSrcNetworkInstance != nil && statement.Conditions.MatchSrcNetworkInstance.Config != nil {
					conditions.MatchSourceVRF = statement.Conditions.MatchSrcNetworkInstance.Config.Name
				}
			}

			result := dozer.SpecRouteMapResultReject
			if statement.Actions == nil || statement.Actions.Config == nil {
				continue
			}
			if statement.Actions.Config.PolicyResult == oc.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE {
				result = dozer.SpecRouteMapResultAccept
			}

			var setComms []string
			var setLocalPref *uint32
			if statement.Actions.BgpActions != nil {
				if statement.Actions.BgpActions.SetCommunity != nil {
					setComm := statement.Actions.BgpActions.SetCommunity

					if setComm.Config != nil && setComm.Inline != nil && setComm.Inline.Config != nil {
						if setComm.Config.Method != oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE {
							slog.Warn("unsupported community set method", "route map", name, "method", setComm.Config.Method)
							continue
						}
						if setComm.Config.Options != oc.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD {
							slog.Warn("unsupported community set options", "route map", name, "options", setComm.Config.Options)
							continue
						}

						for _, comm := range setComm.Inline.Config.Communities {
							if str, ok := comm.(oc.UnionString); ok {
								setComms = append(setComms, string(str))
							} else {
								return nil, errors.Errorf("unexpected community member type: %T", comm)
							}
						}
					}
				}
				if statement.Actions.BgpActions.Config != nil && statement.Actions.BgpActions.Config.SetLocalPref != nil {
					setLocalPref = statement.Actions.BgpActions.Config.SetLocalPref
				}
			}

			statements[*statement.Name] = &dozer.SpecRouteMapStatement{
				Conditions:         conditions,
				SetCommunities:     setComms,
				SetLocalPreference: setLocalPref,
				Result:             result,
			}
		}

		routeMaps[name] = &dozer.SpecRouteMap{
			Statements: statements,
		}
	}

	return routeMaps, nil
}
