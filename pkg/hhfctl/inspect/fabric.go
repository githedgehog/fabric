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
	// Name limits the output to a single Fabric object
	Name string
}

type FabricOut struct {
	Summary  string             `json:"summary,omitempty"`
	Fabrics  []*FabricOutFabric `json:"fabrics,omitempty"`
	Switches []*FabricOutSwitch `json:"switches,omitempty"`
}

type FabricOutFabric struct {
	Name string               `json:"name,omitempty"`
	Spec wiringapi.FabricSpec `json:"spec,omitempty"`
}

type FabricOutSwitch struct {
	Name               string      `json:"name,omitempty"`
	Fabric             string      `json:"fabric,omitempty"`
	Serial             string      `json:"serial,omitempty"`
	Software           string      `json:"software,omitempty"`
	ProfileDisplayName string      `json:"profileDisplayName,omitempty"`
	State              *AgentState `json:"state,omitempty"`
	Role               string      `json:"role,omitempty"`
	Groups             []string    `json:"groups,omitempty"`
}

func (out *FabricOut) MarshalText(_ FabricIn, now time.Time) (string, error) {
	str := &strings.Builder{}

	str.WriteString("Fabrics:\n")

	fabData := [][]string{}
	for _, fab := range out.Fabrics {
		spineASNs, gatewayASNs := []string{}, []string{}
		for name, domain := range fab.Spec.Domains {
			spineASNs = append(spineASNs, fmt.Sprintf("%d (%s)", domain.SpineASN, name))
			gatewayASNs = append(gatewayASNs, fmt.Sprintf("%d (%s)", domain.GatewayASN, name))
		}
		slices.Sort(spineASNs)
		slices.Sort(gatewayASNs)

		fabData = append(fabData, []string{
			fab.Name,
			fmt.Sprintf("%d-%d", fab.Spec.LeafASNStart, fab.Spec.LeafASNEnd),
			strings.Join(spineASNs, ", "),
			strings.Join(gatewayASNs, ", "),
		})
	}
	str.WriteString(RenderTable(
		[]string{"Name", "Leaf ASN Range", "Spine ASN", "Gateway ASN"},
		fabData,
	))

	str.WriteString("\nSwitches:\n")

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
			sw.Fabric,
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
		[]string{"Name", "Fabric", "Profile", "Role", "Groups", "Serial", "State", "Gen", "Applied", "Heartbeat"},
		swData,
	))

	return str.String(), nil
}

var _ Func[FabricIn, *FabricOut] = Fabric

func Fabric(ctx context.Context, kube kclient.Reader, in FabricIn) (*FabricOut, error) {
	out := &FabricOut{}

	fabList := &wiringapi.FabricList{}
	if err := kube.List(ctx, fabList); err != nil {
		return nil, errors.Wrap(err, "cannot list fabrics")
	}
	for _, fab := range fabList.Items {
		if in.Name != "" && fab.Name != in.Name {
			continue
		}
		out.Fabrics = append(out.Fabrics, &FabricOutFabric{Name: fab.Name, Spec: fab.Spec})
	}
	if in.Name != "" && len(out.Fabrics) == 0 {
		return nil, errors.Errorf("fabric %s not found", in.Name)
	}
	slices.SortFunc(out.Fabrics, func(a, b *FabricOutFabric) int {
		return strings.Compare(a.Name, b.Name)
	})

	totalSwitches := 0
	readySwitches := 0

	swList := &wiringapi.SwitchList{}
	if err := kube.List(ctx, swList); err != nil {
		return nil, errors.Wrap(err, "cannot list switches")
	}

	for _, sw := range swList.Items {
		swName := sw.Name
		fabricName := sw.Spec.Topology.Fabric
		if in.Name != "" && fabricName != in.Name {
			continue
		}

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
			Fabric:             fabricName,
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
