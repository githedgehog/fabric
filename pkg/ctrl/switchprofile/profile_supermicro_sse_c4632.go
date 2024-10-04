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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var profileSupermicroSSEC4632SB = wiringapi.SwitchProfile{
	ObjectMeta: metav1.ObjectMeta{
		Name: "supermicro-sse-c4632sb",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName: "Supermicro SSE-C4632SB",
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			VXLAN:         true,
			ACLs:          true,
		},
		NOSType:      profileCelesticaDS3000.Spec.NOSType,
		Config:       profileCelesticaDS3000.Spec.Config,
		Ports:        profileCelesticaDS3000.Spec.Ports,
		PortGroups:   profileCelesticaDS3000.Spec.PortGroups,
		PortProfiles: profileCelesticaDS3000.Spec.PortProfiles,
	},
}
