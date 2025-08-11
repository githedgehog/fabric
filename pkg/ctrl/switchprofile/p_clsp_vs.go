// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"github.com/samber/lo"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var CLSPVS = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: "vs-clsp",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Virtual Switch CLS+",
		SwitchSilicon: SiliconVS,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          false,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          true,
			MCLAG:         true,
			ESLAG:         true,
			ECMPRoCEQPN:   false,
		},
		NOSType:  meta.NOSTypeSONiCCLSPlusVS,
		Platform: "x86_64-kvm_x86_64-r0",
		Config: wiringapi.SwitchProfileConfig{
			MaxPathsEBGP: 16,
		},
		// TODO update with actual values, it's obviously incorrect right now but it doesn't matter for the initial research
		Ports:        lo.OmitBy(DellS5248FON.Spec.Ports, func(_ string, p wiringapi.SwitchProfilePort) bool { return p.BaseNOSName != "" }),
		PortGroups:   DellS5248FON.Spec.PortGroups,
		PortProfiles: lo.OmitBy(DellS5248FON.Spec.PortProfiles, func(_ string, pp wiringapi.SwitchProfilePortProfile) bool { return pp.Breakout != nil }),
	},
}
