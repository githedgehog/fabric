// Copyright 2024 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type LLDPIn struct {
	Switches    []string
	Fabric      bool
	Server      bool
	External    bool
	Gateway     bool
	Strict      bool
	ShowAll     bool
	Description bool
	TTL         bool

	// Unset means nothing is ignored, see the apiutil.DefaultLLDPIgnore* for the usual ones
	IgnorePrefixes []string
	IgnoreSuffixes []string
}

type LLDPOut struct {
	Neighbors map[string]map[string]apiutil.LLDPNeighborStatus `json:"neighbors"`
	Errs      []error                                          `json:"errors"`
}

const lldpExtraNeighbor = "<extra>"

// extra means another neighbor on the same port is the wired one.
type lldpNeighborRow struct {
	neighbor apiutil.LLDPNeighbor
	extra    bool
}

// lldpMatchingNeighborIdx returns the index of the neighbor that's exactly what the wiring expects, or -1. It's the
// index and not the neighbor itself so that the caller can tell two identically reported neighbors apart.
func lldpMatchingNeighborIdx(n apiutil.LLDPNeighborStatus) int {
	// nothing is expected on the external connections and on the ports not in the wiring
	if n.Expected.Name == "" {
		return -1
	}

	for i, actual := range n.Actual {
		if actual.Matches(n.Expected) {
			return i
		}
	}

	return -1
}

// lldpMatchingNeighbor returns the neighbor of the port that's exactly what the wiring expects, if there is one.
func lldpMatchingNeighbor(n apiutil.LLDPNeighborStatus) (apiutil.LLDPNeighbor, bool) {
	if idx := lldpMatchingNeighborIdx(n); idx >= 0 {
		return n.Actual[idx], true
	}

	return apiutil.LLDPNeighbor{}, false
}

// lldpNeighborRows orders the neighbors of a port for rendering and reports how many are left out: the wired one wins
// and the rest are only rendered with showAll, without a match they're all candidates.
func lldpNeighborRows(n apiutil.LLDPNeighborStatus, showAll bool) ([]lldpNeighborRow, int) {
	rows := make([]lldpNeighborRow, 0, len(n.Actual))

	match := lldpMatchingNeighborIdx(n)
	if match < 0 {
		for _, actual := range n.Actual {
			rows = append(rows, lldpNeighborRow{neighbor: actual})
		}

		return rows, 0
	}

	rows = append(rows, lldpNeighborRow{neighbor: n.Actual[match]})

	if !showAll {
		return rows, len(n.Actual) - 1
	}

	for idx, actual := range n.Actual {
		if idx != match {
			rows = append(rows, lldpNeighborRow{neighbor: actual, extra: true})
		}
	}

	return rows, 0
}

// lldpStrictErrs reports what doesn't match the wiring on a port. A port that has the neighbor it's wired to is fine
// no matter what else shows up on it, and one with nothing expected has nothing to be wrong about.
func lldpStrictErrs(swName, port string, n apiutil.LLDPNeighborStatus) []error {
	if n.Type == apiutil.LLDPNeighborTypeExternal || n.Expected.Name == "" {
		return nil
	}

	if _, ok := lldpMatchingNeighbor(n); ok {
		return nil
	}

	var errs []error
	found := false
	unexpected := []string{}

	for _, actual := range n.Actual {
		if !strings.EqualFold(actual.MatchedName(), n.Expected.Name) {
			// a neighbor that doesn't advertise a name is only identifiable by its MAC or port
			unexpected = append(unexpected, cmp.Or(actual.Name, actual.MAC, actual.Port))

			continue
		}

		found = true

		if !strings.EqualFold(n.Expected.Port, actual.Port) {
			errs = append(errs, fmt.Errorf("switch %s: %s: expected neighbor port %q, got %q", swName, port, n.Expected.Port, actual.Port)) //nolint:goerr113
		}

		if n.Expected.Description != "" && !strings.EqualFold(n.Expected.Description, actual.Description) {
			errs = append(errs, fmt.Errorf("switch %s: %s: expected neighbor description %q, got %q", swName, port, n.Expected.Description, actual.Description)) //nolint:goerr113
		}
	}

	if !found {
		if len(unexpected) == 0 {
			errs = append(errs, fmt.Errorf("switch %s: %s: expected neighbor %q not found", swName, port, n.Expected.Name)) //nolint:goerr113
		} else {
			errs = append(errs, fmt.Errorf("switch %s: %s: expected neighbor %q not found, but found: %v", swName, port, n.Expected.Name, unexpected)) //nolint:goerr113
		}
	}

	return errs
}

// lldpNeighborAge renders how long ago the neighbor was updated and optionally its TTL, both in seconds so that they
// are comparable. A negative age means the switch clock is ahead of ours and is reported as is.
func lldpNeighborAge(now time.Time, neighbor apiutil.LLDPNeighbor, withTTL bool) string {
	age := "-"
	if neighbor.LastUpdate != nil && !neighbor.LastUpdate.IsZero() {
		age = fmt.Sprintf("%d", int(now.Sub(neighbor.LastUpdate.Time).Seconds()))
	}

	// both are seconds, so one unit suffix covers the pair
	if withTTL && neighbor.TTL > 0 {
		return fmt.Sprintf("%s/%ds", age, neighbor.TTL)
	}

	if age == "-" {
		return age
	}

	return age + "s"
}

func (out *LLDPOut) MarshalText(in LLDPIn, now time.Time) (string, error) {
	// TODO pass to a marshal func?
	noColor := !isatty.IsTerminal(os.Stdout.Fd())

	red := color.New(color.FgRed).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	if noColor {
		red = fmt.Sprint
		dim = fmt.Sprint
	}

	str := &strings.Builder{}

	age := "Age"
	if in.TTL {
		age = "Age/TTL"
	}

	headers := []string{"Port", "Connection", "Type", "Neighbor", "Port", "MAC", age}

	// descriptions are long enough to make the table unreadable
	if in.Description {
		headers = append(headers, "Description")
	}

	// appendRow adds the optional description cell
	appendRow := func(data [][]string, row []string, descr string) [][]string {
		if in.Description {
			row = append(row, descr)
		}

		return append(data, row)
	}

	for _, swName := range slices.Sorted(maps.Keys(out.Neighbors)) {
		str.WriteString("Switch: " + swName + "\n")

		data := [][]string{}
		anyHidden := false

		ports := slices.Collect(maps.Keys(out.Neighbors[swName]))
		wiringapi.SortPortNames(ports)

		for _, port := range ports {
			if strings.HasPrefix(port, wiringapi.ManagementPortPrefix) {
				continue
			}

			n := out.Neighbors[swName][port]

			// values the wiring doesn't define are reported as is, there is nothing to compare them to
			diff := func(actual, expected string) string {
				if expected == "" || strings.EqualFold(expected, actual) {
					return actual
				}

				want := "(want " + expected + ")"
				external := n.Type == apiutil.LLDPNeighborTypeExternal

				// nothing is reported, so the expectation carries the highlight
				if actual == "" {
					if external {
						return want
					}

					return red(want)
				}

				// the expectation isn't what's wrong, only the actual value
				if !external {
					actual = red(actual)
				}

				return actual + " " + want
			}

			// nothing on the port, but the wiring still says what should have been
			if len(n.Actual) == 0 {
				data = appendRow(data, []string{
					port, n.ConnectionName, n.ConnectionType,
					diff("", n.Expected.Name), diff("", n.Expected.Port), "", "",
				}, diff("", n.Expected.Description))

				continue
			}

			// a port can have more than one neighbor, each rendered one gets its own row
			rows, hidden := lldpNeighborRows(n, in.ShowAll)

			for _, row := range rows {
				actual := row.neighbor

				// extras aren't the wired neighbor, nothing to compare them to
				if row.extra {
					data = appendRow(data, []string{
						port, n.ConnectionName, lldpExtraNeighbor,
						actual.Name, actual.Port, actual.MAC, lldpNeighborAge(now, actual, in.TTL),
					}, actual.Description)

					continue
				}

				// reported as advertised, with the ignored parts dimmed so it's visible why it still matches
				sn := actual.Name
				if !strings.EqualFold(actual.MatchedName(), n.Expected.Name) {
					sn = diff(actual.Name, n.Expected.Name)
				} else if actual.IgnoredPrefix != "" || actual.IgnoredSuffix != "" {
					sn = dim(actual.IgnoredPrefix) + actual.MatchedName() + dim(actual.IgnoredSuffix)
				}

				sp := diff(actual.Port, n.Expected.Port)
				sd := diff(actual.Description, n.Expected.Description)

				// only happens on a full match, so sn is never a red diff here
				if hidden > 0 {
					sn += fmt.Sprintf(" +%d", hidden)
					anyHidden = true
				}

				// MAC and age are only reported by the neighbor, nothing to compare them to
				data = appendRow(data, []string{port, n.ConnectionName, n.ConnectionType, sn, sp, actual.MAC, lldpNeighborAge(now, actual, in.TTL)}, sd)
			}
		}

		str.WriteString(RenderTable(headers, data))

		if anyHidden {
			str.WriteString("Note: +N means N more neighbors on the port, use --show-all to see them\n")
		}
	}

	return str.String(), nil
}

func (out *LLDPOut) Errors() []error {
	return out.Errs
}

var (
	_ Func[LLDPIn, *LLDPOut] = LLDP
	_ WithErrors             = (*LLDPOut)(nil)
)

func LLDP(ctx context.Context, kube kclient.Reader, in LLDPIn) (*LLDPOut, error) {
	out := &LLDPOut{
		Neighbors: map[string]map[string]apiutil.LLDPNeighborStatus{},
	}

	sws := &wiringapi.SwitchList{}
	if err := kube.List(ctx, sws); err != nil {
		return nil, fmt.Errorf("listing switches: %w", err)
	}

	for _, sw := range sws.Items {
		if len(in.Switches) > 0 && !slices.Contains(in.Switches, sw.Name) {
			continue
		}

		neighbors, err := apiutil.GetLLDPNeighbors(ctx, kube, &sw, apiutil.LLDPNeighborsOpts{
			IgnorePrefixes: in.IgnorePrefixes,
			IgnoreSuffixes: in.IgnoreSuffixes,
		})
		if err != nil {
			return nil, fmt.Errorf("getting lldp neighbors for %s: %w", sw.Name, err)
		}

		out.Neighbors[sw.Name] = map[string]apiutil.LLDPNeighborStatus{}

		for name, n := range neighbors {
			if !in.Fabric && n.Type == apiutil.LLDPNeighborTypeFabric {
				continue
			}

			if !in.Server && n.Type == apiutil.LLDPNeighborTypeServer {
				continue
			}

			if !in.External && n.Type == apiutil.LLDPNeighborTypeExternal {
				continue
			}

			if !in.Gateway && n.Type == apiutil.LLDPNeighborTypeGateway {
				continue
			}

			out.Neighbors[sw.Name][name] = n

			if in.Strict {
				out.Errs = append(out.Errs, lldpStrictErrs(sw.Name, name, n)...)
			}
		}
	}

	for _, sw := range in.Switches {
		if _, ok := out.Neighbors[sw]; !ok {
			return nil, fmt.Errorf("switch %s not found", sw) //nolint:goerr113
		}
	}

	return out, nil
}
