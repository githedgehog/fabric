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
