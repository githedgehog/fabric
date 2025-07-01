// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

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
