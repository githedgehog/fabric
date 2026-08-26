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

package inspect

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// a neighbor that doesn't advertise a system name or a port description, so its port falls back to its MAC, seen
	// next to the real server-5 neighbor on ds5000-02/E1/5/1 in env-5
	lldpNeighborExtra = apiutil.LLDPNeighbor{Port: "00:00:00:0b:bb:19", MAC: "00:00:00:0b:bb:19", TTL: 120}
	lldpNeighborWired = apiutil.LLDPNeighbor{Name: "server-5", Port: "enp2s1", MAC: "00:00:00:0b:bb:11", TTL: 120}
)

func TestLLDPNeighborRows(t *testing.T) {
	t.Parallel()

	expected := apiutil.LLDPNeighbor{Name: "server-5", Port: "enp2s1"}

	for _, tt := range []struct {
		name       string
		status     apiutil.LLDPNeighborStatus
		showAll    bool
		wantRows   []lldpNeighborRow
		wantHidden int
	}{
		{
			name:     "no neighbors",
			status:   apiutil.LLDPNeighborStatus{Expected: expected},
			wantRows: []lldpNeighborRow{},
		},
		{
			// the wired neighbor is reported second, it still has to come first and suppress the other one
			name: "wired neighbor found",
			status: apiutil.LLDPNeighborStatus{
				Expected: expected,
				Actual:   []apiutil.LLDPNeighbor{lldpNeighborExtra, lldpNeighborWired},
			},
			wantRows:   []lldpNeighborRow{{neighbor: lldpNeighborWired}},
			wantHidden: 1,
		},
		{
			name: "wired neighbor found with show all",
			status: apiutil.LLDPNeighborStatus{
				Expected: expected,
				Actual:   []apiutil.LLDPNeighbor{lldpNeighborExtra, lldpNeighborWired},
			},
			showAll: true,
			wantRows: []lldpNeighborRow{
				{neighbor: lldpNeighborWired},
				{neighbor: lldpNeighborExtra, extra: true},
			},
		},
		{
			// port doesn't match, so nothing is known to be right and the order is kept as reported
			name: "no matching neighbor",
			status: apiutil.LLDPNeighborStatus{
				Expected: apiutil.LLDPNeighbor{Name: "spark-1", Port: "p1"},
				Actual: []apiutil.LLDPNeighbor{
					{Name: "spark-1", Port: "enP2p1s0f0np0"},
					{Name: "spark-1", Port: "enp1s0f0np0"},
				},
			},
			wantRows: []lldpNeighborRow{
				{neighbor: apiutil.LLDPNeighbor{Name: "spark-1", Port: "enP2p1s0f0np0"}},
				{neighbor: apiutil.LLDPNeighbor{Name: "spark-1", Port: "enp1s0f0np0"}},
			},
		},
		{
			// nothing is expected on external connections, a nameless neighbor must not match the empty expectation
			name: "nothing expected",
			status: apiutil.LLDPNeighborStatus{
				Actual: []apiutil.LLDPNeighbor{lldpNeighborExtra, lldpNeighborWired},
			},
			wantRows: []lldpNeighborRow{
				{neighbor: lldpNeighborExtra},
				{neighbor: lldpNeighborWired},
			},
		},
		{
			// the fabric connections expect a description too, so a mismatching one isn't the wired neighbor
			name: "description mismatch",
			status: apiutil.LLDPNeighborStatus{
				Expected: apiutil.LLDPNeighbor{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric"},
				Actual: []apiutil.LLDPNeighbor{
					{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric 5835"},
				},
			},
			wantRows: []lldpNeighborRow{
				{neighbor: apiutil.LLDPNeighbor{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric 5835"}},
			},
		},
		{
			name: "description matches",
			status: apiutil.LLDPNeighborStatus{
				Expected: apiutil.LLDPNeighbor{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric"},
				Actual: []apiutil.LLDPNeighbor{
					lldpNeighborExtra,
					{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric"},
				},
			},
			wantRows: []lldpNeighborRow{
				{neighbor: apiutil.LLDPNeighbor{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric"}},
			},
			wantHidden: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rows, hidden := lldpNeighborRows(tt.status, tt.showAll)
			require.Equal(t, tt.wantRows, rows)
			require.Equal(t, tt.wantHidden, hidden)
		})
	}
}

// TestLLDPIgnoredNameParts covers a neighbor that only matches the wiring once the ignored parts of its name are taken
// out: it still reports the name it advertises, and it isn't treated as a mismatch.
func TestLLDPIgnoredNameParts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	// what apiutil reports for a host calling itself spark-1.lan wired as spark-1
	neighbor := apiutil.LLDPNeighbor{
		Name: "spark-1.lan", Port: "p2", MAC: "4c:bb:47:e8:ef:fb", IgnoredSuffix: ".lan",
	}
	status := apiutil.LLDPNeighborStatus{
		ConnectionName: "ds5000-02--spark-1-2",
		ConnectionType: "unbundled",
		Type:           apiutil.LLDPNeighborTypeServer,
		Expected:       apiutil.LLDPNeighbor{Name: "spark-1", Port: "p2"},
		Actual:         []apiutil.LLDPNeighbor{neighbor},
	}

	require.Equal(t, "spark-1", neighbor.MatchedName())

	// the ignored parts are what makes it the wired neighbor, so it's neither a mismatch nor an error
	matching, ok := lldpMatchingNeighbor(status)
	require.True(t, ok)
	require.Equal(t, neighbor, matching)
	require.Empty(t, lldpStrictErrs("ds5000-02", "E1/1/1", status))

	out := &LLDPOut{Neighbors: map[string]map[string]apiutil.LLDPNeighborStatus{
		"ds5000-02": {"E1/1/1": status},
	}}

	res, err := out.MarshalText(LLDPIn{}, now)
	require.NoError(t, err)

	// the name is rendered as the neighbor advertises it, not cut down to what matched
	require.Contains(t, res, "spark-1.lan")
	require.NotContains(t, res, "(want")
}

func TestLLDPStrictErrs(t *testing.T) {
	t.Parallel()

	expected := apiutil.LLDPNeighbor{Name: "server-5", Port: "enp2s1"}
	fabric := apiutil.LLDPNeighbor{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric"}

	for _, tt := range []struct {
		name     string
		status   apiutil.LLDPNeighborStatus
		wantErrs []string
	}{
		{
			// the wiring is satisfied, the nameless neighbor next to it isn't an error
			name: "wired neighbor found",
			status: apiutil.LLDPNeighborStatus{
				Type:     apiutil.LLDPNeighborTypeServer,
				Expected: expected,
				Actual:   []apiutil.LLDPNeighbor{lldpNeighborExtra, lldpNeighborWired},
			},
		},
		{
			name: "right name on a wrong port",
			status: apiutil.LLDPNeighborStatus{
				Type:     apiutil.LLDPNeighborTypeServer,
				Expected: expected,
				Actual:   []apiutil.LLDPNeighbor{{Name: "server-5", Port: "enp2s2"}},
			},
			wantErrs: []string{`switch sw: E1/5/1: expected neighbor port "enp2s1", got "enp2s2"`},
		},
		{
			name: "right name and port on a wrong description",
			status: apiutil.LLDPNeighborStatus{
				Type:     apiutil.LLDPNeighborTypeFabric,
				Expected: fabric,
				Actual:   []apiutil.LLDPNeighbor{{Name: "ds5000-03", Port: "E1/61/1", Description: "Hedgehog Fabric 5835"}},
			},
			wantErrs: []string{`switch sw: E1/5/1: expected neighbor description "Hedgehog Fabric", got "Hedgehog Fabric 5835"`},
		},
		{
			name: "no neighbors at all",
			status: apiutil.LLDPNeighborStatus{
				Type:     apiutil.LLDPNeighborTypeServer,
				Expected: expected,
			},
			wantErrs: []string{`switch sw: E1/5/1: expected neighbor "server-5" not found`},
		},
		{
			// a neighbor without a name is reported by its MAC so the error names something usable
			name: "only a nameless neighbor",
			status: apiutil.LLDPNeighborStatus{
				Type:     apiutil.LLDPNeighborTypeServer,
				Expected: expected,
				Actual:   []apiutil.LLDPNeighbor{lldpNeighborExtra},
			},
			wantErrs: []string{`switch sw: E1/5/1: expected neighbor "server-5" not found, but found: [00:00:00:0b:bb:19]`},
		},
		{
			name: "someone else on the port",
			status: apiutil.LLDPNeighborStatus{
				Type:     apiutil.LLDPNeighborTypeServer,
				Expected: apiutil.LLDPNeighbor{Name: "spark-1", Port: "p2"},
				Actual: []apiutil.LLDPNeighbor{
					{Name: "spark-1.lan", Port: "enP2p1s0f1np1"},
					{Name: "spark-1.lan", Port: "enp1s0f1np1"},
				},
			},
			wantErrs: []string{`switch sw: E1/5/1: expected neighbor "spark-1" not found, but found: [spark-1.lan spark-1.lan]`},
		},
		{
			// external connections don't expect a neighbor, whatever is there is fine
			name: "external",
			status: apiutil.LLDPNeighborStatus{
				Type:   apiutil.LLDPNeighborTypeExternal,
				Actual: []apiutil.LLDPNeighbor{{Name: "DS2000-01", Port: "Ethernet27"}},
			},
		},
		{
			// a port that isn't in the wiring has no expectation to check against
			name: "nothing expected",
			status: apiutil.LLDPNeighborStatus{
				Actual: []apiutil.LLDPNeighbor{lldpNeighborExtra, lldpNeighborWired},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			errs := lldpStrictErrs("sw", "E1/5/1", tt.status)

			got := make([]string, 0, len(errs))
			for _, err := range errs {
				got = append(got, err.Error())
			}

			if len(tt.wantErrs) == 0 {
				require.Empty(t, got)

				return
			}

			require.Equal(t, tt.wantErrs, got)
		})
	}
}

// lldpTestOut is the env-5 ds5000-02 shape: a port with an extra neighbor next to the wired one, a port where nothing
// matches, a port without neighbors and a management port.
func lldpTestOut(updated *kmetav1.Time) *LLDPOut {
	return &LLDPOut{
		Neighbors: map[string]map[string]apiutil.LLDPNeighborStatus{
			"ds5000-02": {
				"E1/5/1": {
					ConnectionName: "server-5--unbundled--ds5000-02",
					ConnectionType: "unbundled",
					Type:           apiutil.LLDPNeighborTypeServer,
					Expected:       apiutil.LLDPNeighbor{Name: "server-5", Port: "enp2s1"},
					Actual: []apiutil.LLDPNeighbor{
						lldpNeighborExtra,
						{
							Name:       lldpNeighborWired.Name,
							Port:       lldpNeighborWired.Port,
							MAC:        lldpNeighborWired.MAC,
							TTL:        lldpNeighborWired.TTL,
							LastUpdate: updated,
						},
					},
				},
				"E1/1/1": {
					ConnectionName: "ds5000-02--spark-1-2",
					ConnectionType: "unbundled",
					Type:           apiutil.LLDPNeighborTypeServer,
					Expected:       apiutil.LLDPNeighbor{Name: "spark-1", Port: "p2"},
					Actual: []apiutil.LLDPNeighbor{
						{Name: "spark-1.lan", Port: "enP2p1s0f1np1", MAC: "4c:bb:47:e8:ef:ff"},
						{Name: "spark-1.lan", Port: "enp1s0f1np1", MAC: "4c:bb:47:e8:ef:fb"},
					},
				},
				"E1/6/1": {
					ConnectionName: "server-6--unbundled--ds5000-02",
					ConnectionType: "unbundled",
					Type:           apiutil.LLDPNeighborTypeServer,
					Expected:       apiutil.LLDPNeighbor{Name: "server-6", Port: "enp2s1"},
				},
				// external connections don't expect a neighbor, there is nothing to diff against
				"E1/66": {
					ConnectionName: "ds5000-02--external--5835",
					ConnectionType: "external",
					Type:           apiutil.LLDPNeighborTypeExternal,
					Actual: []apiutil.LLDPNeighbor{
						{Name: "DS2000-01", Port: "Ethernet27", MAC: "0c:48:c6:15:38:83", Description: "some switch"},
					},
				},
				"M1": {
					Actual: []apiutil.LLDPNeighbor{{Name: "ds3000-06", Port: "eth0", MAC: "0c:48:c6:97:3b:12"}},
				},
			},
		},
	}
}

func TestLLDPOutMarshalText(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	updated := kmetav1.Time{Time: now.Add(-30 * time.Second)}

	// red() is a no-op as isatty is false under go test, so the output is stable
	res, err := lldpTestOut(&updated).MarshalText(LLDPIn{}, now)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(res), "\n")
	require.Equal(t, "Switch: ds5000-02", lines[0])
	require.Contains(t, lines[1], "MAC")
	require.Contains(t, lines[1], "AGE")

	// the +N marker is explained right after the table
	require.Equal(t, "Note: +N means N more neighbors on the port, use --show-all to see them", lines[len(lines)-1])

	rows := lines[2 : len(lines)-1]
	require.Len(t, rows, 5, "two rows for the unmatched port, one each for the other three, none for M1")

	// nothing matches, so both neighbors are rendered with their diffs and the order is kept
	require.Contains(t, rows[0], "E1/1/1")
	require.Contains(t, rows[0], "spark-1.lan (want spark-1)")
	require.Contains(t, rows[0], "enP2p1s0f1np1 (want p2)")
	require.Contains(t, rows[1], "enp1s0f1np1 (want p2)")

	// the wired neighbor is the only one rendered, marked with the number of the ones left out
	require.Contains(t, rows[2], "E1/5/1")
	require.Contains(t, rows[2], "server-5 +1")
	require.Contains(t, rows[2], "00:00:00:0b:bb:11")
	require.Contains(t, rows[2], "30s")
	require.NotContains(t, rows[2], "(want")
	require.NotContains(t, res, "00:00:00:0b:bb:19", "the extra neighbor is not rendered without show all")
	require.NotContains(t, res, lldpExtraNeighbor)

	// port without any neighbor still reports what the wiring expected on it
	require.Contains(t, rows[3], "E1/6/1")
	require.Contains(t, rows[3], "server-6--unbundled--ds5000-02")
	require.Contains(t, rows[3], "(want server-6)")
	require.Contains(t, rows[3], "(want enp2s1)")

	// the external neighbor is reported as is, diffing it against an expectation that doesn't exist means nothing
	require.Contains(t, rows[4], "E1/66")
	require.Contains(t, rows[4], "DS2000-01")
	require.Contains(t, rows[4], "Ethernet27")
	require.NotContains(t, rows[4], "(want")

	// descriptions are only rendered on demand, they're way too long for a table
	require.NotContains(t, res, "DESCRIPTION")

	require.NotContains(t, res, "M1")
	require.NotContains(t, res, "0c:48:c6:97:3b:12")
}

func TestLLDPOutMarshalTextDescription(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	out := lldpTestOut(nil)
	// a fabric port where the description is the only thing that doesn't match
	out.Neighbors["ds5000-02"]["E1/63/1"] = apiutil.LLDPNeighborStatus{
		ConnectionName: "ds5000-03--fabric--ds5000-02",
		ConnectionType: "fabric",
		Type:           apiutil.LLDPNeighborTypeFabric,
		Expected:       apiutil.LLDPNeighbor{Name: "ds5000-03", Port: "E1/63/1", Description: "Hedgehog Fabric"},
		Actual: []apiutil.LLDPNeighbor{
			{Name: "ds5000-03", Port: "E1/63/1", Description: "Hedgehog Fabric 5835", MAC: "b4:db:91:9b:60:27"},
		},
	}

	res, err := out.MarshalText(LLDPIn{Description: true, ShowAll: true}, now)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(res), "\n")
	require.Contains(t, lines[1], "DESCRIPTION")

	rows := lines[2:]
	// the fabric description is diffed like the other expected values
	require.Contains(t, strings.Join(rows, "\n"), "Hedgehog Fabric 5835 (want Hedgehog Fabric)")

	// extras report their description as is, there is nothing to compare it to
	for _, row := range rows {
		if strings.Contains(row, lldpExtraNeighbor) {
			require.NotContains(t, row, "(want")
		}
	}

	// server connections don't expect a description, so it must not be diffed against an empty one
	out = lldpTestOut(nil)
	out.Neighbors["ds5000-02"]["E1/5/1"].Actual[1].Description = "Ubuntu 24.04"

	res, err = out.MarshalText(LLDPIn{Description: true}, now)
	require.NoError(t, err)
	require.Contains(t, res, "Ubuntu 24.04")
	require.NotContains(t, res, "Ubuntu 24.04 (want")
}

func TestLLDPOutMarshalTextTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	updated := kmetav1.Time{Time: now.Add(-30 * time.Second)}

	res, err := lldpTestOut(&updated).MarshalText(LLDPIn{TTL: true, ShowAll: true}, now)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(res), "\n")
	require.Contains(t, lines[1], "AGE / TTL")

	// age and TTL are both seconds, so they share the suffix
	require.Contains(t, res, "30/120s")

	// the extra neighbor advertises a TTL but has no last update reported
	require.Contains(t, res, "-/120s")

	// without a TTL only the age is rendered, with its own suffix
	res, err = lldpTestOut(&updated).MarshalText(LLDPIn{TTL: false}, now)
	require.NoError(t, err)
	require.Contains(t, strings.Split(strings.TrimSpace(res), "\n")[1], "AGE")
	require.NotContains(t, res, "TTL")
	require.Contains(t, res, "30s")
	require.NotContains(t, res, "120s")
}

func TestLLDPOutMarshalTextShowAll(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	res, err := lldpTestOut(nil).MarshalText(LLDPIn{ShowAll: true}, now)
	require.NoError(t, err)

	rows := strings.Split(strings.TrimSpace(res), "\n")[2:]
	require.Len(t, rows, 6, "the extra neighbor of E1/5/1 is rendered as well now")
	require.NotContains(t, res, "Note:", "nothing is left out, so there is no marker to explain")

	// the wired neighbor comes first with its connection type, with nothing left out to report
	require.Contains(t, rows[2], "E1/5/1")
	require.Contains(t, rows[2], "server-5--unbundled--ds5000-02")
	require.Contains(t, rows[2], "unbundled")
	require.Contains(t, rows[2], "00:00:00:0b:bb:11")
	require.NotContains(t, rows[2], "+1")

	// the extra one follows it, keeping the connection name but marked as an extra instead of the connection type
	require.Contains(t, rows[3], "E1/5/1")
	require.Contains(t, rows[3], "server-5--unbundled--ds5000-02")
	require.Contains(t, rows[3], lldpExtraNeighbor)
	require.Contains(t, rows[3], "00:00:00:0b:bb:19")
	require.Contains(t, rows[3], "-", "no last update reported")
	require.NotContains(t, rows[3], "(want")

	// unmatched ports are unaffected by show all
	require.Contains(t, rows[0], "spark-1.lan (want spark-1)")
	require.Contains(t, rows[1], "enp1s0f1np1 (want p2)")
	require.Contains(t, rows[4], "E1/6/1")
}
