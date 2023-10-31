package bcm

import (
	"strings"

	"github.com/openconfig/ygot/ygot"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
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

	return ygot.String(speed)
}

func MarshalPortSpeed(speed string) (oc.E_OpenconfigIfEthernet_ETHERNET_SPEED, bool) {
	if !strings.HasPrefix(speed, "SPEED_") {
		speed = "SPEED_" + speed
	}
	if !strings.HasSuffix(speed, "B") {
		speed = speed + "B"
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

func UnmarshalPortBreakout(mode string) string {
	// not really needed right now, just in case
	mode = strings.TrimSuffix(mode, "B")

	return mode
}
