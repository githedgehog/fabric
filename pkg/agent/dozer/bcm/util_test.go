// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
)

func TestUnmarshalAS(t *testing.T) {
	for _, tc := range []struct {
		in       any
		expected *uint32
		err      bool
	}{
		{in: nil},
		{in: oc.UnionUint32(65100), expected: new(uint32(65100))},
		{in: oc.UnionString("65100"), expected: new(uint32(65100))},
		{in: oc.UnionString("0.65100"), expected: new(uint32(65100))},
		{in: oc.UnionString("64086.59904"), expected: new(uint32(4200000000))},
		{in: oc.UnionString("4200000000"), expected: new(uint32(4200000000))},
		{in: oc.UnionString("65536.0"), err: true},
		{in: oc.UnionString("1.2.3"), err: true},
		{in: oc.UnionString(""), err: true},
	} {
		as, err := UnmarshalAS(tc.in)
		if tc.err {
			require.Error(t, err, "%v", tc.in)

			continue
		}
		require.NoError(t, err, "%v", tc.in)
		require.Equal(t, tc.expected, as, "%v", tc.in)
	}
}
