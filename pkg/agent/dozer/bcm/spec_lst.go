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

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
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
					Name: pointer.To(name),
					Config: &oc.OpenconfigLstExt_Lst_LstGroups_LstGroup_Config{
						Name:                pointer.To(name),
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

func unmarshalActualLSTGroups(ocVal *oc.OpenconfigLstExt_Lst_LstGroups) (map[string]*dozer.SpecLSTGroup, error) { //nolint:unparam
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
				GroupName: pointer.To(group),
				Config: &oc.OpenconfigLstExt_Lst_Interfaces_Interface_UpstreamGroups_UpstreamGroup_Config{
					GroupName: pointer.To(group),
				},
			}
		}

		return &oc.OpenconfigLstExt_Lst_Interfaces{
			Interface: map[string]*oc.OpenconfigLstExt_Lst_Interfaces_Interface{
				id: {
					Id: pointer.To(id),
					Config: &oc.OpenconfigLstExt_Lst_Interfaces_Interface_Config{
						Id: pointer.To(id),
					},
					InterfaceRef: &oc.OpenconfigLstExt_Lst_Interfaces_Interface_InterfaceRef{
						Config: &oc.OpenconfigLstExt_Lst_Interfaces_Interface_InterfaceRef_Config{
							Interface: pointer.To(id),
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

func unmarshalActualLSTInterfaces(ocVal *oc.OpenconfigLstExt_Lst_Interfaces) (map[string]*dozer.SpecLSTInterface, error) { //nolint:unparam
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
