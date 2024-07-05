package inspect

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"golang.org/x/exp/maps"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SwitchIn struct {
	Name string
}

type SwitchOut struct {
	Name     string                       `json:"name,omitempty"`
	State    *AgentState                  `json:"state,omitempty"`
	Spec     *wiringapi.SwitchSpec        `json:"spec,omitempty"`
	Profile  *wiringapi.SwitchProfileSpec `json:"profile,omitempty"`
	Ports    []*SwitchOutPort             `json:"ports,omitempty"`
	Serial   string                       `json:"serial,omitempty"`
	Software string                       `json:"software,omitempty"`
}

type AgentState struct {
	Summary         string      `json:"summary,omitempty"`
	DesiredGen      int64       `json:"desiredGen,omitempty"`
	LastHeartbeat   metav1.Time `json:"lastHeartbeat,omitempty"`
	LastAttemptTime metav1.Time `json:"lastAttemptTime,omitempty"`
	LastAttemptGen  int64       `json:"lastAttemptGen,omitempty"`
	LastAppliedTime metav1.Time `json:"lastAppliedTime,omitempty"`
	LastAppliedGen  int64       `json:"lastAppliedGen,omitempty"`
}

type SwitchOutPort struct {
	PortName       string                         `json:"portName,omitempty"`
	ConnectionName string                         `json:"connectionName,omitempty"`
	ConnectionType string                         `json:"connectionType,omitempty"`
	InterfaceState *agentapi.SwitchStateInterface `json:"interfaceState,omitempty"`
	BreakoutState  *agentapi.SwitchStateBreakout  `json:"breakoutState,omitempty"`
}

func (out *SwitchOut) MarshalText() (string, error) {
	str := &strings.Builder{}

	str.WriteString(RenderTable(
		[]string{"Name", "Profile", "Role", "Groups", "Serial", "State", "Gen", "Heartbeat"},
		[][]string{
			{
				out.Name,
				out.Profile.DisplayName,
				string(out.Spec.Role),
				strings.Join(out.Spec.Groups, ", "),
				out.Serial,
				out.State.Summary,
				fmt.Sprintf("%d/%d", out.State.LastAppliedGen, out.State.DesiredGen),
				humanize.Time(out.State.LastHeartbeat.Time),
			},
		},
	))

	str.WriteString("Ports in use:\n")

	portData := [][]string{}

	for _, port := range out.Ports {
		portType := port.ConnectionType
		state := ""
		speed := ""
		trans := ""

		if port.BreakoutState != nil {
			portType = "breakout"
			speed = port.BreakoutState.Mode
			state = strings.ToLower(port.BreakoutState.Status)
		}

		if port.InterfaceState != nil {
			state = fmt.Sprintf("%s/%s", port.InterfaceState.AdminStatus, port.InterfaceState.OperStatus)
			speed = port.InterfaceState.Speed
			trans = port.InterfaceState.Transceiver.Description

			if port.InterfaceState.Transceiver.OperStatus != "" {
				state += fmt.Sprintf(" (%s)", port.InterfaceState.Transceiver.OperStatus)
			}
		}

		portData = append(portData, []string{
			port.PortName,
			portType,
			port.ConnectionName,
			state,
			speed,
			trans,
		})
	}

	str.WriteString(RenderTable(
		[]string{"Name", "Type", "Connection", "Adm/Op (Transc)", "Speed", "Transceiver"},
		portData,
	))

	return str.String(), nil
}

var _ Func[SwitchIn, *SwitchOut] = Switch

func Switch(ctx context.Context, kube client.Reader, in SwitchIn) (*SwitchOut, error) {
	swName := in.Name
	if swName == "" {
		return nil, errors.Errorf("switch name is required")
	}

	out := &SwitchOut{
		Name: swName,
	}

	sw := &wiringapi.Switch{}
	if err := kube.Get(ctx, types.NamespacedName{Name: swName, Namespace: metav1.NamespaceDefault}, sw); err != nil {
		return nil, errors.Wrapf(err, "cannot get switch %s", swName)
	}

	out.Spec = &sw.Spec

	sp := &wiringapi.SwitchProfile{}
	if err := kube.Get(ctx, types.NamespacedName{Name: sw.Spec.Profile, Namespace: metav1.NamespaceDefault}, sp); err != nil {
		return nil, errors.Wrapf(err, "cannot get switch profile %s", sw.Spec.Profile)
	}

	out.Profile = &sp.Spec

	skipActual := false
	agent := &agentapi.Agent{}
	if err := kube.Get(ctx, client.ObjectKey{Name: swName, Namespace: metav1.NamespaceDefault}, agent); err != nil {
		if apierrors.IsNotFound(err) {
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

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns, client.MatchingLabels{
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

			if !skipActual && agent.Status.State.Interfaces != nil {
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

			if !skipActual && strings.Count(portName, "/") == 2 {
				breakoutName := portName[:strings.LastIndex(portName, "/")]

				if agent.Status.State.Breakouts != nil {
					state, exists := agent.Status.State.Breakouts[breakoutName]
					if exists {
						ports[breakoutName] = &SwitchOutPort{
							PortName:      breakoutName,
							BreakoutState: &state,
						}
					}
				}
			}
		}
	}

	portNames := maps.Keys(ports)
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
