// Copyright 2023 Hedgehog
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
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kyaml "sigs.k8s.io/yaml"
)

// TODO dedup with conn

type PortIn struct {
	Port string
}

type PortOut struct {
	ConnectionName      *string                                   `json:"connectionName,omitempty"`
	Connection          *wiringapi.ConnectionSpec                 `json:"connection,omitempty"`
	InterfaceState      *agentapi.SwitchStateInterface            `json:"interfaceState,omitempty"`
	BreakoutState       *agentapi.SwitchStateBreakout             `json:"breakoutState,omitempty"`
	VPCAttachments      map[string]*vpcapi.VPCAttachmentSpec      `json:"vpcAttachments,omitempty"`
	AttachedVPCs        map[string]*vpcapi.VPCSpec                `json:"attachedVPCs,omitempty"`
	ExternalAttachments map[string]*vpcapi.ExternalAttachmentSpec `json:"externalAttachments,omitempty"` // if External conn
	LoopbackWorkarounds map[string]*OutLoopbackWorkaround         `json:"loopbackWorkarounds,omitempty"` // if VPCLoopback conn
}

func (out *PortOut) MarshalText(now time.Time) (string, error) {
	str := strings.Builder{}

	if out.ConnectionName != nil && out.Connection != nil {
		str.WriteString(fmt.Sprintf("Used in Connection %s:\n", *out.ConnectionName))

		data, err := kyaml.Marshal(out.Connection)
		if err != nil {
			return "", errors.Wrap(err, "failed to marshal Connection")
		}
		str.WriteString(string(data))
	}

	if out.InterfaceState != nil && out.InterfaceState.Counters != nil {
		counters := out.InterfaceState.Counters

		lastClear := "-"
		if !counters.LastClear.IsZero() {
			lastClear = HumanizeTime(now, counters.LastClear.Time)
		}

		str.WriteString("\nPort Counters (↓ In ↑ Out):\n")

		str.WriteString(RenderTable(
			[]string{"Speed", "Util %", "Bits/sec In", "Bits/sec Out", "Pkts/sec In", "Pkts/sec Out", "Clear", "Errors", "Discards"},
			[][]string{
				{
					out.InterfaceState.Speed,
					fmt.Sprintf("↓ %3d ↑ %3d ", counters.InUtilization, counters.OutUtilization),
					fmt.Sprintf("↓ %s", humanize.CommafWithDigits(counters.InBitsPerSecond, 0)),
					fmt.Sprintf("↑ %s", humanize.CommafWithDigits(counters.OutBitsPerSecond, 0)),
					fmt.Sprintf("↓ %s", humanize.CommafWithDigits(counters.InPktsPerSecond, 0)),
					fmt.Sprintf("↑ %s", humanize.CommafWithDigits(counters.OutPktsPerSecond, 0)),
					lastClear,
					fmt.Sprintf("↓ %d ↑ %d ", counters.InErrors, counters.OutErrors),
					fmt.Sprintf("↓ %d ↑ %d", counters.InDiscards, counters.OutDiscards),
				},
			},
		))
	}

	if out.BreakoutState != nil {
		str.WriteString("Breakout State:\n")
		str.WriteString(fmt.Sprintf("  Mode: %s\n", out.BreakoutState.Mode))
		str.WriteString(fmt.Sprintf("  Status: %s\n", out.BreakoutState.Status))
	}

	if len(out.VPCAttachments) > 0 {
		str.WriteString("VPC Attachments:\n")

		attachData := [][]string{}
		attachNames := lo.Keys(out.VPCAttachments)
		for _, attachName := range attachNames {
			attach := out.VPCAttachments[attachName]

			subnet := ""
			vlan := ""
			vpcName := strings.SplitN(attach.Subnet, "/", 2)[0]
			subnetName := strings.SplitN(attach.Subnet, "/", 2)[1]
			if vpc, ok := out.AttachedVPCs[vpcName]; ok {
				if vpcSubnet, ok := vpc.Subnets[subnetName]; ok {
					subnet = vpcSubnet.Subnet
					vlan = fmt.Sprintf("%d", vpcSubnet.VLAN)

					if attach.NativeVLAN {
						vlan = "native"
					}
				}
			}

			attachData = append(attachData, []string{
				attachName,
				attach.Subnet,
				subnet,
				vlan,
			})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "VPCSubnet", "Subnet", "VLAN"},
			attachData,
		))
	}

	if len(out.ExternalAttachments) > 0 {
		str.WriteString("External Attachments:\n")

		attachData := [][]string{}
		attachNames := lo.Keys(out.ExternalAttachments)
		for _, attachName := range attachNames {
			attach := out.ExternalAttachments[attachName]

			attachData = append(attachData, []string{
				attachName,
				attach.External,
			})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "External"},
			attachData,
		))
	}

	if len(out.LoopbackWorkarounds) > 0 {
		str.WriteString("Loopback Workarounds:\n")

		// TODO pass to a marshal func?
		noColor := !isatty.IsTerminal(os.Stdout.Fd())

		// TODO dedup
		colored := color.New(color.FgCyan).SprintFunc()
		if noColor {
			colored = fmt.Sprint
		}

		sep := colored("←→")

		loWoData := [][]string{}
		for _, loWo := range out.LoopbackWorkarounds {
			vpcPeerings := []string{}
			for vpcPeeringName, vpcPeering := range loWo.VPCPeerings {
				vpc1, vpc2, err := vpcPeering.VPCs()
				if err != nil {
					return "", errors.Wrapf(err, "failed to get VPCs for VPCPeering %s", vpcPeeringName)
				}

				vpcPeerings = append(vpcPeerings, fmt.Sprintf("%s (%s%s%s)", vpcPeeringName, vpc1, sep, vpc2))
			}

			extPeerings := []string{}
			for extPeeringName, extPeering := range loWo.ExternalPeerings {
				extPeerings = append(extPeerings, fmt.Sprintf("%s (%s%s%s)", extPeeringName, extPeering.Permit.VPC.Name, sep, extPeering.Permit.External.Name))
			}

			loWoData = append(loWoData, []string{
				fmt.Sprintf("%s%s%s", loWo.Link.Switch1.PortName(), sep, loWo.Link.Switch2.PortName()),
				strings.Join(vpcPeerings, "\n"),
				strings.Join(extPeerings, "\n"),
			})
		}
		str.WriteString(RenderTable(
			[]string{"Link", "VPCPeerings", "ExternalPeerings"},
			loWoData,
		))
	}

	return str.String(), nil
}

var _ Func[PortIn, *PortOut] = Port

func Port(ctx context.Context, kube kclient.Reader, in PortIn) (*PortOut, error) {
	if in.Port == "" {
		return nil, errors.New("port is required")
	}

	out := &PortOut{
		VPCAttachments:      map[string]*vpcapi.VPCAttachmentSpec{},
		AttachedVPCs:        map[string]*vpcapi.VPCSpec{},
		ExternalAttachments: map[string]*vpcapi.ExternalAttachmentSpec{},
		LoopbackWorkarounds: map[string]*OutLoopbackWorkaround{},
	}

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns); err != nil {
		return nil, errors.Wrap(err, "cannot list Connections")
	}

	for _, conn := range conns.Items {
		_, _, ports, _, err := conn.Spec.Endpoints()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get connection %s endpoints", conn.Name)
		}

		if !(slices.Contains(ports, in.Port) || strings.Count(in.Port, "/") == 3 && slices.Contains(ports, strings.TrimSuffix(in.Port, "/1"))) {
			continue
		}

		out.ConnectionName = pointer.To(conn.Name)
		out.Connection = pointer.To(conn.Spec)
	}

	swName := strings.SplitN(in.Port, "/", 2)[0]

	skip := false
	sw := &wiringapi.Switch{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: swName, Namespace: kmetav1.NamespaceDefault}, sw); err != nil {
		if kapierrors.IsNotFound(err) {
			skip = true
			slog.Warn("Switch object not found", "name", swName)
		} else {
			return nil, errors.Wrapf(err, "failed to get Switch %s", swName)
		}
	}

	agent := &agentapi.Agent{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: swName, Namespace: kmetav1.NamespaceDefault}, agent); err != nil {
		if kapierrors.IsNotFound(err) {
			skip = true
			slog.Warn("Agent object not found", "name", swName)
		} else {
			return nil, errors.Wrapf(err, "failed to get Agent %s", swName)
		}
	}

	if skip {
		slog.Warn("Skipping actual port state", "name", in.Port)

		return out, nil
	}

	portName := strings.SplitN(in.Port, "/", 2)[1]

	if agent.Status.State.Interfaces != nil {
		state, exists := agent.Status.State.Interfaces[portName]
		if exists {
			out.InterfaceState = &state
		}
	}

	if agent.Status.State.Breakouts != nil {
		state, exists := agent.Status.State.Breakouts[portName]
		if exists {
			out.BreakoutState = &state
		}
	}

	if out.Connection != nil && out.ConnectionName != nil {
		conn := out.Connection
		if conn.VPCLoopback != nil { //nolint:gocritic
			var err error
			out.LoopbackWorkarounds, err = loopbackWorkaroundInfo(ctx, kube, agent)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get loopback workaround info")
			}
		} else if conn.Unbundled != nil || conn.Bundled != nil || conn.MCLAG != nil || conn.ESLAG != nil {
			vpcAttaches := &vpcapi.VPCAttachmentList{}
			if err := kube.List(ctx, vpcAttaches, kclient.MatchingLabels{
				wiringapi.LabelConnection: *out.ConnectionName,
			}); err != nil {
				return nil, errors.Wrapf(err, "failed to list VPCAttachments for connection %s", *out.ConnectionName)
			}

			for _, vpcAttach := range vpcAttaches.Items {
				if vpcAttach.Spec.Connection != *out.ConnectionName {
					continue
				}

				vpcName := strings.SplitN(vpcAttach.Spec.Subnet, "/", 2)[0]
				if _, exists := out.AttachedVPCs[vpcName]; !exists {
					vpc := &vpcapi.VPC{}
					if err := kube.Get(ctx, kclient.ObjectKey{Name: vpcName, Namespace: kmetav1.NamespaceDefault}, vpc); err != nil {
						return nil, errors.Wrapf(err, "failed to get VPC %s", vpcName)
					}
					out.AttachedVPCs[vpcName] = &vpc.Spec
				}

				out.VPCAttachments[vpcAttach.Name] = pointer.To(vpcAttach.Spec)
			}
		} else if conn.External != nil {
			extAttaches := &vpcapi.ExternalAttachmentList{}
			if err := kube.List(ctx, extAttaches, kclient.MatchingLabels{
				wiringapi.LabelConnection: *out.ConnectionName,
			}); err != nil {
				return nil, errors.Wrap(err, "cannot list ExternalAttachments")
			}

			for _, extAttach := range extAttaches.Items {
				out.ExternalAttachments[extAttach.Name] = pointer.To(extAttach.Spec)
			}
		}
	}

	return out, nil
}
