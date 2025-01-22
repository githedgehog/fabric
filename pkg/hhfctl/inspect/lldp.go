// Copyright 2024 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LLDPIn struct {
	Switches []string
	Fabric   bool
	Server   bool
	External bool
	Strict   bool
}

type LLDPOut struct {
	Neighbors map[string]map[string]apiutil.LLDPNeighborStatus `json:"neighbors"`
	Errs      []error                                          `json:"errors"`
}

func (out *LLDPOut) MarshalText() (string, error) {
	// TODO pass to a marshal func?
	noColor := !isatty.IsTerminal(os.Stdout.Fd())

	red := color.New(color.FgRed).SprintFunc()
	if noColor {
		red = func(a ...interface{}) string { return fmt.Sprint(a...) }
	}

	str := &strings.Builder{}

	for _, swName := range slices.Sorted(maps.Keys(out.Neighbors)) {
		str.WriteString("Switch: " + swName + " (actual←→expected)\n")

		data := [][]string{}

		ports := slices.Collect(maps.Keys(out.Neighbors[swName]))
		wiringapi.SortPortNames(ports)

		for _, port := range ports {
			if strings.HasPrefix(port, wiringapi.ManagementPortPrefix) {
				continue
			}

			n := out.Neighbors[swName][port]

			sn := n.Actual.Name
			if n.Expected.Name != n.Actual.Name {
				sn += "←→" + n.Expected.Name
				if n.Type != apiutil.LLDPNeighborTypeExternal {
					sn = red(sn)
				}
			}

			sp := n.Actual.Port
			if n.Expected.Port != n.Actual.Port {
				sp += "←→" + n.Expected.Port
				if n.Type != apiutil.LLDPNeighborTypeExternal {
					sp = red(sp)
				}
			}

			sd := n.Actual.Description
			if n.Expected.Description != n.Actual.Description {
				sd += "←→" + n.Expected.Description
				if n.Type != apiutil.LLDPNeighborTypeExternal {
					sd = red(sd)
				}
			}

			data = append(data, []string{port, n.ConnectionName, n.ConnectionType, sn, sp, sd})
		}

		str.WriteString(RenderTable(
			[]string{"Port", "Connection", "Type", "Neighbor", "Port", "Description"},
			data,
		))
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

func LLDP(ctx context.Context, kube client.Reader, in LLDPIn) (*LLDPOut, error) {
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

			out.Neighbors[sw.Name][name] = n

			if n.Type != apiutil.LLDPNeighborTypeExternal && in.Strict {
				if n.Expected.Name != n.Actual.Name {
					out.Errs = append(out.Errs, fmt.Errorf("switch %s: %s: expected neighbor %q, got %q", sw.Name, name, n.Expected.Name, n.Actual.Name)) //nolint:goerr113
				}

				if n.Expected.Port != n.Actual.Port {
					out.Errs = append(out.Errs, fmt.Errorf("switch %s: %s: expected neighbor port %q, got %q", sw.Name, name, n.Expected.Port, n.Actual.Port)) //nolint:goerr113
				}

				if n.Expected.Description != "" && n.Expected.Description != n.Actual.Description {
					out.Errs = append(out.Errs, fmt.Errorf("switch %s: %s: expected neighbor description %q, got %q", sw.Name, name, n.Expected.Description, n.Actual.Description)) //nolint:goerr113
				}
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
