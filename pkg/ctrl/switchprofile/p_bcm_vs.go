// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"github.com/samber/lo"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var VS = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: meta.SwitchProfileVS,
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Virtual Switch",
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
		NOSType:  meta.NOSTypeSONiCBCMVS,
		Platform: "x86_64-kvm_x86_64-r0",
		Config: wiringapi.SwitchProfileConfig{
			MaxPathsEBGP: 16,
		},
		Ports:        lo.OmitBy(DellS5248FON.Spec.Ports, func(_ string, p wiringapi.SwitchProfilePort) bool { return p.BaseNOSName != "" }),
		PortGroups:   DellS5248FON.Spec.PortGroups,
		PortProfiles: lo.OmitBy(DellS5248FON.Spec.PortProfiles, func(_ string, pp wiringapi.SwitchProfilePortProfile) bool { return pp.Breakout != nil }),
	},
}

// Breakout ports are not supported on VS and should be ignored
var vsIgnoredNOSPorts = map[string]bool{
	"Ethernet48": true,
	"Ethernet52": true,
	"Ethernet56": true,
	"Ethernet60": true,
	"Ethernet64": true,
	"Ethernet68": true,
	"Ethernet72": true,
	"Ethernet76": true,
}

func VSIsIgnoredNOSPort(port string) bool {
	_, ok := vsIgnoredNOSPorts[port]

	return ok
}

// Breakout ports are not supported on VS and should be ignored
var vsIgnoredNOSComponents = map[string]bool{
	"1/49": true,
	"1/50": true,
	"1/51": true,
	"1/52": true,
	"1/53": true,
	"1/54": true,
	"1/55": true,
	"1/56": true,
}

func VSIsIgnoredComponent(name string) bool {
	_, ok := vsIgnoredNOSComponents[name]

	return ok
}
