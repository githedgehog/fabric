// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchstate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeNOSVersion(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"4.5.2-Enterprise_Base", "v4.5.2"},
		{"4.5.2", "v4.5.2"},
		{"v5.0.0", "v5.0.0"},
		{"5.16.5", "v5.16.5"},
		{"4.5", ""},
		{"", ""},
		{"master.123-abc", ""},
	} {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeNOSVersion(tt.in))
		})
	}
}
