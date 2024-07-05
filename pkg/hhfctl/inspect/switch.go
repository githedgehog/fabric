package inspect

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/pointer"
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
	StateSummary string                       `json:"stateSummary,omitempty"`
	Spec         *wiringapi.SwitchSpec        `json:"spec,omitempty"`
	Profile      *wiringapi.SwitchProfileSpec `json:"profile,omitempty"`
	Ports        []*SwitchOutPort             `json:"ports,omitempty"`
}

type SwitchOutPort struct {
	PortName       string                         `json:"portName,omitempty"`
	ConnectionName *string                        `json:"connectionName,omitempty"`
	ConnectionType *string                        `json:"connectionType,omitempty"`
	InterfaceState *agentapi.SwitchStateInterface `json:"interfaceState,omitempty"`
	BreakoutState  *agentapi.SwitchStateBreakout  `json:"breakoutState,omitempty"`
}

func (out *SwitchOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO implement marshal
}

var _ Func[SwitchIn, *SwitchOut] = Switch

func Switch(ctx context.Context, kube client.Reader, in SwitchIn) (*SwitchOut, error) {
	swName := in.Name
	if swName == "" {
		return nil, errors.Errorf("switch name is required")
	}

	out := &SwitchOut{}

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
		slog.Warn("Skipping actual ports state")

		return out, nil
	}

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
				ConnectionName: &conn.Name,
				ConnectionType: pointer.To(conn.Spec.Type()),
			}

			if !skipActual && agent.Status.State.Interfaces != nil {
				state, exists := agent.Status.State.Interfaces[portName]
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

	out.StateSummary = switchStateSummary(sw, agent)

	return out, nil
}

func switchStateSummary(sw *wiringapi.Switch, agent *agentapi.Agent) string {
	if sw == nil || agent == nil {
		return "Unknown"
	}

	out := []string{}

	if agent.Status.LastAppliedGen == agent.Generation {
		out = append(out, "Ready")

		if agent.Status.LastAppliedGen > 0 && !agent.Status.LastAppliedTime.IsZero() {
			out = append(out, fmt.Sprintf("applied gen %d (%s)", agent.Status.LastAppliedGen, humanize.Time(agent.Status.LastAppliedTime.Time)))
		}
	} else {
		out = append(out, "Pending")

		out = append(out, fmt.Sprintf("desired gen %d", agent.Generation))
		out = append(out, fmt.Sprintf("applied gen %d (%s)", agent.Status.LastAppliedGen, humanize.Time(agent.Status.LastAppliedTime.Time)))
	}

	if !agent.Status.LastHeartbeat.IsZero() {
		out = append(out, fmt.Sprintf("last heartbeat %s", humanize.Time(agent.Status.LastHeartbeat.Time)))
	}

	return strings.Join(out, ", ")
}
