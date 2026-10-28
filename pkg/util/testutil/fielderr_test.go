// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type testMeta struct {
	Name string `json:"name,omitempty"`
}

type testInline struct {
	Kind string `json:"kind,omitempty"`
}

type testEntry struct {
	Start string `json:"start,omitempty"`
}

type testSpec struct {
	Mode    string                `json:"mode,omitempty"`
	Entries map[string]*testEntry `json:"entries,omitempty"`
	Groups  [][]string            `json:"groups,omitempty"`
	List    []testEntry           `json:"list,omitempty"`
	Nested  *testEntry            `json:"nested,omitempty"`
	Skipped string                `json:"-"`
}

type testObj struct {
	testInline `json:",inline"`
	Meta       testMeta `json:"metadata,omitempty"`
	Spec       testSpec `json:"spec,omitempty"`
}

func TestResolveFieldPath(t *testing.T) {
	for _, tt := range []struct {
		path  string
		valid bool
	}{
		{path: "kind", valid: true},
		{path: "metadata.name", valid: true},
		{path: "spec", valid: true},
		{path: "spec.entries", valid: true},
		{path: "spec.entries[foo]", valid: true},
		{path: "spec.entries[a.b].start", valid: true},
		{path: "spec.entries[10.0.0.0/24].start", valid: true},
		{path: "spec.groups[0]", valid: true},
		{path: "spec.groups[0][1]", valid: true},
		{path: "spec.list[2].start", valid: true},
		{path: "spec.nested.start", valid: true},
		{path: "spec.entry"},
		{path: "spec.Entries"},
		{path: "spec.Skipped"},
		{path: "spec.entries[foo].end"},
		{path: "spec.mode[0]"},
		{path: "spec.mode.foo"},
		{path: "spec.nested[0]"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			err := ResolveFieldPath(reflect.TypeFor[*testObj](), tt.path)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
