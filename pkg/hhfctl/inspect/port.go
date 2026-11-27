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
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	gwintapi "go.githedgehog.com/fabric/api/gwint/v1alpha1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
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
	TransceiverState    *agentapi.SwitchStateTransceiver          `json:"transceiverState,omitempty"`
	VPCAttachments      map[string]*vpcapi.VPCAttachmentSpec      `json:"vpcAttachments,omitempty"`
	AttachedVPCs        map[string]*vpcapi.VPCSpec                `json:"attachedVPCs,omitempty"`
	ExternalAttachments map[string]*vpcapi.ExternalAttachmentSpec `json:"externalAttachments,omitempty"` // if External conn

	// if a gateway port
	GatewayInterfaceState   *gwintapi.GatewayStateInterface   `json:"gatewayInterfaceState,omitempty"`
	GatewayTransceiverState *gwintapi.GatewayStateTransceiver `json:"gatewayTransceiverState,omitempty"`
}

func (out *PortOut) MarshalText(_ PortIn, now time.Time) (string, error) {
	str := strings.Builder{}

	if out.ConnectionName != nil && out.Connection != nil {
		str.WriteString(fmt.Sprintf("Used in Connection %s:\n", *out.ConnectionName))

		data, err := kyaml.Marshal(out.Connection)
		if err != nil {
			return "", errors.Wrap(err, "failed to marshal Connection")
		}
		str.Write(data)
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

		str.WriteString("\n")

		if len(counters.Queues) > 0 {
			str.WriteString("Port Queue Counters:\n")

			sortedQueueNames := lo.Keys(counters.Queues)
			slices.SortFunc(sortedQueueNames, func(a, b string) int {
				if strings.HasPrefix(a, "UC") && strings.HasPrefix(b, "MC") {
					return -1 // UC queues come before MC queues
				}

				return strings.Compare(a, b)
			})

			queueData := [][]string{}
			for _, queueName := range sortedQueueNames {
				queue := counters.Queues[queueName]
				queueData = append(queueData, []string{
					queueName,
					humanize.CommafWithDigits(float64(queue.TransmitPkts), 0),
					humanize.CommafWithDigits(float64(queue.TransmitBits), 0),
					humanize.CommafWithDigits(float64(queue.TransmitPktsPerSecond), 0),
					humanize.CommafWithDigits(float64(queue.TransmitBitsPerSecond), 0),
					humanize.CommafWithDigits(float64(queue.DroppedPkts), 0),
					humanize.CommafWithDigits(float64(queue.DroppedBits), 0),
				})
			}
			str.WriteString(RenderTable(
				[]string{"Q", "Pkts", "Bits", "Pkts/sec", "Bits/sec", "Drop pkts", "Drop bits"},
				queueData,
			))

			str.WriteString("\nWRED ECN Counters:\n")

			queueData = [][]string{}
			for _, queueName := range sortedQueueNames {
				queue := counters.Queues[queueName]
				queueData = append(queueData, []string{
					queueName,
					humanize.CommafWithDigits(float64(queue.WREDDroppedPkts), 0),
					humanize.CommafWithDigits(float64(queue.ECNMarkedPkts), 0),
					humanize.CommafWithDigits(float64(queue.ECNMarkedBits), 0),
				})
			}
			str.WriteString(RenderTable(
				[]string{"Q", "WRED Dropped pkts", "ECN Marked pkts", "ECN Marked bits"},
				queueData,
			))

			str.WriteString("\n")
		}
	}

	if iface := out.GatewayInterfaceState; iface != nil {
		speed := iface.Speed
		if iface.AutoNegotiate {
			speed += SpeedAutoNeg
		}

		lastChange := "-"
		if iface.LastChange != nil && !iface.LastChange.IsZero() {
			lastChange = HumanizeTime(now, iface.LastChange.Time)
		}

		flaps := "-"
		if iface.LinkFlapCount != nil {
			flaps = fmt.Sprintf("%d", *iface.LinkFlapCount)
		}

		str.WriteString("\nInterface:\n")
		str.WriteString(RenderTable(
			[]string{"Oper", "Speed", "FEC", "MAC", "Changed", "Flaps"},
			[][]string{{string(iface.OperStatus), speed, iface.FEC, iface.MAC, lastChange, flaps}},
		))

		if counters := iface.Counters; counters != nil {
			lastClear := "-"
			if counters.LastClear != nil && !counters.LastClear.IsZero() {
				lastClear = HumanizeTime(now, counters.LastClear.Time)
			}

			str.WriteString("\nPort Counters (↓ In ↑ Out):\n")
			str.WriteString(RenderTable(
				[]string{"Bits/sec In", "Bits/sec Out", "Pkts/sec In", "Pkts/sec Out", "Clear", "Errors", "Discards"},
				[][]string{
					{
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

		if len(iface.LLDPNeighbors) > 0 {
			neighborData := [][]string{}
			for _, neighbor := range iface.LLDPNeighbors {
				updated := "-"
				if neighbor.LastUpdate != nil && !neighbor.LastUpdate.IsZero() {
					updated = HumanizeTime(now, neighbor.LastUpdate.Time)
				}

				neighborData = append(neighborData, []string{
					neighbor.SystemName,
					cmp.Or(neighbor.Port, neighbor.PortID),
					neighbor.PortDescription,
					neighbor.MAC,
					neighbor.Model,
					updated,
				})
			}

			str.WriteString("\nLLDP Neighbors:\n")
			str.WriteString(RenderTable(
				[]string{"Name", "Port", "Port Description", "MAC", "Model", "Updated"},
				neighborData,
			))
		}

		str.WriteString("\n")
	}

	if tr := out.GatewayTransceiverState; tr != nil {
		str.WriteString("Transceiver:\n")
		fmt.Fprintf(&str, "  Description: %s\n", tr.Description)
		fmt.Fprintf(&str, "  FormFactor: %s / %s\n", tr.FormFactor, tr.ConnectorType)
		fmt.Fprintf(&str, "  CableClass: %s\n", tr.CableClass)
		if tr.CableLength != 0 {
			fmt.Fprintf(&str, "  CableLength: %sm\n", strconv.FormatFloat(tr.CableLength, 'f', -1, 64))
		}
		fmt.Fprintf(&str, "  SerialNumber: %s\n", tr.SerialNumber)
		fmt.Fprintf(&str, "  Vendor: %q part %q rev %q\n", tr.Vendor, tr.VendorPart, tr.VendorRev)
		if tr.Voltage != 0 {
			fmt.Fprintf(&str, "  Voltage: %s\n", humanize.CommafWithDigits(tr.Voltage, 2))
		}
		if tr.Temperature != 0 {
			fmt.Fprintf(&str, "  Temperature: %s\n", humanize.CommafWithDigits(tr.Temperature, 2))
		}

		str.WriteString("\n")
	}

	if out.BreakoutState != nil {
		str.WriteString("Breakout State:\n")
		str.WriteString(fmt.Sprintf("  Mode: %s\n", out.BreakoutState.Mode))
		str.WriteString(fmt.Sprintf("  Status: %s\n", out.BreakoutState.Status))

		str.WriteString("\n")
	}

	if out.TransceiverState != nil {
		str.WriteString(fmt.Sprintf("Transceiver: %s\n", out.TransceiverState.OperStatus))
		str.WriteString(fmt.Sprintf("  Description: %s\n", out.TransceiverState.Description))
		str.WriteString(fmt.Sprintf("  FormFactor: %s / %s\n", out.TransceiverState.FormFactor, out.TransceiverState.ConnectorType))
		str.WriteString(fmt.Sprintf("  CableClass: %s\n", out.TransceiverState.CableClass))
		if out.TransceiverState.CableLength != 0 {
			str.WriteString(fmt.Sprintf("  CableLength: %sm\n", humanize.CommafWithDigits(out.TransceiverState.CableLength, 0)))
		}
		str.WriteString(fmt.Sprintf("  SerialNumber: %s\n", out.TransceiverState.SerialNumber))
		str.WriteString(fmt.Sprintf("  Vendor: %q part %q rev %q\n", out.TransceiverState.Vendor, out.TransceiverState.VendorPart, out.TransceiverState.VendorRev))
		if out.TransceiverState.Voltage != 0 {
			str.WriteString(fmt.Sprintf("  Voltage: %s\n", humanize.CommafWithDigits(out.TransceiverState.Voltage, 2)))
		}
		if out.TransceiverState.Temperature != 0 {
			str.WriteString(fmt.Sprintf("  Temperature: %s\n", humanize.CommafWithDigits(out.TransceiverState.Temperature, 2)))
		}

		str.WriteString("\n")
	}

	if len(out.VPCAttachments) > 0 {
		str.WriteString("VPC Attachments:\n")

		attachData := [][]string{}
		attachNames := lo.Keys(out.VPCAttachments)
		for _, attachName := range attachNames {
			attach := out.VPCAttachments[attachName]

			subnet := ""
			vlan := ""
			vpcName, subnetName, _ := strings.Cut(attach.Subnet, "/")
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

		str.WriteString("\n")
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

		str.WriteString("\n")
	}

	return str.String(), nil
}

var _ Func[PortIn, *PortOut] = Port

func Port(ctx context.Context, kube kclient.Reader, in PortIn) (*PortOut, error) {
	if in.Port == "" {
		return nil, errors.New("port is required")
	}
	parts := strings.SplitN(in.Port, "/", 2)
	if len(parts) != 2 {
		return nil, errors.New("port must be in format <switch-name>/<port-name>")
	}
	swName := parts[0]
	portName := parts[1]
	if swName == "" || portName == "" {
		return nil, errors.New("port must be in format <switch-name>/<port-name>")
	}

	gw := &gwapi.Gateway{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: swName, Namespace: kmetav1.NamespaceDefault}, gw); err == nil {
		return gatewayPort(ctx, kube, in.Port, swName, portName)
	} else if !kapierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting gateway %s: %w", swName, err)
	}

	out := &PortOut{
		VPCAttachments:      map[string]*vpcapi.VPCAttachmentSpec{},
		AttachedVPCs:        map[string]*vpcapi.VPCSpec{},
		ExternalAttachments: map[string]*vpcapi.ExternalAttachmentSpec{},
	}

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns, wiringapi.MatchingLabelsForListLabelSwitch(swName)); err != nil {
		return nil, fmt.Errorf("cannot list Connections of switch %s: %w", swName, err)
	}

	for _, conn := range conns.Items {
		_, _, ports, _, err := conn.Spec.Endpoints()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get connection %s endpoints", conn.Name)
		}

		if !(slices.Contains(ports, in.Port) || strings.Count(in.Port, "/") == 3 && slices.Contains(ports, strings.TrimSuffix(in.Port, "/1"))) {
			continue
		}

		out.ConnectionName = new(conn.Name)
		out.Connection = new(conn.Spec)
	}

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

	if agent.Status.State.Breakouts != nil {
		state, exists := agent.Status.State.Breakouts[portName]
		if exists {
			out.BreakoutState = &state
		}
	}

	if agent.Status.State.Transceivers != nil {
		if state, exists := agent.Status.State.Transceivers[portName]; exists {
			out.TransceiverState = &state
		}
	}

	if agent.Status.State.Interfaces != nil {
		state, exists := agent.Status.State.Interfaces[portName]
		if exists {
			out.InterfaceState = &state
		}
	}

	if out.Connection != nil && out.ConnectionName != nil {
		conn := out.Connection
		if conn.Unbundled != nil || conn.Bundled != nil || conn.ESLAG != nil {
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

				vpcName, _, _ := strings.Cut(vpcAttach.Spec.Subnet, "/")
				if _, exists := out.AttachedVPCs[vpcName]; !exists {
					vpc := &vpcapi.VPC{}
					if err := kube.Get(ctx, kclient.ObjectKey{Name: vpcName, Namespace: kmetav1.NamespaceDefault}, vpc); err != nil {
						return nil, errors.Wrapf(err, "failed to get VPC %s", vpcName)
					}
					out.AttachedVPCs[vpcName] = &vpc.Spec
				}

				out.VPCAttachments[vpcAttach.Name] = new(vpcAttach.Spec)
			}
		} else if conn.External != nil {
			extAttaches := &vpcapi.ExternalAttachmentList{}
			if err := kube.List(ctx, extAttaches, kclient.MatchingLabels{
				wiringapi.LabelConnection: *out.ConnectionName,
			}); err != nil {
				return nil, errors.Wrap(err, "cannot list ExternalAttachments")
			}

			for _, extAttach := range extAttaches.Items {
				out.ExternalAttachments[extAttach.Name] = new(extAttach.Spec)
			}
		}
	}

	return out, nil
}

func gatewayPort(ctx context.Context, kube kclient.Reader, port, gwName, ifaceName string) (*PortOut, error) {
	out := &PortOut{}

	// gateway connections have no list label for the gateway
	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns, kclient.InNamespace(kmetav1.NamespaceDefault), kclient.MatchingLabels{
		wiringapi.LabelConnectionType: wiringapi.ConnectionTypeGateway,
	}); err != nil {
		return nil, fmt.Errorf("listing gateway connections: %w", err)
	}

	for _, conn := range conns.Items {
		if conn.Spec.Gateway == nil {
			continue
		}

		if slices.ContainsFunc(conn.Spec.Gateway.Links, func(link wiringapi.GatewayLink) bool { return link.Gateway.PortName() == port }) {
			out.ConnectionName = new(conn.Name)
			out.Connection = new(conn.Spec)

			break
		}
	}

	ag := &gwintapi.GatewayAgent{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: gwName, Namespace: kmetav1.NamespaceDefault}, ag); err != nil {
		if kapierrors.IsNotFound(err) {
			slog.Warn("GatewayAgent object not found, skipping actual port state", "name", gwName)

			return out, nil
		}

		return nil, fmt.Errorf("getting gateway agent %s: %w", gwName, err)
	}

	if state, ok := ag.Status.State.Interfaces[ifaceName]; ok {
		out.GatewayInterfaceState = &state
	}
	if state, ok := ag.Status.State.Transceivers[ifaceName]; ok {
		out.GatewayTransceiverState = &state
	}

	return out, nil
}
