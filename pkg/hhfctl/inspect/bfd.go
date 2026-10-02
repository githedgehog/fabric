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
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type BFDIn struct {
	Switches []string
	// Fabric and Domain select the switches instead of the names
	Fabric string
	Domain string
	Strict bool
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

				last := "-"
				if !p.LastUpTime.IsZero() {
					last = HumanizeTime(now, p.LastUpTime.Time)
				}

				// an unnumbered session is keyed by the port it runs over, so say what it is
				peer := addr
				if p.Unnumbered {
					peer += " IPv6 LL"
				}

				data = append(data, []string{
					t,
					p.Port,
					vrf,
					peer,
					p.RemoteName,
					p.ConnectionName,
					s,
					last,
					fmt.Sprintf("%d", p.FailureTransitions),
				})
			}
		}

		str.WriteString(RenderTable(
			[]string{"Type", "Port", "VRF", "Peer", "RemoteName", "Connection", "State", "LastUp", "Trans"},
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

	status, err := getBGPStatus(ctx, kube, apiutil.SwitchFilter{Names: in.Switches, Fabric: in.Fabric, Domain: in.Domain})
	if err != nil {
		return nil, err
	}

	for _, swName := range slices.Sorted(maps.Keys(status)) {
		peers := status[swName].BFDPeers

		if in.Strict {
			for vrf, vrfPeers := range peers {
				for addr, peer := range vrfPeers {
					if !peer.Expected {
						out.Errs = append(out.Errs, fmt.Errorf("switch %s: vrf %s: unexpected BFD peer %q", swName, vrf, addr)) //nolint:goerr113
					}

					if peer.SessionState == v1beta1.BFDSessionStateUnset {
						out.Errs = append(out.Errs, fmt.Errorf("switch %s: vrf %s: expected BFD peer %q is missing", swName, vrf, addr)) //nolint:goerr113
					} else if peer.SessionState != v1beta1.BFDSessionStateUp {
						out.Errs = append(out.Errs, fmt.Errorf("switch %s: vrf %s: BFD peer %q is not up (state: %s)", swName, vrf, addr, peer.SessionState)) //nolint:goerr113
					}
				}
			}
		}

		out.Peers[swName] = peers
	}

	return out, nil
}
