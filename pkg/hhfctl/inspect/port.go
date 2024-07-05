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
	"log/slog"
	"slices"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PortIn struct {
	Port string
}

type PortOut struct {
	ConnectionName *string                              `json:"connectionName,omitempty"`
	Connection     *wiringapi.ConnectionSpec            `json:"connection,omitempty"`
	InterfaceState *agentapi.SwitchStateInterface       `json:"interfaceState,omitempty"`
	BreakoutState  *agentapi.SwitchStateBreakout        `json:"breakoutState,omitempty"`
	VPCAttachments map[string]*vpcapi.VPCAttachmentSpec `json:"vpcAttachments,omitempty"`
	AttachedVPCs   map[string]*vpcapi.VPCSpec           `json:"attachedVPCs,omitempty"`

	// TODO if VPCLoopback show VPCPeerings and ExtPeerings
	// TODO if External show ExternalAttachments
}

func (out *PortOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO
}

var _ Func[PortIn, *PortOut] = Port

func Port(ctx context.Context, kube client.Reader, in PortIn) (*PortOut, error) {
	if in.Port == "" {
		return nil, errors.New("port is required")
	}

	out := &PortOut{
		VPCAttachments: map[string]*vpcapi.VPCAttachmentSpec{},
		AttachedVPCs:   map[string]*vpcapi.VPCSpec{},
	}

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns); err != nil {
		return nil, errors.Wrap(err, "cannot list Connections")
	}

	for _, conn := range conns.Items {
		_, _, ports, _, err := conn.Spec.Endpoints()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get connection %s endpoints", conn.Name)
		}

		if !(slices.Contains(ports, in.Port) || strings.Count(in.Port, "/") == 3 && slices.Contains(ports, strings.TrimSuffix(in.Port, "/1"))) {
			continue
		}

		out.ConnectionName = pointer.To(conn.Name)
		out.Connection = pointer.To(conn.Spec)
	}

	swName := strings.SplitN(in.Port, "/", 2)[0]

	skip := false
	sw := &wiringapi.Switch{}
	if err := kube.Get(ctx, client.ObjectKey{Name: swName, Namespace: metav1.NamespaceDefault}, sw); err != nil {
		if apierrors.IsNotFound(err) {
			skip = true
			slog.Warn("Switch object not found", "name", swName)
		} else {
			return nil, errors.Wrapf(err, "failed to get Switch %s", swName)
		}
	}

	agent := &agentapi.Agent{}
	if err := kube.Get(ctx, client.ObjectKey{Name: swName, Namespace: metav1.NamespaceDefault}, agent); err != nil {
		if apierrors.IsNotFound(err) {
			skip = true
			slog.Warn("Agent object not found", "name", swName)
		} else {
			return nil, errors.Wrapf(err, "failed to get Agent %s", swName)
		}
	}

	if skip {
		slog.Warn("Skipping actual port state", "name", in.Port)

		return out, nil
	}

	portName := strings.SplitN(in.Port, "/", 2)[1]

	if agent.Status.State.Interfaces != nil {
		state, exists := agent.Status.State.Interfaces[portName]
		if exists {
			out.InterfaceState = &state
		}
	}

	if agent.Status.State.Breakouts != nil {
		state, exists := agent.Status.State.Breakouts[portName]
		if exists {
			out.BreakoutState = &state
		}
	}

	if out.Connection != nil && out.ConnectionName != nil {
		conn := out.Connection
		if conn.VPCLoopback != nil {
			slog.Warn("Port is used for VPC loopback, but it's not yet supported for inspection", "name", in.Port)
			// TODO find vpc peerings
		} else if conn.Unbundled != nil || conn.MCLAG != nil || conn.ESLAG != nil {
			vpcAttaches := &vpcapi.VPCAttachmentList{}
			if err := kube.List(ctx, vpcAttaches, client.MatchingLabels{
				wiringapi.LabelConnection: *out.ConnectionName,
			}); err != nil {
				return nil, errors.Wrapf(err, "failed to list VPCAttachments for connection %s", *out.ConnectionName)
			}

			for _, vpcAttach := range vpcAttaches.Items {
				if vpcAttach.Spec.Connection != *out.ConnectionName {
					continue
				}

				vpcName := strings.SplitN(vpcAttach.Spec.Subnet, "/", 2)[0]
				if _, exists := out.AttachedVPCs[vpcName]; !exists {
					vpc := &vpcapi.VPC{}
					if err := kube.Get(ctx, client.ObjectKey{Name: vpcName, Namespace: metav1.NamespaceDefault}, vpc); err != nil {
						return nil, errors.Wrapf(err, "failed to get VPC %s", vpcName)
					}
					out.AttachedVPCs[vpcName] = &vpc.Spec
				}

				out.VPCAttachments[vpcAttach.Name] = pointer.To(vpcAttach.Spec)
			}
		}
	}

	return out, nil
}
