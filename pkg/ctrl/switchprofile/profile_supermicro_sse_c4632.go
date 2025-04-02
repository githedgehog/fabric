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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var SupermicroSSEC4632SB = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: "supermicro-sse-c4632sb",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Supermicro SSE-C4632SB",
		SwitchSilicon: CelesticaDS3000.Spec.SwitchSilicon,
		Features:      CelesticaDS3000.Spec.Features,
		NOSType:       CelesticaDS3000.Spec.NOSType,
		Platform:      CelesticaDS3000.Spec.Platform,
		Config:        CelesticaDS3000.Spec.Config,
		Ports:         CelesticaDS3000.Spec.Ports,
		PortGroups:    CelesticaDS3000.Spec.PortGroups,
		PortProfiles:  CelesticaDS3000.Spec.PortProfiles,
	},
}
