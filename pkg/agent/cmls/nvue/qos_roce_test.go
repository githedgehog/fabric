// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// env-7 has RoCE only on the SN2201
var roceCaptures = map[string][]string{
	modelSN5610: {"/qos/roce"},
	modelSN2201: {"/qos/roce"},
}

func TestRoCE(t *testing.T) {
	forEachModel(t, []string{modelSN5610, modelSN2201}, func(t *testing.T, client *Client, model string) {
		roce, err := client.RoCE(t.Context())
		require.NoError(t, err)

		if roce.Enabled() {
			require.Equal(t, StateEnabled, roce.State)
			require.Equal(t, RoCEModeLossless, roce.Mode)
		}

		switch model {
		case modelSN5610:
			require.Equal(t, &RoCE{}, roce)
			require.False(t, roce.Enabled())
		case modelSN2201:
			require.Equal(t, &RoCE{State: StateEnabled, Mode: RoCEModeLossless}, roce)
			require.True(t, roce.Enabled())
		}
	})
}

func TestRoCEEnabled(t *testing.T) {
	for _, tt := range []struct {
		roce RoCE
		want bool
	}{
		{roce: RoCE{State: StateEnabled, Mode: RoCEModeLossless}, want: true},
		{roce: RoCE{State: StateEnabled, Mode: "lossy"}},
		{roce: RoCE{State: StateDisabled, Mode: RoCEModeLossless}},
		{roce: RoCE{}},
	} {
		require.Equal(t, tt.want, tt.roce.Enabled(), "%+v", tt.roce)
	}
}
