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
	"time"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type FabricIn struct {
	PortMapping bool
}

type FabricOut struct {
	Summary  string             `json:"summary,omitempty"`
	Switches []*FabricOutSwitch `json:"switches,omitempty"`
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

func (out *FabricOut) MarshalText(_ FabricIn, now time.Time) (string, error) {
	str := &strings.Builder{}

	str.WriteString("Switches:\n")

	swData := [][]string{}
	for _, sw := range out.Switches {
		applied := ""
		if !sw.State.LastAppliedTime.IsZero() {
			applied = HumanizeTime(now, sw.State.LastAppliedTime.Time)
		}

		heartbeat := ""
		if !sw.State.LastHeartbeat.IsZero() {
			heartbeat = HumanizeTime(now, sw.State.LastHeartbeat.Time)
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
			heartbeat,
		})
	}
	str.WriteString(RenderTable(
		[]string{"Name", "Profile", "Role", "Groups", "Serial", "State", "Gen", "Applied", "Heartbeat"},
		swData,
	))

	return str.String(), nil
}

var _ Func[FabricIn, *FabricOut] = Fabric

func Fabric(ctx context.Context, kube kclient.Reader, _ FabricIn) (*FabricOut, error) {
	out := &FabricOut{}

	totalSwitches := 0
	readySwitches := 0

	swList := &wiringapi.SwitchList{}
	if err := kube.List(ctx, swList); err != nil {
		return nil, errors.Wrap(err, "cannot list switches")
	}

	for _, sw := range swList.Items {
		swName := sw.Name

		totalSwitches++

		sp := &wiringapi.SwitchProfile{}
		if err := kube.Get(ctx, kclient.ObjectKey{Name: sw.Spec.Profile, Namespace: kmetav1.NamespaceDefault}, sp); err != nil {
			return nil, errors.Wrapf(err, "cannot get switch profile %s", sw.Spec.Profile)
		}

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

	out.Summary = fmt.Sprintf("Ready: %d/%d switches", readySwitches, totalSwitches)

	return out, nil
}
