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

var specRouteMapsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecRouteMap]{
	Summary:      "Route Map",
	ValueHandler: specRouteMapEnforcer,
}

// TODO it's currently not capable of real updates but it's okay for now, we only set simple ones
var specRouteMapEnforcer = &DefaultValueEnforcer[string, *dozer.SpecRouteMap]{
	Summary:      "Route Maps %s",
	Path:         "/routing-policy/policy-definitions[name=%s]",
	UpdateWeight: ActionWeightRouteMapUpdate,
	DeleteWeight: ActionWeightRouteMapDelete,
	Marshal: func(name string, value *dozer.SpecRouteMap) (ygot.ValidatedGoStruct, error) {
		statements := &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_OrderedMap{}

		for seq, statement := range value.Statements {
			conditions := &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions{}
			ok := false
			if statement.Conditions.DirectlyConnected != nil && *statement.Conditions.DirectlyConnected {
				if conditions.Config == nil {
					conditions.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_Config{}
				}

				conditions.Config.InstallProtocolEq = oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED
				ok = true
			}
			if statement.Conditions.MatchPrefixList != nil {
				if conditions.MatchPrefixSet == nil {
					conditions.MatchPrefixSet = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_MatchPrefixSet{}
				}
				if conditions.MatchPrefixSet.Config == nil {
					conditions.MatchPrefixSet.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_MatchPrefixSet_Config{}
				}

				conditions.MatchPrefixSet.Config.PrefixSet = statement.Conditions.MatchPrefixList
				ok = true
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

				conditions.BgpConditions.MatchEvpnSet.Config.DefaultType5Route = ygot.Bool(true)
				ok = true
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
				ok = true
			}
			if statement.Conditions.Call != nil {
				if conditions.Config == nil {
					conditions.Config = &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_Config{}
				}

				conditions.Config.CallPolicy = statement.Conditions.Call
				ok = true
			}
			if !ok {
				conditions = nil
			}

			result := oc.OpenconfigRoutingPolicy_PolicyResultType_REJECT_ROUTE
			if statement.Result == dozer.SpecRouteMapResultAccept {
				result = oc.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE
			}

			statements.Append(&oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement{
				Name: ygot.String(seq),
				Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Config{
					Name: ygot.String(seq),
				},
				Conditions: conditions,
				Actions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions{
					Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_Config{
						PolicyResult: result,
					},
				},
			})
		}

		return &oc.OpenconfigRoutingPolicy_RoutingPolicy{
			PolicyDefinitions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions{
				PolicyDefinition: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition{
					name: {
						Name: ygot.String(name),
						Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Config{
							Name: ygot.String(name),
						},
						Statements: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements{
							Statement: statements,
						},
					},
				},
			},
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
					conditions.DirectlyConnected = ygot.Bool(true)
				}
				if statement.Conditions.MatchPrefixSet != nil && statement.Conditions.MatchPrefixSet.Config != nil && statement.Conditions.MatchPrefixSet.Config.PrefixSet != nil {
					conditions.MatchPrefixList = statement.Conditions.MatchPrefixSet.Config.PrefixSet
				}
				if statement.Conditions.BgpConditions != nil && statement.Conditions.BgpConditions.MatchEvpnSet != nil && statement.Conditions.BgpConditions.MatchEvpnSet.Config != nil {
					conditions.MatchEVPNDefaultRoute = statement.Conditions.BgpConditions.MatchEvpnSet.Config.DefaultType5Route
					conditions.MatchEVPNVNI = statement.Conditions.BgpConditions.MatchEvpnSet.Config.VniNumber
				}
				if statement.Conditions.Config != nil && statement.Conditions.Config.CallPolicy != nil {
					conditions.Call = statement.Conditions.Config.CallPolicy
				}
			}

			result := dozer.SpecRouteMapResultReject
			if statement.Actions == nil || statement.Actions.Config == nil {
				continue
			}
			if statement.Actions.Config.PolicyResult == oc.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE {
				result = dozer.SpecRouteMapResultAccept
			}

			statements[*statement.Name] = &dozer.SpecRouteMapStatement{
				Conditions: conditions,
				Result:     result,
			}
		}

		routeMaps[name] = &dozer.SpecRouteMap{
			Statements: statements,
		}
	}

	return routeMaps, nil
}
