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
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type SwitchIn struct {
	Name         string
	Details      bool
	Ports        bool
	Transceivers bool
	Counters     bool
	Lasers       bool
}

type SwitchOut struct {
	Name     string                       `json:"name,omitempty"`
	State    *AgentState                  `json:"state,omitempty"`
	Spec     *wiringapi.SwitchSpec        `json:"spec,omitempty"`
	Profile  *wiringapi.SwitchProfileSpec `json:"profile,omitempty"`
	Ports    []*SwitchOutPort             `json:"ports,omitempty"`
	Serial   string                       `json:"serial,omitempty"`
	Software string                       `json:"software,omitempty"`
	Firmware map[string]string            `json:"firmware,omitempty"`
}

type AgentState struct {
	Summary         string       `json:"summary,omitempty"`
	DesiredGen      int64        `json:"desiredGen,omitempty"`
	LastHeartbeat   kmetav1.Time `json:"lastHeartbeat,omitempty"`
	LastAttemptTime kmetav1.Time `json:"lastAttemptTime,omitempty"`
	LastAttemptGen  int64        `json:"lastAttemptGen,omitempty"`
	LastAppliedTime kmetav1.Time `json:"lastAppliedTime,omitempty"`
	LastAppliedGen  int64        `json:"lastAppliedGen,omitempty"`
}

type SwitchOutPort struct {
	PortName         string                           `json:"portName,omitempty"`
	ConnectionName   string                           `json:"connectionName,omitempty"`
	ConnectionType   string                           `json:"connectionType,omitempty"`
	InterfaceState   *agentapi.SwitchStateInterface   `json:"interfaceState,omitempty"`
	BreakoutState    *agentapi.SwitchStateBreakout    `json:"breakoutState,omitempty"`
	TransceiverState *agentapi.SwitchStateTransceiver `json:"transceiverState,omitempty"`
}

func (out *SwitchOut) MarshalText(in SwitchIn, now time.Time) (string, error) {
	str := &strings.Builder{}

	applied := ""
	if !out.State.LastAppliedTime.IsZero() {
		applied = HumanizeTime(now, out.State.LastAppliedTime.Time)
	}

	heartbeat := ""
	if !out.State.LastHeartbeat.IsZero() {
		heartbeat = HumanizeTime(now, out.State.LastHeartbeat.Time)
	}

	str.WriteString(RenderTable(
		[]string{"Name", "Profile", "Role", "Groups", "Serial", "State", "Gen", "Applied", "Heartbeat"},
		[][]string{
			{
				out.Name,
				out.Profile.DisplayName,
				string(out.Spec.Role),
				strings.Join(out.Spec.Groups, ", "),
				out.Serial,
				out.State.Summary,
				fmt.Sprintf("%d/%d", out.State.LastAppliedGen, out.State.DesiredGen),
				applied,
				heartbeat,
			},
		},
	))

	if in.Details {
		str.WriteString("\nFirmware:\n")

		fwData := [][]string{}
		for _, fwName := range slices.Sorted(maps.Keys(out.Firmware)) {
			fwVersion := out.Firmware[fwName]
			fwData = append(fwData, []string{fwName, fwVersion})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "Version"},
			fwData,
		))
	}

	portMap, err := out.Profile.GetAPI2NOSPortsFor(out.Spec)
	if err != nil {
		return "", errors.Wrap(err, "cannot get API to NOS port mapping")
	}

	if in.Ports {
		str.WriteString("\nPorts:\n")

		portData := [][]string{}
		for _, port := range out.Ports {
			portType := port.ConnectionType
			state := ""
			speed := ""
			transName := ""
			trans := ""
			nos := portMap[port.PortName]
			conn := port.ConnectionName

			if port.BreakoutState != nil {
				portType = "breakout"
				speed = port.BreakoutState.Mode
				state = strings.ToLower(port.BreakoutState.Status)

				profile, exists := out.Profile.Ports[port.PortName]
				if exists {
					if profile.NOSName != "" {
						nos = profile.NOSName
					}
				}
			}

			if port.InterfaceState != nil {
				state = fmt.Sprintf("%s/%s", port.InterfaceState.AdminStatus, port.InterfaceState.OperStatus)
				speed = port.InterfaceState.Speed
				if port.InterfaceState.AutoNegotiate {
					speed += "/auto"
				}
			}

			if port.TransceiverState != nil {
				transName = port.TransceiverState.Description
				if port.TransceiverState.OperStatus != "" {
					trans = port.TransceiverState.OperStatus
				}
				trans += "/"
				if port.TransceiverState.CMISStatus != "" {
					trans += strings.ToLower(port.TransceiverState.CMISStatus)
				}
				trans = strings.TrimSuffix(trans, "/")
			}

			portData = append(portData, []string{
				port.PortName,
				nos,
				portType,
				conn,
				state,
				speed,
				transName,
				trans,
			})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "NOS", "Type", "Connection/Mode", "Adm/Op", "Speed", "Transceiver", "Transc/CMIS"},
			portData,
		))
	}

	if in.Transceivers {
		str.WriteString("\nTransceivers:\n")

		trData := [][]string{}
		for _, port := range out.Ports {
			if port.TransceiverState == nil {
				continue
			}

			cmis := ""
			if port.TransceiverState.CMISStatus != "" {
				cmis = port.TransceiverState.CMISStatus
				if port.TransceiverState.CMISRev != "" {
					cmis += " " + port.TransceiverState.CMISRev
					cmis += fmt.Sprintf(" (%d)", port.TransceiverState.CMISApp)
				}
			}

			trData = append(trData, []string{
				port.PortName,
				port.TransceiverState.OperStatus,
				port.TransceiverState.Description,
				port.TransceiverState.CableClass,
				strings.TrimSuffix(port.TransceiverState.ConnectorType, "_CONNECTOR"),
				port.TransceiverState.Vendor,
				port.TransceiverState.VendorPart,
				port.TransceiverState.SerialNumber,
				cmis,
			})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "Oper", "Description", "Class", "Connector", "Vendor", "Part", "Serial", "CMIS"},
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

			line := []string{
				port.PortName,
			}

			for _, chName := range slices.Sorted(maps.Keys(port.TransceiverState.Channels)) {
				chData := port.TransceiverState.Channels[chName]
				var in, out float64
				if chData.In == nil {
					in = math.Inf(-1)
				} else {
					in = *chData.In
				}
				if chData.Out == nil {
					out = math.Inf(-1)
				} else {
					out = *chData.Out
				}

				line = append(line, fmt.Sprintf("%s: %.2f/%.2f dBm (%.2f mA)", chName, in, out, chData.Bias))
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
			if !counters.LastClear.IsZero() {
				lastClear = HumanizeTime(now, counters.LastClear.Time)
			}

			countersData = append(countersData, []string{
				port.PortName,
				port.InterfaceState.Speed,
				fmt.Sprintf("↓ %3d ↑ %3d ", counters.InUtilization, counters.OutUtilization),
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
			[]string{"Name", "Speed", "Util %", "Bits/sec In", "Bits/sec Out", "Pkts/sec In", "Pkts/sec Out", "Clear", "Errors", "Discards"},
			countersData,
		))
	}

	str.WriteString("\nUse flags for more details: -d/--details (e.g. firmware), -p/--ports, -t/--transceivers, -c/--counters, -l/--lasers\n")

	return str.String(), nil
}

var _ Func[SwitchIn, *SwitchOut] = Switch

func Switch(ctx context.Context, kube kclient.Reader, in SwitchIn) (*SwitchOut, error) {
	swName := in.Name
	if swName == "" {
		return nil, errors.Errorf("switch name is required")
	}

	out := &SwitchOut{
		Name: swName,
	}

	sw := &wiringapi.Switch{}
	if err := kube.Get(ctx, ktypes.NamespacedName{Name: swName, Namespace: kmetav1.NamespaceDefault}, sw); err != nil {
		return nil, errors.Wrapf(err, "cannot get switch %s", swName)
	}

	out.Spec = &sw.Spec

	sp := &wiringapi.SwitchProfile{}
	if err := kube.Get(ctx, ktypes.NamespacedName{Name: sw.Spec.Profile, Namespace: kmetav1.NamespaceDefault}, sp); err != nil {
		return nil, errors.Wrapf(err, "cannot get switch profile %s", sw.Spec.Profile)
	}

	out.Profile = &sp.Spec

	skipActual := false
	agent := &agentapi.Agent{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: swName, Namespace: kmetav1.NamespaceDefault}, agent); err != nil {
		if kapierrors.IsNotFound(err) {
			skipActual = true
			slog.Warn("Agent object not found", "name", swName)
		} else {
			return nil, errors.Wrapf(err, "failed to get Agent %s", swName)
		}
	}

	if skipActual {
		slog.Warn("Skipping actual state")

		return out, nil
	}

	out.Serial = agent.Status.State.NOS.SerialNumber
	out.Software = agent.Status.State.NOS.SoftwareVersion
	out.Firmware = agent.Status.State.Firmware

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns, kclient.MatchingLabels{
		wiringapi.ListLabelSwitch(swName): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrap(err, "cannot list Connections")
	}

	ports := map[string]*SwitchOutPort{}
	for _, conn := range conns.Items {
		_, _, connPorts, _, err := conn.Spec.Endpoints()
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get endpoints for connection %s", conn.Name)
		}

		for _, connPort := range connPorts {
			if !strings.HasPrefix(connPort, swName+"/") {
				continue
			}

			portName := strings.SplitN(connPort, "/", 2)[1]
			port := &SwitchOutPort{
				PortName:       portName,
				ConnectionName: conn.Name,
				ConnectionType: conn.Spec.Type(),
			}

			if agent.Status.State.Interfaces != nil {
				state, exists := agent.Status.State.Interfaces[portName]
				if !exists {
					state, exists = agent.Status.State.Interfaces[portName+"/1"]
					if exists {
						port.PortName += "/1"
					}
				}

				if exists {
					port.InterfaceState = &state
				}
			}

			ports[portName] = port
		}
	}

	for ifaceName, ifaceState := range agent.Status.State.Interfaces {
		if !strings.HasPrefix(ifaceName, "E1") {
			continue
		}
		if _, ok := ports[ifaceName]; !ok {
			ports[ifaceName] = &SwitchOutPort{
				ConnectionType: "-",
				PortName:       ifaceName,
				InterfaceState: &ifaceState,
			}
		}
	}

	for transceiverName, transceiver := range agent.Status.State.Transceivers {
		if transceiver.OperStatus == "" {
			continue
		}

		port, ok := ports[transceiverName]
		if !ok {
			port = &SwitchOutPort{}
			ports[transceiverName] = port
		}

		port.PortName = transceiverName
		port.TransceiverState = &transceiver
	}

	for breakoutName, breakout := range agent.Status.State.Breakouts {
		port, ok := ports[breakoutName]
		if !ok {
			port = &SwitchOutPort{}
			ports[breakoutName] = port
		}

		port.PortName = breakoutName
		port.BreakoutState = &breakout
	}

	portNames := lo.Keys(ports)
	wiringapi.SortPortNames(portNames)

	for _, portName := range portNames {
		out.Ports = append(out.Ports, ports[portName])
	}

	out.State = switchStateSummary(agent)

	return out, nil
}

func switchStateSummary(agent *agentapi.Agent) *AgentState {
	res := &AgentState{
		Summary: "Unknown",
	}

	if agent == nil {
		return res
	}

	if agent.Status.LastAppliedGen == agent.Generation {
		res.Summary = "Ready"
	} else {
		res.Summary = "Pending"
	}

	res.DesiredGen = agent.Generation

	res.LastHeartbeat = agent.Status.LastHeartbeat
	res.LastAttemptTime = agent.Status.LastAttemptTime
	res.LastAttemptGen = agent.Status.LastAttemptGen
	res.LastAppliedTime = agent.Status.LastAppliedTime
	res.LastAppliedGen = agent.Status.LastAppliedGen

	return res
}
