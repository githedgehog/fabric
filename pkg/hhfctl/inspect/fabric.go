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
	"slices"
	"strings"

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
	Name  string      `json:"name,omitempty"`
	State *AgentState `json:"state,omitempty"`
}

type FabricOutSwitch struct {
	Name               string      `json:"name,omitempty"`
	Serial             string      `json:"serial,omitempty"`
	Software           string      `json:"software,omitempty"`
	ProfileDisplayName string      `json:"profileDisplayName,omitempty"`
	State              *AgentState `json:"state,omitempty"`
	Role               string      `json:"role,omitempty"`
	Groups             []string    `json:"groups,omitempty"`
}

func (out *FabricOut) MarshalText() (string, error) {
	str := &strings.Builder{}

	str.WriteString("Control Nodes:\n")

	ctrlData := [][]string{}
	for _, ctrl := range out.ControlNodes {
		applied := ""

		if !ctrl.State.LastAppliedTime.IsZero() {
			applied = humanize.Time(ctrl.State.LastAppliedTime.Time)
		}

		ctrlData = append(ctrlData, []string{
			ctrl.Name,
			ctrl.State.Summary,
			fmt.Sprintf("%d/%d", ctrl.State.LastAppliedGen, ctrl.State.DesiredGen),
			applied,
			humanize.Time(ctrl.State.LastHeartbeat.Time),
		})
	}
	str.WriteString(RenderTable(
		[]string{"Name", "State", "Gen", "Applied", "Heartbeat"},
		ctrlData,
	))

	str.WriteString("Switches:\n")

	swData := [][]string{}
	for _, sw := range out.Switches {
		applied := ""

		if !sw.State.LastAppliedTime.IsZero() {
			applied = humanize.Time(sw.State.LastAppliedTime.Time)
		}

		swData = append(swData, []string{
			sw.Name,
			sw.ProfileDisplayName,
			sw.Role,
			strings.Join(sw.Groups, ", "),
			sw.Serial,
			sw.State.Summary,
			fmt.Sprintf("%d/%d", sw.State.LastAppliedGen, sw.State.DesiredGen),
			applied,
			humanize.Time(sw.State.LastHeartbeat.Time),
		})
	}
	str.WriteString(RenderTable(
		[]string{"Name", "Profile", "Role", "Groups", "Serial", "State", "Gen", "Applied", "Heartbeat"},
		swData,
	))

	return str.String(), nil
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
			State:              switchStateSummary(agent),
			Role:               string(sw.Spec.Role),
			Groups:             sw.Spec.Groups,
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
			Name:  srvName,
			State: controlStateSummary(agent),
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
