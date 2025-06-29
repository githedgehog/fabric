// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"context"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

var specECMPRoCEEnforcer = &DefaultValueEnforcer[string, *dozer.Spec]{
	Summary: "ECMP RoCE",
	Path:    "/loadshare/roce-attrs",
	Getter:  func(_ string, value *dozer.Spec) any { return value.ECMPRoCEQPN },
	Weight:  ActionWeightECMPRoCE,
	Marshal: func(_ string, value *dozer.Spec) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigLoadshareModeExt_Loadshare{
			RoceAttrs: &oc.OpenconfigLoadshareModeExt_Loadshare_RoceAttrs{
				Config: &oc.OpenconfigLoadshareModeExt_Loadshare_RoceAttrs_Config{
					Hash: pointer.To("hash"),
					Qpn:  value.ECMPRoCEQPN,
				},
			},
		}, nil
	},
}

func loadActualECMPRoCE(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocECMP := &oc.OpenconfigLoadshareModeExt_Loadshare{}
	err := client.Get(ctx, "/loadshare/roce-attrs", ocECMP, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read ecmp roce config")
	}
	spec.ECMPRoCEQPN, err = unmarshalOCECMPRoCEQPNConfig(ocECMP)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal ecmp roce config")
	}

	return nil
}

func unmarshalOCECMPRoCEQPNConfig(ocVal *oc.OpenconfigLoadshareModeExt_Loadshare) (*bool, error) {
	if ocVal == nil {
		return nil, errors.Errorf("no ECMP RoCE config found")
	}

	if ocVal.RoceAttrs != nil && ocVal.RoceAttrs.Config != nil && ocVal.RoceAttrs.Config.Qpn != nil {
		return ocVal.RoceAttrs.Config.Qpn, nil
	}

	return pointer.To(false), nil
}
