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

// TODO it's currently not capable of real updates but it's okay for now, we only set no advertise community
var specRouteMapEnforcer = &DefaultValueEnforcer[string, *dozer.SpecRouteMap]{
	Summary:      "Route Maps %s",
	Path:         "/routing-policy/policy-definitions[name=%s]",
	UpdateWeight: ActionWeightRouteMapUpdate,
	DeleteWeight: ActionWeightRouteMapDelete,
	Marshal: func(name string, value *dozer.SpecRouteMap) (ygot.ValidatedGoStruct, error) {
		communities := []oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union{}

		if value.NoAdvertise != nil && *value.NoAdvertise {
			communities = append(communities, oc.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NO_ADVERTISE)
		}

		statement := &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_OrderedMap{}
		statement.Append(&oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement{
			Name: ygot.String("10"),
			Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Config{
				Name: ygot.String("10"),
			},
			Actions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions{
				Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_Config{
					PolicyResult: oc.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE,
				},
				BgpActions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions{
					SetCommunity: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity{
						Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config{
							Method:  oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE,
							Options: oc.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD,
						},
						Inline: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline{
							Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config{
								Communities: communities,
							},
						},
					},
				},
			},
		})

		return &oc.OpenconfigRoutingPolicy_RoutingPolicy{
			PolicyDefinitions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions{
				PolicyDefinition: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition{
					name: {
						Name: ygot.String(name),
						Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Config{
							Name: ygot.String(name),
						},
						Statements: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements{
							Statement: statement,
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

		var noAdvertise *bool
		for _, statement := range ocRouteMap.Statements.Statement.Values() {
			if statement.Actions == nil || statement.Actions.Config == nil || statement.Actions.Config.PolicyResult != oc.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE {
				continue
			}
			if statement.Actions.BgpActions == nil || statement.Actions.BgpActions.SetCommunity == nil {
				continue
			}
			setCommunity := statement.Actions.BgpActions.SetCommunity
			if setCommunity.Inline == nil || setCommunity.Config == nil {
				continue
			}
			if setCommunity.Config.Method != oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE {
				continue
			}
			if setCommunity.Config.Options != oc.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD {
				continue
			}
			if setCommunity.Inline.Config == nil {
				continue
			}

			for _, community := range setCommunity.Inline.Config.Communities {
				if community == oc.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NO_ADVERTISE {
					noAdvertise = ygot.Bool(true)
				}
			}
		}

		routeMaps[name] = &dozer.SpecRouteMap{
			NoAdvertise: noAdvertise,
		}
	}

	return routeMaps, nil
}
