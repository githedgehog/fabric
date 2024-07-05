package inspect

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FabricIn struct {
	PortMapping bool
}

type FabricOut struct {
	Summary      string              `json:"summary,omitempty"`
	ControlNodes []*FabricOutControl `json:"controlNodes,omitempty"`
	Switches     []*FabricOutSwitch  `json:"switches,omitempty"`
}

type FabricOutControl struct {
	Name         string `json:"name,omitempty"`
	StateSummary string `json:"stateSummary,omitempty"`
}

type FabricOutSwitch struct {
	Name               string `json:"name,omitempty"`
	Serial             string `json:"serial,omitempty"`
	Software           string `json:"software,omitempty"`
	ProfileDisplayName string `json:"profileDisplayName,omitempty"`
	StateSummary       string `json:"stateSummary,omitempty"`
}

func (out *FabricOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO implement marshal
}

var _ Func[FabricIn, *FabricOut] = Fabric

func Fabric(ctx context.Context, kube client.Reader, _ FabricIn) (*FabricOut, error) {
	out := &FabricOut{}

	totalControls := 0
	totalSwitches := 0
	readyControls := 0
	readySwitches := 0

	swList := &wiringapi.SwitchList{}
	if err := kube.List(ctx, swList); err != nil {
		return nil, errors.Wrap(err, "cannot list switches")
	}

	for _, sw := range swList.Items {
		swName := sw.Name

		totalSwitches++

		sp := &wiringapi.SwitchProfile{}
		if err := kube.Get(ctx, client.ObjectKey{Name: sw.Spec.Profile, Namespace: metav1.NamespaceDefault}, sp); err != nil {
			return nil, errors.Wrapf(err, "cannot get switch profile %s", sw.Spec.Profile)
		}

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

		swState := &FabricOutSwitch{
			Name:               swName,
			ProfileDisplayName: sp.Spec.DisplayName,
			StateSummary:       switchStateSummary(&sw, agent),
		}

		if !skipActual {
			swState.Serial = agent.Status.State.NOS.SerialNumber
			swState.Software = agent.Status.State.NOS.SoftwareVersion

			if agent.Status.LastAppliedGen == agent.Generation {
				readySwitches++
			}
		}

		out.Switches = append(out.Switches, swState)
	}

	slices.SortFunc(out.Switches, func(a, b *FabricOutSwitch) int {
		return strings.Compare(a.Name, b.Name)
	})

	servers := &wiringapi.ServerList{}
	if err := kube.List(ctx, servers, client.MatchingLabels{
		wiringapi.LabelServerType: string(wiringapi.ServerTypeControl),
	}); err != nil {
		return nil, errors.Wrap(err, "cannot list control nodes")
	}

	for _, srv := range servers.Items {
		srvName := srv.Name

		totalControls++

		skipActual := false
		agent := &agentapi.ControlAgent{}
		if err := kube.Get(ctx, client.ObjectKey{Name: srvName, Namespace: metav1.NamespaceDefault}, agent); err != nil {
			if apierrors.IsNotFound(err) {
				skipActual = true
				slog.Warn("ControlAgent object not found", "name", srvName)
			} else {
				return nil, errors.Wrapf(err, "failed to get ControlAgent %s", srvName)
			}
		}

		out.ControlNodes = append(out.ControlNodes, &FabricOutControl{
			Name:         srvName,
			StateSummary: controlStateSummary(agent),
		})

		if !skipActual && agent.Status.LastAppliedGen == agent.Generation {
			readyControls++
		}
	}

	slices.SortFunc(out.ControlNodes, func(a, b *FabricOutControl) int {
		return strings.Compare(a.Name, b.Name)
	})

	out.Summary = fmt.Sprintf("Ready: %d/%d control nodes, %d/%d switches", readyControls, totalControls, readySwitches, totalSwitches)

	return out, nil
}

// TODO dedup with switch agent state summary
func controlStateSummary(agent *agentapi.ControlAgent) string {
	if agent == nil {
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
