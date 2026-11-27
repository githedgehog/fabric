// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	gwintapi "go.githedgehog.com/fabric/api/gwint/v1alpha1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type GatewayIn struct {
	Name         string
	Ports        bool
	Transceivers bool
	Counters     bool
	Lasers       bool
}

type GatewayOut struct {
	Name      string             `json:"name,omitempty"`
	Spec      *gwapi.GatewaySpec `json:"spec,omitempty"`
	State     *AgentState        `json:"state,omitempty"`
	Dataplane string             `json:"dataplane,omitempty"`
	Ports     []*GatewayOutPort  `json:"ports,omitempty"`
}

type GatewayOutPort struct {
	PortName         string                            `json:"portName,omitempty"`
	ConnectionName   string                            `json:"connectionName,omitempty"`
	InterfaceState   *gwintapi.GatewayStateInterface   `json:"interfaceState,omitempty"`
	TransceiverState *gwintapi.GatewayStateTransceiver `json:"transceiverState,omitempty"`
}

func (out *GatewayOut) MarshalText(in GatewayIn, now time.Time) (string, error) {
	str := &strings.Builder{}

	state := ""
	gen := ""
	applied := ""
	heartbeat := ""
	if out.State != nil {
		state = out.State.Summary
		gen = fmt.Sprintf("%d/%d", out.State.LastAppliedGen, out.State.DesiredGen)
		if !out.State.LastAppliedTime.IsZero() {
			applied = HumanizeTime(now, out.State.LastAppliedTime.Time)
		}
		if !out.State.LastHeartbeat.IsZero() {
			heartbeat = HumanizeTime(now, out.State.LastHeartbeat.Time)
		}
	}

	groups := make([]string, 0, len(out.Spec.Groups))
	for _, group := range out.Spec.Groups {
		groups = append(groups, group.Name)
	}

	str.WriteString(RenderTable(
		[]string{"Name", "Driver", "Groups", "Dataplane", "State", "Gen", "Applied", "Heartbeat"},
		[][]string{{out.Name, string(out.Spec.Driver), strings.Join(groups, ", "), out.Dataplane, state, gen, applied, heartbeat}},
	))

	if in.Ports {
		str.WriteString("\nPorts:\n")

		portData := make([][]string, 0, len(out.Ports))
		for _, port := range out.Ports {
			// the kernel name is ignored with the dpdk driver
			device := ""
			if iface, ok := out.Spec.Interfaces[port.PortName]; ok {
				if out.Spec.Driver == gwapi.GatewayDriverDPDK {
					device = iface.PCI
				} else {
					device = cmp.Or(iface.Kernel, port.PortName)
				}
			}

			oper, speed, mac, flaps := "", "", "", ""
			if iface := port.InterfaceState; iface != nil {
				oper = string(iface.OperStatus)
				speed = iface.Speed
				if iface.AutoNegotiate {
					speed += SpeedAutoNeg
				}
				mac = iface.MAC
				if iface.LinkFlapCount != nil {
					flaps = fmt.Sprintf("%d", *iface.LinkFlapCount)
				}
			}

			trans := ""
			if port.TransceiverState != nil {
				trans = port.TransceiverState.Description
			}

			portData = append(portData, []string{port.PortName, device, port.ConnectionName, oper, speed, mac, flaps, trans})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "Device", "Connection", "Oper", "Speed", "MAC", "Flaps", "Transceiver"},
			portData,
		))
	}

	if in.Transceivers {
		str.WriteString("\nTransceivers:\n")

		trData := [][]string{}
		for _, port := range out.Ports {
			tr := port.TransceiverState
			if tr == nil {
				continue
			}

			cmis := ""
			if tr.CMISStatus != "" {
				cmis = tr.CMISStatus
				if tr.CMISRev != "" {
					cmis += " " + tr.CMISRev
					cmis += fmt.Sprintf(" (%d)", tr.CMISApp)
				}
			}

			length := ""
			if tr.CableLength != 0 {
				length = strconv.FormatFloat(tr.CableLength, 'f', -1, 64) + "m"
			}

			trData = append(trData, []string{
				port.PortName,
				tr.Description,
				tr.CableClass,
				strings.TrimSuffix(tr.ConnectorType, "_CONNECTOR"),
				length,
				tr.Vendor,
				tr.VendorPart,
				tr.SerialNumber,
				cmis,
			})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "Description", "Class", "Connector", "Length", "Vendor", "Part", "Serial", "CMIS"},
			trData,
		))
	}

	if in.Lasers {
		str.WriteString("\nLaser Status:\n")

		laserData := [][]string{}
		for _, port := range out.Ports {
			if port.TransceiverState == nil || port.TransceiverState.Channels == nil {
				continue
			}

			line := []string{port.PortName}

			// lanes are keyed by their index, so 10 has to come after 9
			lanes := slices.Collect(maps.Keys(port.TransceiverState.Channels))
			wiringapi.SortPortNames(lanes)

			for _, lane := range lanes {
				chData := port.TransceiverState.Channels[lane]
				in, out := math.Inf(-1), math.Inf(-1)
				if chData.In != nil {
					in = *chData.In
				}
				if chData.Out != nil {
					out = *chData.Out
				}

				line = append(line, fmt.Sprintf("%s: %.2f/%.2f dBm (%.2f mA)", lane, in, out, chData.Bias))
			}

			laserData = append(laserData, line)
		}
		str.WriteString(RenderTable(
			[]string{"Name", "Channels In/Out(Bias)"},
			laserData,
		))
	}

	if in.Counters {
		str.WriteString("\nPort Counters (↓ In ↑ Out):\n")

		countersData := [][]string{}
		for _, port := range out.Ports {
			if port.InterfaceState == nil || port.InterfaceState.Counters == nil {
				continue
			}

			counters := port.InterfaceState.Counters

			lastClear := "-"
			if counters.LastClear != nil && !counters.LastClear.IsZero() {
				lastClear = HumanizeTime(now, counters.LastClear.Time)
			}

			countersData = append(countersData, []string{
				port.PortName,
				port.InterfaceState.Speed,
				fmt.Sprintf("↓ %s", humanize.CommafWithDigits(counters.InBitsPerSecond, 0)),
				fmt.Sprintf("↑ %s", humanize.CommafWithDigits(counters.OutBitsPerSecond, 0)),
				fmt.Sprintf("↓ %s", humanize.CommafWithDigits(counters.InPktsPerSecond, 0)),
				fmt.Sprintf("↑ %s", humanize.CommafWithDigits(counters.OutPktsPerSecond, 0)),
				lastClear,
				fmt.Sprintf("↓ %d ↑ %d ", counters.InErrors, counters.OutErrors),
				fmt.Sprintf("↓ %d ↑ %d", counters.InDiscards, counters.OutDiscards),
			})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "Speed", "Bits/sec In", "Bits/sec Out", "Pkts/sec In", "Pkts/sec Out", "Clear", "Errors", "Discards"},
			countersData,
		))
	}

	str.WriteString("\nUse flags for more details: -p/--ports, -t/--transceivers, -c/--counters, -l/--lasers\n")

	return str.String(), nil
}

var _ Func[GatewayIn, *GatewayOut] = Gateway

func Gateway(ctx context.Context, kube kclient.Reader, in GatewayIn) (*GatewayOut, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("gateway name is required") //nolint:err113
	}

	gw := &gwapi.Gateway{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: in.Name, Namespace: kmetav1.NamespaceDefault}, gw); err != nil {
		return nil, fmt.Errorf("getting gateway %s: %w", in.Name, err)
	}

	out := &GatewayOut{
		Name: in.Name,
		Spec: &gw.Spec,
	}

	ports := map[string]*GatewayOutPort{}
	for ifaceName := range gw.Spec.Interfaces {
		ports[ifaceName] = &GatewayOutPort{PortName: ifaceName}
	}

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

		for _, link := range conn.Spec.Gateway.Links {
			if link.Gateway.DeviceName() != in.Name {
				continue
			}

			ifaceName := link.Gateway.LocalPortName()
			if _, ok := ports[ifaceName]; !ok {
				ports[ifaceName] = &GatewayOutPort{PortName: ifaceName}
			}
			ports[ifaceName].ConnectionName = conn.Name
		}
	}

	ag := &gwintapi.GatewayAgent{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: in.Name, Namespace: kmetav1.NamespaceDefault}, ag); err != nil {
		if !kapierrors.IsNotFound(err) {
			return nil, fmt.Errorf("getting gateway agent %s: %w", in.Name, err)
		}

		slog.Warn("GatewayAgent object not found, skipping actual state", "name", in.Name)
	} else {
		out.Dataplane = ag.Status.State.Dataplane.Version
		out.State = &AgentState{
			Summary:         "Pending",
			DesiredGen:      ag.Generation,
			LastHeartbeat:   ag.Status.LastHeartbeat,
			LastAppliedTime: ag.Status.LastAppliedTime,
			LastAppliedGen:  ag.Status.LastAppliedGen,
		}
		if ag.Status.LastAppliedGen == ag.Generation {
			out.State.Summary = "Ready"
		}

		for ifaceName, state := range ag.Status.State.Interfaces {
			if _, ok := ports[ifaceName]; !ok {
				ports[ifaceName] = &GatewayOutPort{PortName: ifaceName}
			}
			ports[ifaceName].InterfaceState = &state
		}

		for ifaceName, state := range ag.Status.State.Transceivers {
			if _, ok := ports[ifaceName]; !ok {
				ports[ifaceName] = &GatewayOutPort{PortName: ifaceName}
			}
			ports[ifaceName].TransceiverState = &state
		}
	}

	portNames := slices.Collect(maps.Keys(ports))
	wiringapi.SortPortNames(portNames)
	for _, portName := range portNames {
		out.Ports = append(out.Ports, ports[portName])
	}

	return out, nil
}
