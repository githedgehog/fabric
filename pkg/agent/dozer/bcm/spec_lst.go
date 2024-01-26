package bcm

import (
	"context"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

var specLSTGroupsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecLSTGroup]{
	Summary:      "SpecLSTGroups",
	ValueHandler: specLSTGroupEnforcer,
}

var specLSTGroupEnforcer = &DefaultValueEnforcer[string, *dozer.SpecLSTGroup]{
	Summary:      "LST Group %s",
	Path:         "/lst/lst-groups/lst-group[name=%s]",
	CreatePath:   "/lst/lst-groups/lst-group",
	UpdateWeight: ActionWeightLSTGroupUpdate,
	DeleteWeight: ActionWeightLSTGroupDelete,
	Marshal: func(name string, value *dozer.SpecLSTGroup) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigLstExt_Lst_LstGroups{
			LstGroup: map[string]*oc.OpenconfigLstExt_Lst_LstGroups_LstGroup{
				name: {
					Name: stringPtr(name),
					Config: &oc.OpenconfigLstExt_Lst_LstGroups_LstGroup_Config{
						Name:                stringPtr(name),
						AllEvpnEsDownstream: value.AllEVPNESDownstream,
						Timeout:             value.Timeout,
					},
				},
			},
		}, nil
	},
}

func loadActualLSTGroups(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocLST := &oc.OpenconfigLstExt_Lst{}
	err := client.Get(ctx, "/lst/lst-groups", ocLST)
	if err != nil {
		return errors.Wrapf(err, "failed to get lst groups")
	}

	spec.LSTGroups, err = unmarshalActualLSTGroups(ocLST.LstGroups)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal lst groups")
	}

	return nil
}

func unmarshalActualLSTGroups(ocVal *oc.OpenconfigLstExt_Lst_LstGroups) (map[string]*dozer.SpecLSTGroup, error) {
	lstGroups := map[string]*dozer.SpecLSTGroup{}

	if ocVal == nil {
		return lstGroups, nil
	}

	for name, ocLSTGroup := range ocVal.LstGroup {
		if ocLSTGroup.Config == nil {
			continue
		}

		lstGroups[name] = &dozer.SpecLSTGroup{
			AllEVPNESDownstream: ocLSTGroup.Config.AllEvpnEsDownstream,
			Timeout:             ocLSTGroup.Config.Timeout,
		}
	}

	return lstGroups, nil
}

var specLSTInterfacesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecLSTInterface]{
	Summary:      "LSTInterfaces",
	ValueHandler: specLSTInterfaceEnforcer,
}

var specLSTInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecLSTInterface]{
	Summary:      "LST Interface %s",
	Path:         "/lst/interfaces/interface[id=%s]",
	UpdateWeight: ActionWeightLSTInterfaceUpdate,
	DeleteWeight: ActionWeightLSTInterfaceDelete,
	Marshal: func(id string, value *dozer.SpecLSTInterface) (ygot.ValidatedGoStruct, error) {
		groups := map[string]*oc.OpenconfigLstExt_Lst_Interfaces_Interface_UpstreamGroups_UpstreamGroup{}
		for _, group := range value.Groups {
			groups[group] = &oc.OpenconfigLstExt_Lst_Interfaces_Interface_UpstreamGroups_UpstreamGroup{
				GroupName: stringPtr(group),
				Config: &oc.OpenconfigLstExt_Lst_Interfaces_Interface_UpstreamGroups_UpstreamGroup_Config{
					GroupName: stringPtr(group),
				},
			}
		}

		return &oc.OpenconfigLstExt_Lst_Interfaces{
			Interface: map[string]*oc.OpenconfigLstExt_Lst_Interfaces_Interface{
				id: {
					Id: stringPtr(id),
					Config: &oc.OpenconfigLstExt_Lst_Interfaces_Interface_Config{
						Id: stringPtr(id),
					},
					InterfaceRef: &oc.OpenconfigLstExt_Lst_Interfaces_Interface_InterfaceRef{
						Config: &oc.OpenconfigLstExt_Lst_Interfaces_Interface_InterfaceRef_Config{
							Interface: stringPtr(id),
						},
					},
					UpstreamGroups: &oc.OpenconfigLstExt_Lst_Interfaces_Interface_UpstreamGroups{
						UpstreamGroup: groups,
					},
				},
			},
		}, nil
	},
}

func loadActualLSTInterfaces(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocLST := &oc.OpenconfigLstExt_Lst{}
	err := client.Get(ctx, "/lst/interfaces", ocLST)
	if err != nil {
		return errors.Wrapf(err, "failed to get lst interfaces")
	}

	spec.LSTInterfaces, err = unmarshalActualLSTInterfaces(ocLST.Interfaces)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal lst interfaces")
	}

	return nil
}

func unmarshalActualLSTInterfaces(ocVal *oc.OpenconfigLstExt_Lst_Interfaces) (map[string]*dozer.SpecLSTInterface, error) {
	lstIfaces := map[string]*dozer.SpecLSTInterface{}

	if ocVal == nil {
		return lstIfaces, nil
	}

	for name, ocLSTInterface := range ocVal.Interface {
		if ocLSTInterface.InterfaceRef == nil || ocLSTInterface.InterfaceRef.Config == nil {
			continue
		}
		if ocLSTInterface.UpstreamGroups == nil {
			continue
		}

		groups := []string{}
		for groupName := range ocLSTInterface.UpstreamGroups.UpstreamGroup {
			groups = append(groups, groupName)
		}

		lstIfaces[name] = &dozer.SpecLSTInterface{
			Groups: groups,
		}
	}

	return lstIfaces, nil
}
