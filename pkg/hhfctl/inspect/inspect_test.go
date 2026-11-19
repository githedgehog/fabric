// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComparePortNames(t *testing.T) {
	sign := func(n int) int {
		switch {
		case n < 0:
			return -1
		case n > 0:
			return 1
		default:
			return 0
		}
	}

	for _, tt := range []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"Ethernet1", "Ethernet1", 0},
		{"Ethernet1", "", -1},           // empty sorts last
		{"", "Ethernet1", 1},            // empty sorts last
		{"Ethernet2", "Ethernet10", -1}, // natural, not lexical
		{"Ethernet10", "Ethernet2", 1},
		{"M1", "Ethernet1", -1}, // management port has priority
		{"Ethernet1", "M1", 1},
	} {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			require.Equal(t, tt.want, sign(comparePortNames(tt.a, tt.b)))
		})
	}

	t.Run("full sort", func(t *testing.T) {
		got := []string{"Ethernet10", "", "Ethernet2", "M1", "Ethernet1"}
		slices.SortFunc(got, comparePortNames)
		require.Equal(t, []string{"M1", "Ethernet1", "Ethernet2", "Ethernet10", ""}, got)
	})
}
