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
	"strings"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

var specDHCPRelaysEnforcer = &DefaultMapEnforcer[string, *dozer.SpecDHCPRelay]{
	Summary:      "DHCP relays",
	ValueHandler: specDHCPRelayEnforcer,
}

var specDHCPRelayEnforcer = &DefaultValueEnforcer[string, *dozer.SpecDHCPRelay]{
	Summary:      "DHCP relay %s",
	Path:         "relay-agent/dhcp/interfaces/interface[id=%s]",
	UpdateWeight: ActionWeightDHCPRelayUpdate,
	DeleteWeight: ActionWeightDHCPRelayDelete,
	Marshal: func(name string, value *dozer.SpecDHCPRelay) (ygot.ValidatedGoStruct, error) {
		linkSelect := oc.OpenconfigRelayAgentExt_Mode_UNSET
		if value.LinkSelect {
			linkSelect = oc.OpenconfigRelayAgentExt_Mode_ENABLE
		}

		vrfSelect := oc.OpenconfigRelayAgentExt_Mode_UNSET
		if value.VRFSelect {
			vrfSelect = oc.OpenconfigRelayAgentExt_Mode_ENABLE
		}

		return &oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces{
			Interface: map[string]*oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface{
				name: {
					Id: ygot.String(name),
					AgentInformationOption: &oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_AgentInformationOption{
						Config: &oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_AgentInformationOption_Config{
							LinkSelect: linkSelect,
							VrfSelect:  vrfSelect,
						},
					},
					Config: &oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_Config{
						HelperAddress: value.RelayAddress,
						SrcIntf:       value.SourceInterface,
					},
				},
			},
		}, nil
	},
}

func loadActualDHCPRelays(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocDHCPRelayInterfaces := &oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces{}
	err := client.Get(ctx, "/relay-agent/dhcp/interfaces/interface", ocDHCPRelayInterfaces, api.DataTypeCONFIG())
	if err != nil {
		if !strings.Contains(err.Error(), "rpc error: code = NotFound") { // TODO rework client to handle it
			return errors.Wrapf(err, "failed to read dhcp relay interfaces")
		}
	}

	spec.DHCPRelays, err = unmarshalOCDHCPRelays(ocDHCPRelayInterfaces)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal dhcp relay interfaces")
	}

	return nil
}

func unmarshalOCDHCPRelays(ocVal *oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces) (map[string]*dozer.SpecDHCPRelay, error) {
	relays := map[string]*dozer.SpecDHCPRelay{}

	if ocVal == nil {
		return relays, nil
	}

	for name, ocRelayIface := range ocVal.Interface {
		if ocRelayIface.AgentInformationOption == nil || ocRelayIface.AgentInformationOption.Config == nil {
			continue
		}
		cfg := ocRelayIface.AgentInformationOption.Config
		if ocRelayIface.Config == nil {
			continue
		}

		relays[name] = &dozer.SpecDHCPRelay{
			SourceInterface: ocRelayIface.Config.SrcIntf,
			RelayAddress:    ocRelayIface.Config.HelperAddress,
			LinkSelect:      cfg.LinkSelect == oc.OpenconfigRelayAgentExt_Mode_ENABLE,
			VRFSelect:       cfg.VrfSelect == oc.OpenconfigRelayAgentExt_Mode_ENABLE,
		}
	}

	return relays, nil
}
