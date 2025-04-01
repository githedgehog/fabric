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
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var VS = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: meta.SwitchProfileVS,
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName: "Virtual Switch",
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			VXLAN:         true,
			ACLs:          false,
		},
		NOSType:  meta.NOSTypeSONiCBCMVS,
		Platform: "x86_64-kvm_x86_64-r0",
		Config: wiringapi.SwitchProfileConfig{
			MaxPathsEBGP: 16,
		},
		Ports:        DellS5248FON.Spec.Ports,
		PortGroups:   DellS5248FON.Spec.PortGroups,
		PortProfiles: DellS5248FON.Spec.PortProfiles,
	},
}
