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
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServerIn struct {
	Name string
}

type ServerOut struct {
	Name           string                               `json:"name,omitempty"`
	ControlState   *AgentState                          `json:"controlState,omitempty"`
	Connections    map[string]*wiringapi.ConnectionSpec `json:"connections,omitempty"`
	VPCAttachments map[string]*vpcapi.VPCAttachmentSpec `json:"vpcAttachments,omitempty"`
	AttachedVPCs   map[string]*vpcapi.VPCSpec           `json:"attachedVPCs,omitempty"`
}

func (out *ServerOut) MarshalText() (string, error) {
	str := &strings.Builder{}

	// TODO pass to a marshal func?
	noColor := !isatty.IsTerminal(os.Stdout.Fd())

	if len(out.Connections) > 0 {
		str.WriteString("Connections:\n")

		connData := [][]string{}
		connNames := maps.Keys(out.Connections)
		for _, connName := range connNames {
			conn := out.Connections[connName]

			connData = append(connData, []string{
				connName,
				conn.Type(),
				strings.Join(conn.LinkSummary(noColor), "\n"),
			})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "Type", "Links"},
			connData,
		))
	} else {
		str.WriteString("No connections\n")
	}

	if len(out.VPCAttachments) > 0 {
		str.WriteString("VPC Attachments:\n")

		attachData := [][]string{}
		attachNames := maps.Keys(out.VPCAttachments)
		for _, attachName := range attachNames {
			attach := out.VPCAttachments[attachName]

			subnet := ""
			vlan := ""
			vpcName := strings.SplitN(attach.Subnet, "/", 2)[0]
			subnetName := strings.SplitN(attach.Subnet, "/", 2)[1]
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
	} else {
		str.WriteString("No VPC attachments\n")
	}

	return str.String(), nil
}

var _ Func[ServerIn, *ServerOut] = Server

func Server(ctx context.Context, kube client.Reader, in ServerIn) (*ServerOut, error) {
	if in.Name == "" {
		return nil, errors.New("server name is required")
	}

	out := &ServerOut{
		Name:           in.Name,
		Connections:    map[string]*wiringapi.ConnectionSpec{},
		VPCAttachments: map[string]*vpcapi.VPCAttachmentSpec{},
		AttachedVPCs:   map[string]*vpcapi.VPCSpec{},
	}

	srv := &wiringapi.Server{}
	if err := kube.Get(ctx, client.ObjectKey{Name: in.Name, Namespace: metav1.NamespaceDefault}, srv); err != nil {
		return nil, errors.Wrap(err, "cannot get server")
	}

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns, client.MatchingLabels{
		wiringapi.ListLabelServer(in.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrap(err, "cannot list connections")
	}

	for _, conn := range conns.Items {
		out.Connections[conn.Name] = pointer.To(conn.Spec)

		vpcAttaches := &vpcapi.VPCAttachmentList{}
		if err := kube.List(ctx, vpcAttaches, client.MatchingLabels{
			wiringapi.LabelConnection: conn.Name,
		}); err != nil {
			return nil, errors.Wrap(err, "cannot list VPC attachments")
		}

		for _, vpcAttach := range vpcAttaches.Items {
			out.VPCAttachments[vpcAttach.Name] = pointer.To(vpcAttach.Spec)

			vpcName := strings.SplitN(vpcAttach.Spec.Subnet, "/", 2)[0]

			vpc := &vpcapi.VPC{}
			if err := kube.Get(ctx, client.ObjectKey{Name: vpcName, Namespace: metav1.NamespaceDefault}, vpc); err != nil {
				return nil, errors.Wrapf(err, "cannot get VPC %s", vpcName)
			}

			out.AttachedVPCs[vpcName] = pointer.To(vpc.Spec)
		}
	}

	return out, nil
}

func controlStateSummary(agent agentapi.ControlAgent) *AgentState {
	res := &AgentState{
		Summary: "Unknown",
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
