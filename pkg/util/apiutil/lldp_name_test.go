// Copyright 2026 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apiutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLLDPNeighborMatches(t *testing.T) {
	t.Parallel()

	expected := LLDPNeighbor{Name: "spark-1", Port: "p2"}
	fabric := LLDPNeighbor{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric"}

	for _, tt := range []struct {
		name     string
		actual   LLDPNeighbor
		expected LLDPNeighbor
		want     bool
	}{
		{name: "exact", actual: LLDPNeighbor{Name: "spark-1", Port: "p2"}, expected: expected, want: true},
		// host and port names are case insensitive, the expected ones are spelled however the wiring spells them
		{name: "name case", actual: LLDPNeighbor{Name: "SPARK-1", Port: "p2"}, expected: expected, want: true},
		{name: "port case", actual: LLDPNeighbor{Name: "spark-1", Port: "P2"}, expected: expected, want: true},
		{
			name:     "ignored suffix and case",
			actual:   LLDPNeighbor{Name: "SPARK-1.LAN", Port: "p2", IgnoredSuffix: ".LAN"},
			expected: expected, want: true,
		},
		{name: "wrong name", actual: LLDPNeighbor{Name: "spark-2", Port: "p2"}, expected: expected},
		{name: "wrong port", actual: LLDPNeighbor{Name: "spark-1", Port: "p1"}, expected: expected},
		// only the fabric connections expect a description, and it's compared ignoring case too
		{
			name:     "description case",
			actual:   LLDPNeighbor{Name: "ds5000-03", Port: "E1/61/1", Description: "HEDGEHOG FABRIC"},
			expected: fabric, want: true,
		},
		{
			name:     "wrong description",
			actual:   LLDPNeighbor{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric 5835"},
			expected: fabric,
		},
		{
			name:     "no description expected",
			actual:   LLDPNeighbor{Name: "spark-1", Port: "p2", Description: "Ubuntu 24.04"},
			expected: expected, want: true,
		},
		// a port with nothing expected on it can't be right
		{name: "nothing expected", actual: LLDPNeighbor{Name: "spark-1", Port: "p2"}},
		{name: "both empty", actual: LLDPNeighbor{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.actual.Matches(tt.expected))
		})
	}
}

func TestLLDPNeighborNameCut(t *testing.T) {
	t.Parallel()

	defaults := LLDPNeighborsOpts{
		IgnorePrefixes: DefaultLLDPIgnorePrefixes,
		IgnoreSuffixes: DefaultLLDPIgnoreSuffixes,
	}

	for _, tt := range []struct {
		name     string
		in       string
		expected string
		opts     LLDPNeighborsOpts
		// what's left of the reported name once the ignored parts are cut off
		want string
	}{
		{name: "already matching", in: "server-1", expected: "server-1", opts: defaults, want: "server-1"},
		{name: "nothing configured to ignore", in: "spark-1.lan", expected: "spark-1", want: "spark-1.lan"},
		{name: "lan", in: "spark-1.lan", expected: "spark-1", opts: defaults, want: "spark-1"},
		{name: "maas", in: "pai-8xb200-92-1.maas", expected: "pai-8xb200-92-1", opts: defaults, want: "pai-8xb200-92-1"},
		{name: "dpu", in: "ash033-dpu", expected: "ash033", opts: defaults, want: "ash033"},
		// the wiring is free to name the DPU in full, stripping must not turn a match into a mismatch
		{name: "expected keeps the suffix", in: "ash033-dpu", expected: "ash033-dpu", opts: defaults, want: "ash033-dpu"},
		// host names are case insensitive and the expected name is a Kubernetes object name, so always lower case,
		// but the neighbor keeps the case it reported: only the parts that were ignored are cut off
		{name: "uppercase suffix", in: "SPARK-1.LAN", expected: "spark-1", opts: defaults, want: "SPARK-1"},
		{name: "uppercase name only", in: "SPARK-1", expected: "spark-1", opts: defaults, want: "SPARK-1"},
		{name: "uppercase without a match", in: "SPARK-1.LAN", expected: "spark-2", opts: defaults, want: "SPARK-1.LAN"},
		// a DPU reachable through a local domain carries both, in either order
		{name: "chained", in: "ash033-dpu.lan", expected: "ash033", opts: defaults, want: "ash033"},
		{name: "chained partially", in: "ash033-dpu.lan", expected: "ash033-dpu", opts: defaults, want: "ash033-dpu"},
		// stripping is only worth it when it produces the expected name, otherwise report what's on the wire
		{name: "no combination matches", in: "spark-1.lan", expected: "spark-2", opts: defaults, want: "spark-1.lan"},
		{name: "nothing expected", in: "spark-1.lan", expected: "", opts: defaults, want: "spark-1.lan"},
		{name: "empty name", in: "", expected: "server-1", opts: defaults, want: ""},
		// an empty name can never be what the wiring expects, so it's never stripped that far
		{name: "would strip to nothing", in: ".lan", expected: "", opts: defaults, want: ".lan"},
		{
			name: "prefix",
			in:   "sw-leaf-1", expected: "leaf-1",
			opts: LLDPNeighborsOpts{IgnorePrefixes: []string{"sw-"}},
			want: "leaf-1",
		},
		{
			name: "prefix and suffix",
			in:   "sw-leaf-1.lan", expected: "leaf-1",
			opts: LLDPNeighborsOpts{IgnorePrefixes: []string{"sw-"}, IgnoreSuffixes: DefaultLLDPIgnoreSuffixes},
			want: "leaf-1",
		},
		// an empty entry would match everything and never shorten the name
		{
			name: "empty entries ignored",
			in:   "server-1.lan", expected: "server-1",
			opts: LLDPNeighborsOpts{IgnorePrefixes: []string{""}, IgnoreSuffixes: []string{"", ".lan"}},
			want: "server-1",
		},
		// overlapping suffixes: stripping "pu" first leads nowhere, so both branches have to be explored
		{
			name: "overlapping suffixes",
			in:   "ash033-dpu", expected: "ash033",
			opts: LLDPNeighborsOpts{IgnoreSuffixes: []string{"pu", "-dpu"}},
			want: "ash033",
		},
		{
			name: "overlapping suffixes reversed",
			in:   "ash033-dpu", expected: "ash033",
			opts: LLDPNeighborsOpts{IgnoreSuffixes: []string{"-dpu", "pu"}},
			want: "ash033",
		},
		// and the branch the other one leads to is reachable too
		{
			name: "overlapping suffixes other branch",
			in:   "ash033-dpu", expected: "ash033-d",
			opts: LLDPNeighborsOpts{IgnoreSuffixes: []string{"pu", "-dpu"}},
			want: "ash033-d",
		},
		// repeated stripping of the same suffix is reachable as well
		{
			name: "repeated suffix",
			in:   "server-1.lan.lan", expected: "server-1",
			opts: defaults,
			want: "server-1",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prefix, suffix := lldpNeighborNameCut(tt.in, tt.expected, tt.opts)

			// what's left after ignoring the reported parts is what got compared against the wiring
			require.Equal(t, tt.want, strings.TrimSuffix(strings.TrimPrefix(tt.in, prefix), suffix))
			require.True(t, strings.HasPrefix(tt.in, prefix), "ignored prefix %q not in %q", prefix, tt.in)
			require.True(t, strings.HasSuffix(tt.in, suffix), "ignored suffix %q not in %q", suffix, tt.in)
		})
	}
}
