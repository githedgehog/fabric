// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNumberUnmarshal(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Number
		err  bool
	}{
		{in: `7020`, want: Number{Value: 7020, OK: true, Raw: "7020"}},
		{in: `8.998164098689202e-08`, want: Number{Value: 8.998164098689202e-08, OK: true, Raw: "8.998164098689202e-08"}},
		{in: `"7020"`, want: Number{Value: 7020, OK: true, Raw: "7020"}},
		{in: `"54.0"`, want: Number{Value: 54, OK: true, Raw: "54.0"}},
		{in: `"28.80 C"`, want: Number{Value: 28.8, OK: true, Raw: "28.80 C"}},
		{in: `"-10.00 C"`, want: Number{Value: -10, OK: true, Raw: "-10.00 C"}},
		{in: `"2.00m"`, want: Number{Value: 2, OK: true, Raw: "2.00m"}},
		{in: `"N/A"`, want: Number{Raw: "N/A"}},
		{in: `"external"`, want: Number{Raw: "external"}},
		{in: `""`, want: Number{}},
		{in: `null`, want: Number{}},
		{in: `true`, err: true},
		{in: `{}`, err: true},
	} {
		t.Run(tt.in, func(t *testing.T) {
			var got Number
			err := json.Unmarshal([]byte(tt.in), &got)
			if tt.err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCounterUnmarshal(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Counter
		err  bool
	}{
		{in: `313978`, want: Counter{Value: 313978, OK: true}},
		{in: `18446744073709551615`, want: Counter{Value: math.MaxUint64, OK: true}},
		{in: `"42"`, want: Counter{Value: 42, OK: true}},
		{in: `"n/a"`, want: Counter{}},
		{in: `-1`, want: Counter{}},
		{in: `1.5`, want: Counter{}},
		{in: `null`, want: Counter{}},
		{in: `[]`, err: true},
	} {
		t.Run(tt.in, func(t *testing.T) {
			var got Counter
			err := json.Unmarshal([]byte(tt.in), &got)
			if tt.err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPowerUnmarshal(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Power
	}{
		{in: `"0.6004 mW / -2.22 dBm"`, want: Power{DBm: -2.22, OK: true, Raw: "0.6004 mW / -2.22 dBm"}},
		{in: `"1.00 dBm"`, want: Power{DBm: 1, OK: true, Raw: "1.00 dBm"}},
		{in: `-8`, want: Power{DBm: -8, OK: true, Raw: "-8"}},
		{in: `"0.0000 mW / -inf dBm"`, want: Power{Raw: "0.0000 mW / -inf dBm"}},
		{in: `"N/A"`, want: Power{Raw: "N/A"}},
		{in: `null`, want: Power{}},
	} {
		t.Run(tt.in, func(t *testing.T) {
			var got Power
			require.NoError(t, json.Unmarshal([]byte(tt.in), &got))
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("milliwatts only", func(t *testing.T) {
		var got Power
		require.NoError(t, json.Unmarshal([]byte(`"0.5 mW"`), &got))
		require.True(t, got.OK)
		require.InDelta(t, -3.0103, got.DBm, 0.0001)
	})
}

func TestTransceiverDecode(t *testing.T) {
	// a trimmed GET /platform/transceiver/swp65 from a SN5610
	data := `{
		"cable-type": "Optical module", "identifier": "SFP", "status": "Inserted", "pmd-type": "ETH_XAUI_1_10G",
		"vendor-name": "Edgecore", "vendor-sn": "L22310000360",
		"temperature": {"alarm": "Off", "high-alarm-threshold": "85.00 C", "temperature": "28.89 C", "setpoint": 75},
		"voltage": {"alarm": "Off", "voltage": "3.2766 V"},
		"channel": {"channel-1": {
			"rx-power": {"alarm": "Off", "high-alarm-threshold": "1.00 dBm", "power": "0.6004 mW / -2.22 dBm"},
			"tx-bias-current": {"alarm": "Off", "current": "5.480 mA"},
			"tx-power": {"alarm": "Off", "power": "0.5533 mW / -2.57 dBm"}
		}}
	}`

	var tr Transceiver
	require.NoError(t, json.Unmarshal([]byte(data), &tr))
	require.Equal(t, TransceiverStatusInserted, tr.Status)
	require.InDelta(t, 28.89, tr.Temperature.Temperature.Value, 0.001)
	require.InDelta(t, 85, tr.Temperature.HighAlarm.Value, 0.001)
	require.InDelta(t, 3.2766, tr.Voltage.Voltage.Value, 0.0001)
	ch := tr.Channel["channel-1"]
	require.InDelta(t, -2.22, ch.RxPower.Power.DBm, 0.001)
	require.InDelta(t, 1, ch.RxPower.HighAlarm.Value, 0.001)
	require.InDelta(t, -2.57, ch.TxPower.Power.DBm, 0.001)
	require.InDelta(t, 5.48, ch.TxBiasCurrent.Current.Value, 0.001)
}
