package bcm

import (
	"context"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

var specLLDPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecLLDP]{
	Summary: "LLDP",
	Path:    "/lldp/config",
	Weight:  ActionWeightLLDP,
	Marshal: func(name string, value *dozer.SpecLLDP) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigLldp_Lldp{
			Config: &oc.OpenconfigLldp_Lldp_Config{
				Enabled:           value.Enabled,
				HelloTimer:        value.HelloTimer,
				SystemName:        value.SystemName,
				SystemDescription: value.SystemDescription,
			},
		}, nil
	},
}

func loadActualLLDP(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocLLDP := &oc.OpenconfigLldp_Lldp{}
	err := client.Get(ctx, "/lldp/config", ocLLDP)
	if err != nil {
		return errors.Wrapf(err, "failed to get lldp")
	}

	spec.LLDP, err = unmarshalActualLLDP(ocLLDP)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal lldp")
	}

	return nil
}

func unmarshalActualLLDP(ocVal *oc.OpenconfigLldp_Lldp) (*dozer.SpecLLDP, error) {
	lldp := &dozer.SpecLLDP{}

	if ocVal == nil {
		return lldp, nil
	}

	if ocVal.Config != nil {
		lldp.Enabled = ocVal.Config.Enabled
		lldp.HelloTimer = ocVal.Config.HelloTimer
		lldp.SystemName = ocVal.Config.SystemName
		lldp.SystemDescription = ocVal.Config.SystemDescription
	}

	return lldp, nil
}

var specLLDPInterfacesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecLLDPInterface]{
	Summary:      "LLDP Interfaces",
	ValueHandler: specLLDPInterfaceEnforcer,
}

var specLLDPInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecLLDPInterface]{
	Summary:      "LLDP Interface %s",
	CreatePath:   "/lldp/interfaces/interface",
	Path:         "/lldp/interfaces/interface[name=%s]",
	UpdateWeight: ActionWeightLLDPInterfaceUpdate,
	DeleteWeight: ActionWeightLLDPInterfaceDelete,
	Marshal: func(name string, value *dozer.SpecLLDPInterface) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigLldp_Lldp_Interfaces{
			Interface: map[string]*oc.OpenconfigLldp_Lldp_Interfaces_Interface{
				name: {
					Name: ygot.String(name),
					Config: &oc.OpenconfigLldp_Lldp_Interfaces_Interface_Config{
						Enabled:               value.Enabled,
						ManagementAddressIpv4: value.ManagementIPv4,
						// TODO do we need mode?
					},
				},
			},
		}, nil
	},
}

func loadActualLLDPInterfaces(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocLLDP := &oc.OpenconfigLldp_Lldp{}
	err := client.Get(ctx, "/lldp/interfaces", ocLLDP)
	if err != nil {
		return errors.Wrapf(err, "failed to get lldp interfaces")
	}

	spec.LLDPInterfaces, err = unmarshalActualLLDPInterfaces(ocLLDP.Interfaces)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal lldp interfaces")
	}

	return nil
}

func unmarshalActualLLDPInterfaces(ocVal *oc.OpenconfigLldp_Lldp_Interfaces) (map[string]*dozer.SpecLLDPInterface, error) {
	lldpInterfaces := map[string]*dozer.SpecLLDPInterface{}

	if ocVal == nil {
		return lldpInterfaces, nil
	}

	for name, ocInterface := range ocVal.Interface {
		if ocInterface.Config == nil {
			continue
		}

		lldpInterfaces[name] = &dozer.SpecLLDPInterface{
			Enabled:        ocInterface.Config.Enabled,
			ManagementIPv4: ocInterface.Config.ManagementAddressIpv4,
		}
	}

	return lldpInterfaces, nil
}
