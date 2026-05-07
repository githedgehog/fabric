// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"context"
	"slices"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

const errDisableLinkFlapEnabled = "on"

var specErrDisableGlobalEnforcer = &DefaultValueEnforcer[string, *dozer.SpecErrDisableGlobal]{
	Summary:      "ErrDisable Global",
	Path:         "/openconfig-errdisable-ext:errdisable/config",
	UpdateWeight: ActionWeightErrDisableGlobalUpdate,
	DeleteWeight: ActionWeightErrDisableGlobalDelete,
	Marshal: func(_ string, value *dozer.SpecErrDisableGlobal) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigErrdisableExt_Errdisable{
			Config: &oc.OpenconfigErrdisableExt_Errdisable_Config{
				Cause: []oc.E_OpenconfigErrdisableTypes_ERRDISABLE_RECOVERY_CAUSE{
					oc.OpenconfigErrdisableTypes_ERRDISABLE_RECOVERY_CAUSE_LINK_FLAP,
				},
				Interval: value.RecoveryInterval,
			},
		}, nil
	},
}

var specErrDisableInterfacesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecErrDisable]{
	Summary:      "ErrDisable Interfaces",
	ValueHandler: specErrDisableInterfaceEnforcer,
}

var specErrDisableInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecErrDisable]{
	Summary:      "ErrDisable Interface %s",
	Path:         "/openconfig-errdisable-ext:errdisable-port/port[name=%s]/link-flap",
	UpdateWeight: ActionWeightErrDisablePortUpdate,
	DeleteWeight: ActionWeightErrDisablePortDelete,
	Marshal: func(_ string, value *dozer.SpecErrDisable) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigErrdisableExt_ErrdisablePort_Port{
			LinkFlap: &oc.OpenconfigErrdisableExt_ErrdisablePort_Port_LinkFlap{
				Config: &oc.OpenconfigErrdisableExt_ErrdisablePort_Port_LinkFlap_Config{
					ErrorDisable:     pointer.To(errDisableLinkFlapEnabled),
					FlapThreshold:    value.FlapThreshold,
					SamplingInterval: value.SamplingInterval,
					RecoveryInterval: value.RecoveryInterval,
				},
			},
		}, nil
	},
}

func loadActualErrDisableGlobal(ctx context.Context, client GNMICClient, spec *dozer.Spec) error {
	ocErrdisable := &oc.OpenconfigErrdisableExt_Errdisable{}
	err := client.Get(ctx, "/openconfig-errdisable-ext:errdisable/config", ocErrdisable)
	if err != nil {
		return errors.Wrapf(err, "failed to get global errdisable config")
	}

	spec.ErrDisableGlobal = unmarshalActualErrDisableGlobal(ocErrdisable)

	return nil
}

func unmarshalActualErrDisableGlobal(ocVal *oc.OpenconfigErrdisableExt_Errdisable) *dozer.SpecErrDisableGlobal {
	if ocVal == nil || ocVal.Config == nil {
		return nil
	}
	if !slices.Contains(ocVal.Config.Cause, oc.OpenconfigErrdisableTypes_ERRDISABLE_RECOVERY_CAUSE_LINK_FLAP) {
		return nil
	}

	return &dozer.SpecErrDisableGlobal{
		RecoveryInterval: ocVal.Config.Interval,
	}
}

func loadActualErrDisableInterfaces(ctx context.Context, client GNMICClient, spec *dozer.Spec) error {
	ocErrDisable := &oc.OpenconfigErrdisableExt_ErrdisablePort{}
	err := client.Get(ctx, "/openconfig-errdisable-ext:errdisable-port/port", ocErrDisable)
	if err != nil {
		return errors.Wrapf(err, "failed to get errdisable port config")
	}

	spec.ErrDisableInterfaces, err = unmarshalActualErrDisableInterfaces(ocErrDisable)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal errdisable port config")
	}

	return nil
}

func unmarshalActualErrDisableInterfaces(ocVal *oc.OpenconfigErrdisableExt_ErrdisablePort) (map[string]*dozer.SpecErrDisable, error) { //nolint:unparam
	result := map[string]*dozer.SpecErrDisable{}

	if ocVal == nil {
		return result, nil
	}

	for name, port := range ocVal.Port {
		if port.LinkFlap == nil || port.LinkFlap.Config == nil {
			continue
		}
		cfg := port.LinkFlap.Config
		if cfg.ErrorDisable == nil || *cfg.ErrorDisable != errDisableLinkFlapEnabled {
			continue
		}
		result[name] = &dozer.SpecErrDisable{
			FlapThreshold:    cfg.FlapThreshold,
			SamplingInterval: cfg.SamplingInterval,
			RecoveryInterval: cfg.RecoveryInterval,
		}
	}

	return result, nil
}
