// Copyright 2025 Hedgehog
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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	coreapi "k8s.io/api/core/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kyaml "sigs.k8s.io/yaml"
)

type BFDIn struct {
	Switches []string
	Strict   bool
}

type BFDOut struct {
	Peers map[string]map[string]map[string]apiutil.BFDPeerStatus `json:"peers"`
	Errs  []error                                                `json:"errors"`
}

func (out *BFDOut) MarshalText(_ BFDIn, now time.Time) (string, error) {
	noColor := !isatty.IsTerminal(os.Stdout.Fd())

	red := color.New(color.FgRed).SprintFunc()
	if noColor {
		red = fmt.Sprint
	}

	str := &strings.Builder{}

	for _, swName := range slices.Sorted(maps.Keys(out.Peers)) {
		str.WriteString("Switch: " + swName + "\n")

		data := [][]string{}

		for _, vrf := range slices.Sorted(maps.Keys(out.Peers[swName])) {
			for _, addr := range slices.Sorted(maps.Keys(out.Peers[swName][vrf])) {
				p := out.Peers[swName][vrf][addr]
				t := string(p.Type)
				if !p.Expected {
					if t != "" {
						t += " (unexpected)"
					} else {
						t = "unexpected"
					}

					t = red(t)
				}

				s := string(p.SessionState)
				if s == "" {
					s = "-"
				}
				if s != string(v1beta1.BFDSessionStateUp) {
					s = red(s)
				}

				uptime := "-"
				if !p.LastUpTime.IsZero() {
					uptime = HumanizeTime(now, p.LastUpTime.Time)
				}

				data = append(data, []string{
					t,
					p.Port,
					vrf,
					addr,
					p.RemoteName,
					p.ConnectionName,
					s,
					fmt.Sprintf("%d", p.FailureTransitions),
					uptime,
				})
			}
		}

		str.WriteString(RenderTable(
			[]string{"Type", "Port", "VRF", "Peer", "RemoteName", "Connection", "Status", "Fails", "Uptime"},
			data,
		))
	}

	return str.String(), nil
}

func (out *BFDOut) Errors() []error {
	return out.Errs
}

var (
	_ Func[BFDIn, *BFDOut] = BFD
	_ WithErrors           = (*BFDOut)(nil)
)

func BFD(ctx context.Context, kube kclient.Reader, in BFDIn) (*BFDOut, error) {
	out := &BFDOut{
		Peers: map[string]map[string]map[string]apiutil.BFDPeerStatus{},
	}

	fabCfgCM := &coreapi.ConfigMap{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: "fabric-ctrl-config", Namespace: "fab"}, fabCfgCM); err != nil {
		return nil, fmt.Errorf("getting fabric-ctrl-config: %w", err)
	}

	fabCfg := &meta.FabricConfig{}
	if err := kyaml.UnmarshalStrict([]byte(fabCfgCM.Data["config.yaml"]), fabCfg); err != nil {
		return nil, fmt.Errorf("unmarshalling fabric config: %w", err)
	}

	if _, err := fabCfg.Init(); err != nil {
		return nil, fmt.Errorf("initializing fabric config: %w", err)
	}

	sws := &wiringapi.SwitchList{}
	if err := kube.List(ctx, sws); err != nil {
		return nil, fmt.Errorf("listing switches: %w", err)
	}

	for _, sw := range sws.Items {
		if len(in.Switches) > 0 && !slices.Contains(in.Switches, sw.Name) {
			continue
		}

		peers, err := apiutil.GetBFDPeers(ctx, kube, fabCfg, &sw)
		if err != nil {
			return nil, fmt.Errorf("getting BFD peers for switch %s: %w", sw.Name, err)
		}

		if in.Strict {
			for vrf, vrfPeers := range peers {
				for addr, peer := range vrfPeers {
					if !peer.Expected {
						out.Errs = append(out.Errs, fmt.Errorf("switch %s: vrf %s: unexpected BFD peer %q", sw.Name, vrf, addr)) //nolint:goerr113
					}

					if peer.SessionState == v1beta1.BFDSessionStateUnset {
						out.Errs = append(out.Errs, fmt.Errorf("switch %s: vrf %s: expected BFD peer %q is missing", sw.Name, vrf, addr)) //nolint:goerr113
					} else if peer.SessionState != v1beta1.BFDSessionStateUp {
						out.Errs = append(out.Errs, fmt.Errorf("switch %s: vrf %s: BFD peer %q is not up (state: %s)", sw.Name, vrf, addr, peer.SessionState)) //nolint:goerr113
					}
				}
			}
		}

		out.Peers[sw.Name] = peers
	}

	for _, sw := range in.Switches {
		if _, ok := out.Peers[sw]; !ok {
			return nil, fmt.Errorf("switch %s not found", sw) //nolint:goerr113
		}
	}

	return out, nil
}
