package librarian_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
)

func TestNextFreeUin16Allocator(t *testing.T) {
	for _, test := range []struct {
		name     string
		values   librarian.Values[uint16]
		known    map[string]uint16
		updates  map[string]bool
		expected map[string]uint16
		err      bool
	}{
		{
			name:   "simple-noop",
			values: librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
		},
		{
			name:   "simple-no-updates",
			values: librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
			known:  map[string]uint16{"a": 1, "b": 2, "c": 3},
		},
		{
			name:     "simple-no-new-updates",
			values:   librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
			known:    map[string]uint16{"a": 1, "b": 2, "c": 3},
			updates:  map[string]bool{"a": true, "b": true, "c": true},
			expected: map[string]uint16{"a": 1, "b": 2, "c": 3},
		},
		{
			name:     "simple-remove-updates",
			values:   librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
			known:    map[string]uint16{"a": 1, "b": 2, "c": 3},
			updates:  map[string]bool{"a": true, "c": true},
			expected: map[string]uint16{"a": 1, "c": 3},
		},
		{
			name:     "simple-check-false-updates",
			values:   librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
			known:    map[string]uint16{"a": 1, "b": 2, "c": 3},
			updates:  map[string]bool{"a": true, "b": false, "c": true},
			expected: map[string]uint16{"a": 1, "c": 3},
		},
		{
			name:     "simple-some-new-updates",
			values:   librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
			known:    map[string]uint16{"a": 1, "b": 2, "c": 3},
			updates:  map[string]bool{"a": true, "b": true, "c": true, "d": true},
			expected: map[string]uint16{"a": 1, "b": 2, "c": 3, "d": 4},
		},
		{
			name:     "simple-all-new-updates",
			values:   librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
			known:    map[string]uint16{"a": 1, "b": 2, "c": 3},
			updates:  map[string]bool{"d": true},
			expected: map[string]uint16{"d": 1},
		},
		{
			name:     "simple-some-replaced-updates",
			values:   librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
			known:    map[string]uint16{"a": 1, "b": 2, "c": 3},
			updates:  map[string]bool{"a": true, "c": true, "d": true},
			expected: map[string]uint16{"a": 1, "d": 2, "c": 3},
		},
		{
			name:    "simple-not-enough-values",
			values:  librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 2}}, 1),
			known:   map[string]uint16{"a": 1, "b": 2},
			updates: map[string]bool{"a": true, "b": true, "c": true},
			err:     true,
		},
		{
			name:     "simple-big-inc",
			values:   librarian.NewNextFreeValueFromRanges([][2]uint16{{100, 600}}, 100),
			known:    map[string]uint16{"a": 100, "b": 200},
			updates:  map[string]bool{"a": true, "b": true, "c": true},
			expected: map[string]uint16{"a": 100, "b": 200, "c": 300},
		},
		{
			name:    "simple-big-inc-out-of-range",
			values:  librarian.NewNextFreeValueFromRanges([][2]uint16{{100, 600}}, 100),
			known:   map[string]uint16{"a": 1, "b": 200},
			updates: map[string]bool{"a": true, "b": true, "c": true},
			err:     true,
		},
		{
			name:     "vlan-some-new-updates",
			values:   librarian.NewNextFreeValueFromVLANRanges([]meta.VLANRange{{From: 1, To: 6}}),
			known:    map[string]uint16{"a": 1, "b": 2, "c": 3},
			updates:  map[string]bool{"a": true, "b": true, "c": true, "d": true},
			expected: map[string]uint16{"a": 1, "b": 2, "c": 3, "d": 4},
		},
		{
			name:     "vlan-some-new-updates-multiple",
			values:   librarian.NewNextFreeValueFromVLANRanges([]meta.VLANRange{{From: 1, To: 2}, {From: 5, To: 5}, {From: 10, To: 12}}),
			known:    map[string]uint16{"a": 1, "b": 2, "c": 5},
			updates:  map[string]bool{"a": true, "b": true, "c": true, "d": true},
			expected: map[string]uint16{"a": 1, "b": 2, "c": 5, "d": 10},
		},
		{
			name:    "vlan-some-new-updates-multiple-not-enough-values",
			values:  librarian.NewNextFreeValueFromVLANRanges([]meta.VLANRange{{From: 1, To: 2}, {From: 5, To: 5}}),
			known:   map[string]uint16{"a": 1, "b": 2, "c": 5},
			updates: map[string]bool{"a": true, "b": true, "c": true, "d": true},
			err:     true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := librarian.Allocator[uint16]{
				Values: test.values,
			}
			actual, err := a.Allocate(test.known, test.updates)

			if test.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				if test.expected == nil {
					test.expected = map[string]uint16{}
				}

				require.Equal(t, test.expected, actual)
			}
		})
	}
}

func TestBalancedStringAllocator(t *testing.T) {
	for _, test := range []struct {
		name     string
		values   librarian.Values[string]
		known    map[string]string
		updates  map[string]bool
		expected map[string]string
		err      bool
	}{
		{
			name:   "simple-noop",
			values: librarian.NewBalancedValues([]string{"1", "2"}),
		},
		// {
		// 	name:   "simple-no-updates",
		// 	values: librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
		// 	known:  map[string]uint16{"a": 1, "b": 2, "c": 3},
		// },
		{
			name:   "simple-no-updates",
			values: librarian.NewBalancedValues([]string{"1", "2"}),
			known:  map[string]string{"a": "1", "b": "2", "c": "2"},
		},
		// {
		// 	name:     "simple-no-new-updates",
		// 	values:   librarian.NewNextFreeValueFromRanges([][2]uint16{{1, 6}}, 1),
		// 	known:    map[string]uint16{"a": 1, "b": 2, "c": 3},
		// 	updates:  map[string]bool{"a": true, "b": true, "c": true},
		// 	expected: map[string]uint16{"a": 1, "b": 2, "c": 3},
		// },
		{
			name:     "simple-no-new-updates-1",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "2", "c": "2"},
			updates:  map[string]bool{"a": true, "b": true, "c": true},
			expected: map[string]string{"a": "1", "b": "2", "c": "2"},
		},
		{
			name:     "simple-no-new-updates-2",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "2", "c": "1"},
			updates:  map[string]bool{"a": true, "b": true, "c": true},
			expected: map[string]string{"a": "1", "b": "2", "c": "1"},
		},
		{
			name:     "simple-remove-updates",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "2", "c": "1"},
			updates:  map[string]bool{"a": true, "c": true},
			expected: map[string]string{"a": "1", "c": "1"},
		},
		{
			name:     "simple-check-false-updates",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "2", "c": "1"},
			updates:  map[string]bool{"a": true, "b": false, "c": true},
			expected: map[string]string{"a": "1", "c": "1"},
		},
		{
			name:     "simple-some-new-updates",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "2", "c": "1"},
			updates:  map[string]bool{"a": true, "b": true, "c": true, "d": true},
			expected: map[string]string{"a": "1", "b": "2", "c": "1", "d": "2"},
		},
		{
			name:     "simple-all-new-updates",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "2", "c": "1"},
			updates:  map[string]bool{"d": true},
			expected: map[string]string{"d": "1"},
		},
		{
			name:     "simple-some-replaced-updates",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "2", "c": "1"},
			updates:  map[string]bool{"a": true, "c": true, "d": true},
			expected: map[string]string{"a": "1", "c": "1", "d": "2"},
		},
		{
			name:    "simple-out-of-range-1",
			values:  librarian.NewBalancedValues([]string{"1", "2"}),
			known:   map[string]string{"a": "1", "b": "2", "c": "3"},
			updates: map[string]bool{"a": true, "b": true, "c": true},
			err:     true,
		},
		{
			name:     "simple-out-of-range-2",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "2", "d": "3"},
			updates:  map[string]bool{"a": true, "b": true},
			expected: map[string]string{"a": "1", "b": "2"},
		},
		{
			name:    "simple-empty-values",
			values:  librarian.NewBalancedValues([]string{}),
			known:   map[string]string{},
			updates: map[string]bool{"a": true},
			err:     true,
		},
		{
			name:     "simple-keep-known-unbalanced",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "1", "c": "1"},
			updates:  map[string]bool{"a": true, "b": true, "c": true},
			expected: map[string]string{"a": "1", "b": "1", "c": "1"},
		},
		{
			name:     "simple-replace-and-balanced",
			values:   librarian.NewBalancedValues([]string{"1", "2"}),
			known:    map[string]string{"a": "1", "b": "1", "c": "1"},
			updates:  map[string]bool{"a": true, "b": true, "d": true},
			expected: map[string]string{"a": "1", "b": "1", "d": "2"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := librarian.Allocator[string]{
				Values: test.values,
			}
			actual, err := a.Allocate(test.known, test.updates)

			if test.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				if test.expected == nil {
					test.expected = map[string]string{}
				}

				require.Equal(t, test.expected, actual)
			}
		})
	}
}
