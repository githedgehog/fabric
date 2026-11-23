// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// the values are the forms NVUE reports them in on the env-7 switches
func TestNumber(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Number
	}{
		{in: `9216`, want: Number{Value: 9216, OK: true, Raw: "9216"}},
		{in: `15.5418788609217`, want: Number{Value: 15.5418788609217, OK: true, Raw: "15.5418788609217"}},
		{in: `-1`, want: Number{Value: -1, OK: true, Raw: "-1"}},
		{in: `"6974"`, want: Number{Value: 6974, OK: true, Raw: "6974"}},
		{in: `"54.0"`, want: Number{Value: 54, OK: true, Raw: "54.0"}},
		{in: `"113.932 mA"`, want: Number{Value: 113.932, OK: true, Raw: "113.932 mA"}},
		{in: `"3.2428 V"`, want: Number{Value: 3.2428, OK: true, Raw: "3.2428 V"}},
		{in: `"51.38 C"`, want: Number{Value: 51.38, OK: true, Raw: "51.38 C"}},
		{in: `"-5.00 C"`, want: Number{Value: -5, OK: true, Raw: "-5.00 C"}},
		{in: `"2.00m"`, want: Number{Value: 2, OK: true, Raw: "2.00m"}},
		{in: `"0.00 degrees C / 32.00 degrees F"`, want: Number{Value: 0, OK: true, Raw: "0.00 degrees C / 32.00 degrees F"}},
		{in: `"N/A"`, want: Number{Raw: "N/A"}},
		{in: `"n/a"`, want: Number{Raw: "n/a"}},
		{in: `"external"`, want: Number{Raw: "external"}},
		{in: `""`, want: Number{}},
		{in: `null`, want: Number{}},
	} {
		t.Run(tt.in, func(t *testing.T) {
			got := Number{Value: 42, OK: true, Raw: "stale"}
			require.NoError(t, json.Unmarshal([]byte(tt.in), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCounter(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Counter
	}{
		{in: `276736`, want: Counter{Value: 276736, OK: true}},
		{in: `0`, want: Counter{Value: 0, OK: true}},
		{in: `18446744073709551615`, want: Counter{Value: 18446744073709551615, OK: true}},
		{in: `"1798"`, want: Counter{Value: 1798, OK: true}},
		{in: `"n/a"`, want: Counter{}},
		{in: `"N/A"`, want: Counter{}},
		{in: `-1`, want: Counter{}},
		{in: `1.5`, want: Counter{}},
		{in: `null`, want: Counter{}},
	} {
		t.Run(tt.in, func(t *testing.T) {
			got := Counter{Value: 42, OK: true}
			require.NoError(t, json.Unmarshal([]byte(tt.in), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPower(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Power
	}{
		{in: `"1.0479 mW / 0.20 dBm"`, want: Power{DBm: 0.2, OK: true, Raw: "1.0479 mW / 0.20 dBm"}},
		{in: `"0.6004 mW / -2.22 dBm"`, want: Power{DBm: -2.22, OK: true, Raw: "0.6004 mW / -2.22 dBm"}},
		{in: `"0.0000 mW / -inf dBm"`, want: Power{Raw: "0.0000 mW / -inf dBm"}},
		{in: `"6.00 dBm"`, want: Power{DBm: 6, OK: true, Raw: "6.00 dBm"}},
		{in: `"-10.40 dBm"`, want: Power{DBm: -10.4, OK: true, Raw: "-10.40 dBm"}},
		{in: `"1.0479 mW"`, want: Power{Raw: "1.0479 mW"}},
		{in: `"N/A"`, want: Power{Raw: "N/A"}},
		{in: `null`, want: Power{}},
	} {
		t.Run(tt.in, func(t *testing.T) {
			got := Power{DBm: 42, OK: true, Raw: "stale"}
			require.NoError(t, json.Unmarshal([]byte(tt.in), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFlag(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Flag
	}{
		{in: `"True"`, want: Flag{Value: true, OK: true, Raw: "True"}},
		{in: `"False"`, want: Flag{OK: true, Raw: "False"}},
		{in: `"N/A"`, want: Flag{Raw: "N/A"}},
		{in: `null`, want: Flag{}},
	} {
		t.Run(tt.in, func(t *testing.T) {
			got := Flag{Value: true, OK: true, Raw: "stale"}
			require.NoError(t, json.Unmarshal([]byte(tt.in), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestValuesNotScalar(t *testing.T) {
	for _, in := range []string{`{"value": 1}`, `[1]`, `true`} {
		require.ErrorIs(t, json.Unmarshal([]byte(in), &Number{}), errNotScalar, in)
		require.ErrorIs(t, json.Unmarshal([]byte(in), &Counter{}), errNotScalar, in)
		require.ErrorIs(t, json.Unmarshal([]byte(in), &Flag{}), errNotScalar, in)
	}
	for _, in := range []string{`{"value": 1}`, `[1]`, `true`, `1.5`} {
		require.ErrorIs(t, json.Unmarshal([]byte(in), &Power{}), errNotScalar, in)
	}
}

func TestValuesInStruct(t *testing.T) {
	got := struct {
		Speed  Number  `json:"current-speed"`
		InPkts Counter `json:"in-pkts"`
		Drops  Counter `json:"transmit"`
	}{}
	require.NoError(t, json.Unmarshal([]byte(`{"current-speed": "6974", "in-pkts": 1798, "transmit": "n/a"}`), &got))
	require.Equal(t, Number{Value: 6974, OK: true, Raw: "6974"}, got.Speed)
	require.Equal(t, Counter{Value: 1798, OK: true}, got.InPkts)
	require.Equal(t, Counter{}, got.Drops)
}
