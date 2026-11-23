// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var transceiverCaptures = map[string][]string{
	modelSN5610: {"/platform/transceiver?view=detail"},
	modelSN4700: {"/platform/transceiver?view=detail"},
	modelSN4600: {"/platform/transceiver?view=detail"},
	modelSN2700: {"/platform/transceiver?view=detail"},
	modelSN2201: {"/platform/transceiver?view=detail"},
}

func TestTransceivers(t *testing.T) {
	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		transceivers, err := client.Transceivers(t.Context())
		require.NoError(t, err)
		require.NotEmpty(t, transceivers)

		for port, xcvr := range transceivers {
			require.NotEmpty(t, xcvr.Identifier, port)
			require.Equal(t, "Inserted", xcvr.Status, port)
			require.NotEmpty(t, xcvr.VendorName, port)
			require.NotEmpty(t, xcvr.VendorPN, port)

			if xcvr.Temperature != nil && xcvr.Temperature.Temperature.OK {
				require.InDelta(t, 35, xcvr.Temperature.Temperature.Value, 65, port)
			}
			for id, channel := range xcvr.Channel {
				for _, power := range []*TransceiverPower{channel.RxPower, channel.TxPower} {
					// a reading of no light isn't one
					if power != nil && power.Power.OK {
						require.InDelta(t, -10, power.Power.DBm, 30, "%s %s", port, id)
					}
				}
				if channel.TxBiasCurrent != nil {
					require.True(t, channel.TxBiasCurrent.Current.OK, "%s %s", port, id)
				}
			}
		}

		requireGolden(t, model, "transceivers", transceivers)

		noLight := &TransceiverPower{Power: Power{Raw: "0.0000 mW / -inf dBm"}}
		notAvailable := Flag{Raw: "N/A"}

		// the modules the switches have for the fixtures, see testdata/README.md
		switch model {
		case modelSN5610:
			optic := transceivers["swp29"]
			require.Equal(t, "0x19 (OSFP 8X Pluggable Transceiver)", optic.Identifier)
			require.Equal(t, "Optical module", optic.CableType)
			require.Equal(t, "ETH_400GBASE_CR4", optic.PMDType)
			// the 4 lanes of 400G CR4 have light, the other 4 report a loss of signal
			require.Len(t, optic.Channel, 8)
			for i := 1; i <= 8; i++ {
				id := fmt.Sprintf("channel-%d", i)
				require.True(t, optic.Channel[id].RxPower.Power.OK, id)
				require.Equal(t, Flag{Value: i > 4, OK: true, Raw: map[bool]string{false: "False", true: "True"}[i > 4]},
					optic.Channel[id].RxLOS, id)
			}

			// a DAC has no diagnostics, also on the base port of a breakout
			for port, length := range map[string]Number{
				"swp1":  {Value: 2, OK: true, Raw: "2.00m"},
				"swp63": {Value: 1, OK: true, Raw: "1.00m"},
			} {
				dac := transceivers[port]
				require.Equal(t, "Passive copper cable", dac.CableType, port)
				require.Equal(t, length, dac.CableLength, port)
				require.Nil(t, dac.Temperature, port)
				require.Empty(t, dac.Channel, port)
			}

			// an SFP has no flags
			sfp := transceivers["swp65"]
			require.Equal(t, "0x03 (SFP)", sfp.Identifier)
			require.Len(t, sfp.Channel, 1)
			require.Equal(t, notAvailable, sfp.Channel["channel-1"].RxLOS)
			require.Equal(t, notAvailable, sfp.Channel["channel-1"].TxFault)
		case modelSN4700:
			qsfpdd := transceivers["swp9"]
			require.Equal(t, "0x18 (QSFP-DD Double Density 8X Pluggable Transceiver (INF-8628))", qsfpdd.Identifier)
			require.Equal(t, "ETH_400GBASE_CR8", qsfpdd.PMDType)
			require.Len(t, qsfpdd.Channel, 8)

			// a breakout cable with diagnostics but nothing to report, without the cable type
			breakout := transceivers["swp1"]
			require.Equal(t, "0x11 (QSFP28)", breakout.Identifier)
			require.Empty(t, breakout.CableType)
			require.Len(t, breakout.Channel, 4)
			require.Equal(t, noLight, breakout.Channel["channel-1"].RxPower)
			require.Equal(t, noLight, breakout.Channel["channel-1"].TxPower)
		case modelSN2700:
			// an older DAC reports the cable length without the decimals
			dac := transceivers["swp1"]
			require.Equal(t, "Passive copper cable", dac.CableType)
			require.Equal(t, Number{Value: 1, OK: true, Raw: "1m"}, dac.CableLength)
			require.Empty(t, dac.PMDType)
			require.Equal(t, noLight, dac.Channel["channel-1"].RxPower)
		}
	})
}
