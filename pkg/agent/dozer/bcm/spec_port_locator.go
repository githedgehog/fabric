// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"context"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

const portLocatorsPath = "/openconfig-system:system/openconfig-system-beacon-led:port-locators"

var specPortLocatorsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecPortLocator]{
	Summary:      "Port Locators",
	ValueHandler: specPortLocatorEnforcer,
}

var specPortLocatorEnforcer = &DefaultValueEnforcer[string, *dozer.SpecPortLocator]{
	Summary:      "Port Locator %s",
	Path:         portLocatorsPath + "/port-locator[ifname=%s]",
	CreatePath:   portLocatorsPath + "/port-locator",
	UpdateWeight: ActionWeightPortLocatorUpdate,
	DeleteWeight: ActionWeightPortLocatorDelete,
	Marshal: func(name string, value *dozer.SpecPortLocator) (ygot.ValidatedGoStruct, error) {
		mode := oc.OpenconfigSystemBeaconLed_BeaconLedPortLocatorMode_OFF
		if value.Enabled != nil && *value.Enabled {
			mode = oc.OpenconfigSystemBeaconLed_BeaconLedPortLocatorMode_ON
		}

		return &oc.OpenconfigSystem_System_PortLocators{
			PortLocator: map[string]*oc.OpenconfigSystem_System_PortLocators_PortLocator{
				name: {
					Ifname: new(name),
					Config: &oc.OpenconfigSystem_System_PortLocators_PortLocator_Config{
						Ifname:     new(name),
						Mode:       mode,
						ExpireTime: value.Expire,
					},
				},
			},
		}, nil
	},
}

func loadActualPortLocators(ctx context.Context, client GNMICClient, spec *dozer.Spec) error {
	ocSystem := &oc.OpenconfigSystem_System{}
	if err := client.Get(ctx, portLocatorsPath, ocSystem); err != nil {
		return errors.Wrapf(err, "failed to get port locators")
	}

	spec.PortLocators = unmarshalActualPortLocators(ocSystem.PortLocators)

	return nil
}

func unmarshalActualPortLocators(ocVal *oc.OpenconfigSystem_System_PortLocators) map[string]*dozer.SpecPortLocator {
	portLocators := map[string]*dozer.SpecPortLocator{}

	if ocVal == nil {
		return portLocators
	}

	for name, ocPortLocator := range ocVal.PortLocator {
		if ocPortLocator.Config == nil {
			continue
		}

		// the device reports an entry for every port it knows about, only the ones with the
		// locator LED actually on are a part of the config we manage
		if ocPortLocator.Config.Mode != oc.OpenconfigSystemBeaconLed_BeaconLedPortLocatorMode_ON {
			continue
		}

		portLocators[name] = &dozer.SpecPortLocator{
			Enabled: new(true),
			Expire:  ocPortLocator.Config.ExpireTime,
		}
	}

	return portLocators
}
