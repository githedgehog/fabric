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
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type FabricIn struct {
	PortMapping bool
	// Name limits the output to a single Fabric object
	Name string
}

type FabricOut struct {
	Summary    string               `json:"summary,omitempty"`
	Controller *FabricOutController `json:"controller,omitempty"`
	Fabrics    []*FabricOutFabric   `json:"fabrics,omitempty"`
	Switches   []*FabricOutSwitch   `json:"switches,omitempty"`
}

// FabricOutController is the state of the fabric controller, nothing is deployed while it isn't ready
type FabricOutController struct {
	Ready       bool                               `json:"ready"`
	Version     string                             `json:"version,omitempty"`
	Initialized string                             `json:"initialized,omitempty"`
	State       string                             `json:"state,omitempty"`
	Message     string                             `json:"message,omitempty"`
	Maintenance bool                               `json:"maintenance,omitempty"`
	Error       string                             `json:"error,omitempty"`
	Problems    []fcintapi.FabricControllerProblem `json:"problems,omitempty"`
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

	if fc := out.Controller; fc != nil && !fc.Ready {
		str.WriteString("Fabric controller isn't ready, nothing is deployed:\n")
		maintenance := ""
		if fc.Maintenance {
			maintenance = "true"
		}
		str.WriteString(RenderTable(
			[]string{"Version", "Initialized", "State", "Message", "Maintenance"},
			[][]string{{fc.Version, fc.Initialized, fc.State, fc.Message, maintenance}},
		))

		if fc.Error != "" {
			str.WriteString("\nError: " + fc.Error + "\n")
		}

		if len(fc.Problems) > 0 {
			str.WriteString("\nObjects to fix in maintenance (only the first ones are listed):\n")
			problemData := make([][]string, 0, len(fc.Problems))
			for _, p := range fc.Problems {
				problemData = append(problemData, []string{p.Kind, p.Namespace, p.Name, p.Reason, p.Message})
			}
			str.WriteString(RenderTable([]string{"Kind", "Namespace", "Name", "Reason", "Message"}, problemData))
		}

		str.WriteString("\n")
	}

	str.WriteString("Fabrics:\n")

	fabData := [][]string{} //nolint:prealloc
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

	swData := [][]string{} //nolint:prealloc
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
			sw.State.Notes,
		})
	}
	str.WriteString(RenderTable(
		[]string{"Name", "Fabric", "Profile", "Role", "Groups", "Serial", "State", "Gen", "Applied", "Heartbeat", "Notes"},
		swData,
	))

	return str.String(), nil
}

var _ Func[FabricIn, *FabricOut] = Fabric

func Fabric(ctx context.Context, kube kclient.Reader, in FabricIn) (*FabricOut, error) {
	out := &FabricOut{}

	fc, err := fabricController(ctx, kube)
	if err != nil {
		return nil, err
	}
	out.Controller = fc

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

	// agents carry the labels of their switches
	fabricSel := kclient.MatchingLabels(apiutil.SwitchFilter{Fabric: in.Name}.FabricLabels(false))

	swList := &wiringapi.SwitchList{}
	if err := kube.List(ctx, swList, fabricSel); err != nil {
		return nil, fmt.Errorf("cannot list switches: %w", err)
	}

	agList := &agentapi.AgentList{}
	if err := kube.List(ctx, agList, fabricSel); err != nil {
		return nil, fmt.Errorf("cannot list agents: %w", err)
	}
	agents := map[string]*agentapi.Agent{}
	for idx := range agList.Items {
		agents[agList.Items[idx].Name] = &agList.Items[idx]
	}

	profiles := map[string]*wiringapi.SwitchProfile{}
	for _, sw := range swList.Items {
		swName := sw.Name
		fabricName := sw.Spec.Topology.Fabric

		totalSwitches++

		sp, ok := profiles[sw.Spec.Profile]
		if !ok {
			sp = &wiringapi.SwitchProfile{}
			if err := kube.Get(ctx, kclient.ObjectKey{Name: sw.Spec.Profile, Namespace: kmetav1.NamespaceDefault}, sp); err != nil {
				return nil, fmt.Errorf("cannot get switch profile %s: %w", sw.Spec.Profile, err)
			}
			profiles[sw.Spec.Profile] = sp
		}

		skipActual := false
		agent, ok := agents[swName]
		if !ok {
			skipActual = true
			agent = &agentapi.Agent{}
			slog.Warn("Agent object not found", "name", swName)
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

func fabricController(ctx context.Context, kube kclient.Reader) (*FabricOutController, error) {
	// it lives in the fabric controller's namespace, which isn't known here
	fcs := &fcintapi.FabricControllerList{}
	if err := kube.List(ctx, fcs); err != nil {
		return nil, fmt.Errorf("listing fabric controllers: %w", err)
	}

	var fc *fcintapi.FabricController
	for idx := range fcs.Items {
		if fcs.Items[idx].Name != fcintapi.FabricControllerName {
			continue
		}
		if fc != nil {
			return nil, fmt.Errorf("fabric controller found in namespaces %s and %s", fc.Namespace, fcs.Items[idx].Namespace) //nolint:err113
		}
		fc = &fcs.Items[idx]
	}
	if fc == nil {
		return &FabricOutController{State: "Missing", Message: "not created yet"}, nil
	}

	out := &FabricOutController{
		// the running version is the one it records itself, kubectl-fabric may be another one
		Ready:       fc.IsReady(fc.Status.Version),
		Version:     fc.Status.Version,
		Initialized: fc.Status.InitializedVersion,
		Maintenance: fc.Spec.Maintenance,
		Error:       fc.Status.Error,
		Problems:    fc.Status.Problems,
	}
	if ready := kmeta.FindStatusCondition(fc.Status.Conditions, fcintapi.ConditionReady); ready != nil {
		out.State = ready.Reason
		out.Message = ready.Message
	}

	return out, nil
}
