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
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/valid"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	coreapi "k8s.io/api/core/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kyaml "sigs.k8s.io/yaml"
)

type BGPIn struct {
	Switches []string
	// Fabric and Domain select the switches instead of the names
	Fabric string
	Domain string
	Strict bool
}

type BGPOut struct {
	Neighbors map[string]map[string]map[string]apiutil.BGPNeighborStatus `json:"neighbors"`
	Errs      []error                                                    `json:"errors"`
}

func (out *BGPOut) MarshalText(_ BGPIn, now time.Time) (string, error) {
	// TODO pass to a marshal func?
	noColor := !isatty.IsTerminal(os.Stdout.Fd())

	red := color.New(color.FgRed).SprintFunc()
	if noColor {
		red = fmt.Sprint
	}

	str := &strings.Builder{}

	for _, swName := range slices.Sorted(maps.Keys(out.Neighbors)) {
		str.WriteString("Switch: " + swName + "\n")

		data := [][]string{}

		for _, vrf := range slices.Sorted(maps.Keys(out.Neighbors[swName])) {
			for _, name := range slices.Sorted(maps.Keys(out.Neighbors[swName][vrf])) {
				n := out.Neighbors[swName][vrf][name]
				t := string(n.Type)
				if !n.Expected {
					if t != "" {
						t += " (unexpected)"
					} else {
						t = "unexpected"
					}

					t = red(t)
				}

				s := string(n.SessionState)
				if s != string(v1beta1.BGPNeighborSessionStateEstablished) {
					s = red(s)
				}

				last := "-"
				if !n.LastEstablished.IsZero() {
					last = HumanizeTime(now, n.LastEstablished.Time)
				}

				// an unnumbered session is keyed by the port it runs over, so say what it is
				neighbor := name
				if n.Unnumbered {
					neighbor += " IPv6 LL"
				}

				data = append(data, []string{
					t,
					n.Port,
					vrf,
					neighbor,
					n.RemoteName,
					n.ConnectionName,
					s,
					last,
					fmt.Sprintf("%d", n.EstablishedTransitions),
				})
			}
		}

		// sort by port, then vrf, then neighbor name for stable output
		slices.SortFunc(data, func(a, b []string) int {
			if c := comparePortNames(a[1], b[1]); c != 0 {
				return c
			}
			if c := strings.Compare(a[2], b[2]); c != 0 {
				return c
			}

			return strings.Compare(a[3], b[3])
		})

		str.WriteString(RenderTable(
			[]string{"Type", "Port", "VRF", "Neighbor", "RemoteName", "Connection", "State", "LastEstab", "Trans"},
			data,
		))
	}

	return str.String(), nil
}

func (out *BGPOut) Errors() []error {
	return out.Errs
}

var (
	_ Func[BGPIn, *BGPOut] = BGP
	_ WithErrors           = (*BGPOut)(nil)
)

func BGP(ctx context.Context, kube kclient.Reader, in BGPIn) (*BGPOut, error) {
	out := &BGPOut{
		Neighbors: map[string]map[string]map[string]apiutil.BGPNeighborStatus{},
	}

	status, err := getBGPStatus(ctx, kube, apiutil.SwitchFilter{Names: in.Switches, Fabric: in.Fabric, Domain: in.Domain})
	if err != nil {
		return nil, err
	}

	for _, swName := range slices.Sorted(maps.Keys(status)) {
		neighs := status[swName].Neighbors

		if in.Strict {
			for vrf, vrfNeighbors := range neighs {
				for name, neighbor := range vrfNeighbors {
					if !neighbor.Expected {
						out.Errs = append(out.Errs, fmt.Errorf("switch %s: vrf %s: unexpected neighbor %q", swName, vrf, name)) //nolint:goerr113
					}

					if neighbor.SessionState != v1beta1.BGPNeighborSessionStateEstablished {
						out.Errs = append(out.Errs, fmt.Errorf("switch %s: vrf %s: neighbor %q is not established", swName, vrf, name)) //nolint:goerr113
					}
				}
			}
		}

		out.Neighbors[swName] = neighs
	}

	return out, nil
}

// getBGPStatus is shared by the BGP and BFD inspects, both are computed from the same switch state
func getBGPStatus(ctx context.Context, kube kclient.Reader, filter apiutil.SwitchFilter) (map[string]*apiutil.BGPSwitchStatus, error) {
	if err := filter.Validate(); err != nil {
		return nil, fmt.Errorf("invalid switch filter: %w", err)
	}

	fabCfgCM := &coreapi.ConfigMap{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: "fabric-ctrl-config", Namespace: "fab"}, fabCfgCM); err != nil {
		return nil, fmt.Errorf("getting fabric-ctrl-config: %w", err)
	}

	fabCfg := &meta.FabricConfig{}
	if err := kyaml.UnmarshalStrict([]byte(fabCfgCM.Data["config.yaml"]), fabCfg); err != nil {
		return nil, fmt.Errorf("unmarshalling fabric config: %w", err)
	}

	if _, err := fabCfg.Init(meta.ExtraValidators{
		Peering: valid.Peering,
	}); err != nil {
		return nil, fmt.Errorf("initializing fabric config: %w", err)
	}

	status, err := apiutil.GetBGPStatus(ctx, kube, fabCfg, filter)
	if err != nil {
		return nil, fmt.Errorf("getting BGP status: %w", err)
	}

	return status, nil
}
