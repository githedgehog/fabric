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

var profileEdgecoreEPS203 = wiringapi.SwitchProfile{
	ObjectMeta: metav1.ObjectMeta{
		Name: "edgecore-eps203",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName: "Edgecore EPS203",
		OtherNames:  []string{"Edgecore AS4630-54NPE"},
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: false, // TODO(sergeymatov) verify
			VXLAN:         true,
			ACLs:          true,
		},
		Config: wiringapi.SwitchProfileConfig{
			MaxPathsEBGP: 16,
		},
		// Ports:        map[string]wiringapi.SwitchProfilePort{},
		// PortGroups:   map[string]wiringapi.SwitchProfilePortGroup{},
		// PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{},
	},
}
