// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeVPCID(t *testing.T) {
	for _, tt := range []struct {
		id       uint32
		expected string
		err      bool
	}{
		{0, "00000", false},
		{1, "00001", false},
		{2, "00002", false},
		{9, "00009", false},
		{10, "0000a", false},
		{11, "0000b", false},
		{12, "0000c", false},
		{33, "0000x", false},
		{34, "0000y", false},
		{35, "0000z", false},
		{36, "0000A", false},
		{37, "0000B", false},
		{38, "0000C", false},
		{59, "0000X", false},
		{60, "0000Y", false},
		{61, "0000Z", false},
		{62, "00010", false},
		{63, "00011", false},
		{64, "00012", false},
		{3843, "000ZZ", false},
		{3844, "00100", false},
		{916132831, "ZZZZZ", false},
		{916132832, "", true},
		{math.MaxUint32, "", true},
	} {
		t.Run(fmt.Sprintf("id-%d", tt.id), func(t *testing.T) {
			got, err := VPCID.Encode(tt.id)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expected, got)
		})
	}
}

func TestDecodeVPCID(t *testing.T) {
	for _, tt := range []struct {
		id       string
		expected uint32
		err      bool
	}{
		{"00000", 0, false},
		{"00001", 1, false},
		{"00002", 2, false},
		{"00009", 9, false},
		{"0000a", 10, false},
		{"0000b", 11, false},
		{"0000c", 12, false},
		{"0000x", 33, false},
		{"0000y", 34, false},
		{"0000z", 35, false},
		{"0000A", 36, false},
		{"0000B", 37, false},
		{"0000C", 38, false},
		{"0000X", 59, false},
		{"0000Y", 60, false},
		{"0000Z", 61, false},
		{"00010", 62, false},
		{"00011", 63, false},
		{"00012", 64, false},
		{"000ZZ", 3843, false},
		{"00100", 3844, false},
		{"ZZZZZ", 916132831, false},
		{"ZZZZZZ", 0, true},
		{"", 0, true},
		{"0", 0, true},
	} {
		t.Run(fmt.Sprintf("id-%s", tt.id), func(t *testing.T) {
			got, err := VPCID.Decode(tt.id)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expected, got)
		})
	}
}
