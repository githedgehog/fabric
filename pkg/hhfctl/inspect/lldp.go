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
}

type LLDPOut struct {
	Neighbors map[string]map[string]apiutil.LLDPNeighborStatus `json:"neighbors"`
	Errs      []error                                          `json:"errors"`
}

const lldpExtraNeighbor = "<extra>"

// lldpNeighborRow is a single LLDP neighbor to render, extra means it isn't the neighbor the wiring expects on that
// port while some other neighbor on the same port is.
type lldpNeighborRow struct {
	neighbor apiutil.LLDPNeighbor
	extra    bool
}

// lldpMatchingNeighbor returns the neighbor of the port that's exactly what the wiring expects, if there is one. Every
// expected value has to match: a neighbor with the right name on a wrong port doesn't make the port correct.
func lldpMatchingNeighbor(n apiutil.LLDPNeighborStatus) (apiutil.LLDPNeighbor, bool) {
	// nothing is expected on the external connections and on the ports that aren't in the wiring at all
	if n.Expected.Name == "" {
		return apiutil.LLDPNeighbor{}, false
	}

	for _, actual := range n.Actual {
		if actual.Name != n.Expected.Name || actual.Port != n.Expected.Port {
			continue
		}

		// only the fabric connections advertise an expected description
		if n.Expected.Description != "" && n.Expected.Description != actual.Description {
			continue
		}

		return actual, true
	}

	return apiutil.LLDPNeighbor{}, false
}

// lldpNeighborRows orders the neighbors of a port for rendering and reports how many of them are left out: if one of
// them is exactly what the wiring expects it goes first and the rest are just noise on an otherwise correct port, so
// they are only rendered with showAll. Without a match nothing is known to be right, so all of them are rendered as
// candidates.
func lldpNeighborRows(n apiutil.LLDPNeighborStatus, showAll bool) ([]lldpNeighborRow, int) {
	rows := make([]lldpNeighborRow, 0, len(n.Actual))

	if matching, ok := lldpMatchingNeighbor(n); ok {
		rows = append(rows, lldpNeighborRow{neighbor: matching})
	}

	if len(rows) == 0 {
		for _, actual := range n.Actual {
			rows = append(rows, lldpNeighborRow{neighbor: actual})
		}

		return rows, 0
	}

	if !showAll {
		return rows, len(n.Actual) - 1
	}

	for _, actual := range n.Actual {
		if actual != rows[0].neighbor {
			rows = append(rows, lldpNeighborRow{neighbor: actual, extra: true})
		}
	}

	return rows, 0
}

// lldpStrictErrs reports everything that doesn't match the wiring on a single port. Nothing is reported when there is
// nothing to compare against: the external connections don't expect a neighbor and neither do the ports the wiring
// doesn't mention. A port that has exactly the neighbor it's wired to is fine no matter what else shows up on it.
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
		if actual.Name != n.Expected.Name {
			// a neighbor that doesn't advertise a name is only identifiable by its MAC or port
			unexpected = append(unexpected, cmp.Or(actual.Name, actual.MAC, actual.Port))

			continue
		}

		found = true

		if n.Expected.Port != actual.Port {
			errs = append(errs, fmt.Errorf("switch %s: %s: expected neighbor port %q, got %q", swName, port, n.Expected.Port, actual.Port)) //nolint:goerr113
		}

		if n.Expected.Description != "" && n.Expected.Description != actual.Description {
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

// lldpNeighborAge renders how long ago the neighbor was last updated and, if asked for, the TTL it advertises. Always
// in seconds so both are directly comparable, an age above the TTL means the neighbor is about to expire. A negative
// age means the switch clock is ahead of ours, it's reported as is instead of being clamped.
func lldpNeighborAge(now time.Time, neighbor apiutil.LLDPNeighbor, withTTL bool) string {
	age := "-"
	if neighbor.LastUpdate != nil && !neighbor.LastUpdate.IsZero() {
		age = fmt.Sprintf("%d", int(now.Sub(neighbor.LastUpdate.Time).Seconds()))
	}

	// both are seconds, so a single unit suffix covers the pair
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
	if noColor {
		red = fmt.Sprint
	}

	str := &strings.Builder{}

	age := "Age"
	if in.TTL {
		age = "Age/TTL"
	}

	headers := []string{"Port", "Connection", "Type", "Neighbor", "Port", "MAC", age}

	// descriptions are long enough to make the table unreadable, so they're only rendered on demand
	if in.Description {
		headers = append(headers, "Description")
	}

	// appendRow adds the optional description cell to a row before storing it
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

			// values the wiring doesn't define are reported as is, there is nothing to compare them to: the
			// external connections don't expect a neighbor at all and only the fabric ones expect a description
			diff := func(actual, expected string) string {
				if expected == "" || expected == actual {
					return actual
				}

				want := "(want " + expected + ")"
				external := n.Type == apiutil.LLDPNeighborTypeExternal

				// nothing is reported, so the expectation itself has to carry the highlight
				if actual == "" {
					if external {
						return want
					}

					return red(want)
				}

				// the expectation isn't what's wrong here, only the actual value is highlighted
				if !external {
					actual = red(actual)
				}

				return actual + " " + want
			}

			// nothing is on the port, but the wiring still says what should have been
			if len(n.Actual) == 0 {
				data = appendRow(data, []string{
					port, n.ConnectionName, n.ConnectionType,
					diff("", n.Expected.Name), diff("", n.Expected.Port), "", "",
				}, diff("", n.Expected.Description))

				continue
			}

			// a port can have more than one neighbor, each of the rendered ones gets its own row
			rows, hidden := lldpNeighborRows(n, in.ShowAll)

			for _, row := range rows {
				actual := row.neighbor

				// extras aren't the wired neighbor, so there is nothing to compare them to
				if row.extra {
					data = appendRow(data, []string{
						port, n.ConnectionName, lldpExtraNeighbor,
						actual.Name, actual.Port, actual.MAC, lldpNeighborAge(now, actual, in.TTL),
					}, actual.Description)

					continue
				}

				sn := diff(actual.Name, n.Expected.Name)
				sp := diff(actual.Port, n.Expected.Port)
				sd := diff(actual.Description, n.Expected.Description)

				// only happens on a full match, so sn is never a red diff at this point
				if hidden > 0 {
					sn += fmt.Sprintf(" +%d", hidden)
					anyHidden = true
				}

				// MAC and age are only reported by the neighbor, so there is nothing to compare them to
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

		neighbors, err := apiutil.GetLLDPNeighbors(ctx, kube, &sw)
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
