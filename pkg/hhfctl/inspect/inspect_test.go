// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// colours would otherwise depend on whether go test streams its output to a terminal
func TestMain(m *testing.M) {
	color.NoColor = true //nolint:reassign // the way fatih/color is meant to be turned off
	os.Exit(m.Run())
}

func TestSwitchHeader(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) kmetav1.Time { return kmetav1.Time{Time: now.Add(-d)} }

	for _, tt := range []struct {
		name      string
		collected apiutil.Collected
		want      string
	}{
		{
			name:      "fresh",
			collected: apiutil.Collected{Time: ago(10 * time.Second)},
			want:      "Switch: leaf-1 (collected 10 seconds ago)\n",
		},
		{
			name:      "stale",
			collected: apiutil.Collected{Time: ago(5 * time.Minute), Stale: true},
			want:      "Switch: leaf-1 (collected 5 minutes ago, STALE: the agent missed heartbeats)\n",
		},
		{
			name:      "never",
			collected: apiutil.Collected{Stale: true},
			want:      "Switch: leaf-1 (never collected, STALE: the agent missed heartbeats)\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, switchHeader("leaf-1", tt.collected, now))
		})
	}
}

func TestCheckStale(t *testing.T) {
	collected := kmetav1.Time{Time: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}

	require.NoError(t, checkStale("leaf-1", apiutil.Collected{Time: collected}, true), "fresh state is fine")
	require.NoError(t, checkStale("leaf-1", apiutil.Collected{Time: collected, Stale: true}, false), "only a warning without strict")
	require.EqualError(t, checkStale("leaf-1", apiutil.Collected{Time: collected, Stale: true}, true),
		"switch leaf-1: state is stale, the agent missed heartbeats (last: 2026-11-04T12:00:00Z)")
	require.EqualError(t, checkStale("leaf-1", apiutil.Collected{Stale: true}, true),
		"switch leaf-1: state is stale, the agent missed heartbeats (last: never)")
}

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
