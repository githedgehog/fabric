// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
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

		power := func(dbm float64, raw string) *TransceiverPower {
			return &TransceiverPower{Power: Power{DBm: dbm, OK: true, Raw: raw}}
		}
		noLight := &TransceiverPower{Power: Power{Raw: "0.0000 mW / -inf dBm"}}
		notAvailable := Flag{Raw: "N/A"}
		no := Flag{OK: true, Raw: "False"}

		switch model {
		case modelSN5610:
			optic := transceivers["swp29"]
			require.Equal(t, "0x19 (OSFP 8X Pluggable Transceiver)", optic.Identifier)
			require.Equal(t, "Optical module", optic.CableType)
			require.Equal(t, "ETH_400GBASE_CR4", optic.PMDType)
			require.Equal(t, "Diagnostic Data Available", optic.DiagnosticsStatus)
			require.Equal(t, "Eoptolink", optic.VendorName)
			require.Equal(t, "70:ee:a3", optic.VendorOUI)
			require.Equal(t, "EOLO-138HG-5H-B", optic.VendorPN)
			require.Equal(t, "SN00000005", optic.VendorSN)
			require.Equal(t, Number{Value: 52.04, OK: true, Raw: "52.04 C"}, optic.Temperature.Temperature)
			require.Equal(t, Number{Value: 3.2443, OK: true, Raw: "3.2443 V"}, optic.Voltage.Voltage)
			require.Len(t, optic.Channel, 8)
			require.Equal(t, TransceiverChannel{
				RxPower:       power(0.14, "1.0325 mW / 0.14 dBm"),
				TxPower:       power(1.85, "1.5297 mW / 1.85 dBm"),
				TxBiasCurrent: &TransceiverBias{Current: Number{Value: 75.4, OK: true, Raw: "75.400 mA"}},
				RxLOS:         no,
				TxLOS:         no,
				TxFault:       no,
				RxCDRLOL:      no,
				TxCDRLOL:      no,
			}, optic.Channel["channel-1"])

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

			sfp := transceivers["swp65"]
			require.Equal(t, "0x03 (SFP)", sfp.Identifier)
			require.Len(t, sfp.Channel, 1)
			require.Equal(t, TransceiverChannel{
				RxPower:       power(-2.22, "0.6004 mW / -2.22 dBm"),
				TxPower:       power(-2.57, "0.5533 mW / -2.57 dBm"),
				TxBiasCurrent: &TransceiverBias{Current: Number{Value: 5.48, OK: true, Raw: "5.480 mA"}},
				RxLOS:         notAvailable,
				TxLOS:         notAvailable,
				TxFault:       notAvailable,
				RxCDRLOL:      notAvailable,
				TxCDRLOL:      notAvailable,
			}, sfp.Channel["channel-1"])
		case modelSN4700:
			qsfpdd := transceivers["swp9"]
			require.Equal(t, "0x18 (QSFP-DD Double Density 8X Pluggable Transceiver (INF-8628))", qsfpdd.Identifier)
			require.Equal(t, "ETH_400GBASE_CR8", qsfpdd.PMDType)
			require.Len(t, qsfpdd.Channel, 8)
			require.Equal(t, power(0.93, "1.2400 mW / 0.93 dBm"), qsfpdd.Channel["channel-1"].RxPower)

			// a breakout cable with diagnostics but nothing to report, without the cable type
			breakout := transceivers["swp1"]
			require.Equal(t, "0x11 (QSFP28)", breakout.Identifier)
			require.Empty(t, breakout.CableType)
			require.Len(t, breakout.Channel, 4)
			require.Equal(t, noLight, breakout.Channel["channel-1"].RxPower)
			require.Equal(t, noLight, breakout.Channel["channel-1"].TxPower)
			require.Equal(t, Number{Value: 0, OK: true, Raw: "0.000 mA"}, breakout.Channel["channel-1"].TxBiasCurrent.Current)
		case modelSN2700:
			// an older DAC reports the cable length without the decimals
			dac := transceivers["swp1"]
			require.Equal(t, "Passive copper cable", dac.CableType)
			require.Equal(t, Number{Value: 1, OK: true, Raw: "1m"}, dac.CableLength)
			require.Empty(t, dac.PMDType)
			require.Equal(t, Number{Value: 31.3, OK: true, Raw: "31.30 C"}, dac.Temperature.Temperature)
			require.Equal(t, noLight, dac.Channel["channel-1"].RxPower)
		}
	})
}
