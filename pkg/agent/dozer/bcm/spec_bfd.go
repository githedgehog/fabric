// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

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

var specBFDProfilesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecBFDProfile]{
	Summary:      "BFD Profiles",
	ValueHandler: specBFDProfileEnforcer,
}

var specBFDProfileEnforcer = &DefaultValueEnforcer[string, *dozer.SpecBFDProfile]{
	Summary:      "BFD Profile %s",
	CreatePath:   "/openconfig-bfd:bfd/openconfig-bfd-ext:bfd-profile/profile",
	Path:         "/openconfig-bfd:bfd/openconfig-bfd-ext:bfd-profile/profile[profile-name=%s]",
	UpdateWeight: ActionWeightBFDProfileUpdate,
	DeleteWeight: ActionWeightBFDProfileDelete,
	Marshal: func(name string, value *dozer.SpecBFDProfile) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigBfd_Bfd_BfdProfile{
			Profile: map[string]*oc.OpenconfigBfd_Bfd_BfdProfile_Profile{
				name: {
					ProfileName: pointer.To(name),
					Config: &oc.OpenconfigBfd_Bfd_BfdProfile_Profile_Config{
						Enabled:                  pointer.To(true),
						ProfileName:              pointer.To(name),
						PassiveMode:              value.PassiveMode,
						DetectionMultiplier:      value.DetectionMultiplier,
						DesiredMinimumTxInterval: value.DesiredMinimumTxInterval,
						RequiredMinimumReceive:   value.RequiredMinimumReceive,
					},
				},
			},
		}, nil
	},
}

func loadActualBFDProfiles(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocBFD := &oc.OpenconfigBfd_Bfd{}
	err := client.Get(ctx, "/openconfig-bfd:bfd/openconfig-bfd-ext:bfd-profile", ocBFD)
	if err != nil {
		return errors.Wrapf(err, "failed to get bfd profiles")
	}

	spec.BFDProfiles, err = unmarshalActualBFDProfiles(ocBFD.BfdProfile)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal bfd profiles")
	}

	return nil
}

func unmarshalActualBFDProfiles(ocVal *oc.OpenconfigBfd_Bfd_BfdProfile) (map[string]*dozer.SpecBFDProfile, error) { //nolint:unparam
	bfdProfiles := map[string]*dozer.SpecBFDProfile{}

	if ocVal == nil {
		return bfdProfiles, nil
	}

	for name, profile := range ocVal.Profile {
		if profile.Config == nil || profile.Config.Enabled == nil || !*profile.Config.Enabled {
			continue
		}

		bfdProfiles[name] = &dozer.SpecBFDProfile{
			PassiveMode:              profile.Config.PassiveMode,
			DetectionMultiplier:      profile.Config.DetectionMultiplier,
			DesiredMinimumTxInterval: profile.Config.DesiredMinimumTxInterval,
			RequiredMinimumReceive:   profile.Config.RequiredMinimumReceive,
		}
	}

	return bfdProfiles, nil
}
