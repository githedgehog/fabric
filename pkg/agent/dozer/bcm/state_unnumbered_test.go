// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The device reports an unnumbered BGP session, and the BFD session under it, keyed by the NOS
// interface it runs over. Those names never leave the switch, so the agent reports the API port
// name instead.
func TestAPIIfaceName(t *testing.T) {
	portMap := map[string]string{
		"Ethernet0": "E1/1",
		"Ethernet4": "E1/5",
	}
	th5VLANs := map[string]uint16{"E1/5": 3123}

	for _, tt := range []struct {
		in   string
		want string
	}{
		{in: "Ethernet0", want: "E1/1"},
		{in: "Ethernet0.1001", want: "E1/1.1001"}, // hostBGP subinterface
		{in: "Vlan3123", want: "E1/5.3123"},       // TH5 workaround SVI
		{in: "Vlan1001", want: "Vlan1001"},        // not a workaround VLAN, nothing to map it to
		{in: "PortChannel12", want: "PortChannel12"},
		{in: "Ethernet96", want: "Ethernet96"}, // no mapping, kept as is rather than dropped
		{in: "172.30.128.1", want: "172.30.128.1"},
		{in: "fe80::1", want: "fe80::1"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, apiIfaceName(tt.in, portMap, th5VLANs))
		})
	}
}
