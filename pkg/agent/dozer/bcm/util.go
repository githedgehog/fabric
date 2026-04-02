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

package bcm

import (
	"strings"

	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

func UnmarshalPortSpeed(speedRaw oc.E_OpenconfigIfEthernet_ETHERNET_SPEED) *string {
	speed := ""
	if speedRaw > oc.OpenconfigIfEthernet_ETHERNET_SPEED_UNSET && speedRaw < oc.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_UNKNOWN {
		speed = oc.ΛEnum["E_OpenconfigIfEthernet_ETHERNET_SPEED"][int64(speedRaw)].Name
	} else {
		return nil
	}

	speed = strings.TrimPrefix(speed, "SPEED_")
	speed = strings.TrimSuffix(speed, "B")

	if speed == "2500M" {
		speed = "2.5G"
	}

	return pointer.To(speed)
}

func MarshalPortSpeed(speed string) (oc.E_OpenconfigIfEthernet_ETHERNET_SPEED, bool) {
	if speed == "2.5G" {
		speed = "2500M"
	}

	if !strings.HasPrefix(speed, "SPEED_") {
		speed = "SPEED_" + speed
	}
	if !strings.HasSuffix(speed, "B") {
		speed += "B"
	}
	res := oc.OpenconfigIfEthernet_ETHERNET_SPEED_UNSET

	ok := false
	for speedVal, name := range oc.ΛEnum["E_OpenconfigIfEthernet_ETHERNET_SPEED"] {
		if name.Name == speed {
			res = oc.E_OpenconfigIfEthernet_ETHERNET_SPEED(speedVal)
			ok = true

			break
		}
	}

	return res, ok
}

func MarshalPortFEC(fec string) (oc.E_OpenconfigPlatformTypes_FEC_MODE_TYPE, bool) {
	switch wiringapi.PortFECMode(fec) {
	case wiringapi.PortFECModeRS:
		return oc.OpenconfigPlatformTypes_FEC_MODE_TYPE_FEC_RS, true
	case wiringapi.PortFECModeFC:
		return oc.OpenconfigPlatformTypes_FEC_MODE_TYPE_FEC_FC, true
	case wiringapi.PortFECModeAuto:
		return oc.OpenconfigPlatformTypes_FEC_MODE_TYPE_FEC_AUTO, true
	case wiringapi.PortFECModeDisabled:
		return oc.OpenconfigPlatformTypes_FEC_MODE_TYPE_FEC_DISABLED, true
	default:
		return oc.OpenconfigPlatformTypes_FEC_MODE_TYPE_UNSET, false
	}
}

func UnmarshalPortBreakout(mode string) string {
	// not really needed right now, just in case
	mode = strings.TrimSuffix(mode, "B")

	return mode
}
