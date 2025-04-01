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

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kyaml "sigs.k8s.io/yaml"
)

const (
	NativeVLAN = "native"
)

type ConnectionIn struct {
	Name string
}

type ConnectionOut struct {
	Spec                wiringapi.ConnectionSpec                  `json:"spec,omitempty"`
	Ports               []*ConnectionOutPort                      `json:"ports,omitempty"`
	VPCAttachments      map[string]*vpcapi.VPCAttachmentSpec      `json:"vpcAttachments,omitempty"`      // if server-facing conn
	AttachedVPCs        map[string]*vpcapi.VPCSpec                `json:"attachedVPCs,omitempty"`        // if server-facing conn
	ExternalAttachments map[string]*vpcapi.ExternalAttachmentSpec `json:"externalAttachments,omitempty"` // if External conn
	LoopbackWorkarounds map[string]*OutLoopbackWorkaround         `json:"loopbackWorkarounds,omitempty"` // if VPCLoopback conn
}

type ConnectionOutPort struct {
	Name  string                         `json:"name,omitempty"`
	State *agentapi.SwitchStateInterface `json:"state,omitempty"`
}

type OutLoopbackWorkaround struct {
	Link             wiringapi.SwitchToSwitchLink           `json:"link,omitempty"`
	VPCPeerings      map[string]*vpcapi.VPCPeeringSpec      `json:"vpcPeerings,omitempty"`
	ExternalPeerings map[string]*vpcapi.ExternalPeeringSpec `json:"externalPeerings,omitempty"`
}

func (out *ConnectionOut) MarshalText() (string, error) {
	str := &strings.Builder{}

	data, err := kyaml.Marshal(out.Spec)
	if err != nil {
		return "", errors.Wrap(err, "failed to yaml marshal connection spec")
	}
	str.Write(data)
	str.WriteString("\n")

	if len(out.VPCAttachments) > 0 {
		str.WriteString("VPC Attachments:\n")

		attachData := [][]string{}
		attachNames := lo.Keys(out.VPCAttachments)
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
						vlan = NativeVLAN
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
	}

	if len(out.ExternalAttachments) > 0 {
		str.WriteString("External Attachments:\n")

		attachData := [][]string{}
		attachNames := lo.Keys(out.ExternalAttachments)
		for _, attachName := range attachNames {
			attach := out.ExternalAttachments[attachName]

			attachData = append(attachData, []string{
				attachName,
				attach.External,
			})
		}
		str.WriteString(RenderTable(
			[]string{"Name", "External"},
			attachData,
		))
	}

	if len(out.LoopbackWorkarounds) > 0 {
		str.WriteString("Loopback Workarounds:\n")

		// TODO pass to a marshal func?
		noColor := !isatty.IsTerminal(os.Stdout.Fd())

		// TODO dedup
		colored := color.New(color.FgCyan).SprintFunc()
		if noColor {
			colored = func(a ...interface{}) string { return fmt.Sprint(a...) }
		}

		sep := colored("←→")

		loWoData := [][]string{}
		for _, loWo := range out.LoopbackWorkarounds {
			vpcPeerings := []string{}
			for vpcPeeringName, vpcPeering := range loWo.VPCPeerings {
				vpc1, vpc2, err := vpcPeering.VPCs()
				if err != nil {
					return "", errors.Wrapf(err, "failed to get VPCs for VPCPeering %s", vpcPeeringName)
				}

				vpcPeerings = append(vpcPeerings, fmt.Sprintf("%s (%s%s%s)", vpcPeeringName, vpc1, sep, vpc2))
			}

			extPeerings := []string{}
			for extPeeringName, extPeering := range loWo.ExternalPeerings {
				extPeerings = append(extPeerings, fmt.Sprintf("%s (%s%s%s)", extPeeringName, extPeering.Permit.VPC.Name, sep, extPeering.Permit.External.Name))
			}

			loWoData = append(loWoData, []string{
				fmt.Sprintf("%s%s%s", loWo.Link.Switch1.PortName(), sep, loWo.Link.Switch2.PortName()),
				strings.Join(vpcPeerings, "\n"),
				strings.Join(extPeerings, "\n"),
			})
		}
		str.WriteString(RenderTable(
			[]string{"Link", "VPCPeerings", "ExternalPeerings"},
			loWoData,
		))
	}

	return str.String(), nil
}

var _ Func[ConnectionIn, *ConnectionOut] = Connection

func Connection(ctx context.Context, kube kclient.Reader, in ConnectionIn) (*ConnectionOut, error) {
	if in.Name == "" {
		return nil, errors.New("connection name is required")
	}

	out := &ConnectionOut{
		VPCAttachments:      map[string]*vpcapi.VPCAttachmentSpec{},
		AttachedVPCs:        map[string]*vpcapi.VPCSpec{},
		ExternalAttachments: map[string]*vpcapi.ExternalAttachmentSpec{},
	}

	conn := &wiringapi.Connection{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: in.Name, Namespace: kmetav1.NamespaceDefault}, conn); err != nil {
		return nil, errors.Wrap(err, "cannot get connection")
	}

	out.Spec = conn.Spec

	vpcAttches := &vpcapi.VPCAttachmentList{}
	if err := kube.List(ctx, vpcAttches, kclient.MatchingLabels{
		wiringapi.LabelConnection: in.Name,
	}); err != nil {
		return nil, errors.Wrap(err, "cannot list VPCAttachments")
	}

	for _, vpcAttach := range vpcAttches.Items {
		out.VPCAttachments[vpcAttach.Name] = pointer.To(vpcAttach.Spec)

		vpcName := strings.SplitN(vpcAttach.Spec.Subnet, "/", 2)[0]
		if _, exists := out.AttachedVPCs[vpcName]; !exists {
			vpc := &vpcapi.VPC{}
			if err := kube.Get(ctx, kclient.ObjectKey{Name: vpcName, Namespace: kmetav1.NamespaceDefault}, vpc); err != nil {
				return nil, errors.Wrapf(err, "failed to get VPC %s", vpcName)
			}
			out.AttachedVPCs[vpcName] = &vpc.Spec
		}

		out.VPCAttachments[vpcAttach.Name] = pointer.To(vpcAttach.Spec)
	}

	switches, _, ports, _, err := conn.Spec.Endpoints()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get connection %s endpoints", conn.Name)
	}

	agents := map[string]*agentapi.Agent{}
	for _, port := range ports {
		parts := strings.SplitN(port, "/", 2)
		swName := parts[0]
		portName := parts[1]

		agent, exists := agents[swName]
		if !exists {
			agent = &agentapi.Agent{}
			if err := kube.Get(ctx, kclient.ObjectKey{Name: swName, Namespace: kmetav1.NamespaceDefault}, agent); err != nil {
				if !kapierrors.IsNotFound(err) {
					return nil, errors.Wrapf(err, "failed to get Agent %s", swName)
				}

				continue
			}

			agents[swName] = agent
		}

		port := &ConnectionOutPort{
			Name: port,
		}

		if agent.Status.State.Interfaces != nil {
			state, exists := agent.Status.State.Interfaces[portName]
			if !exists {
				state, exists = agent.Status.State.Interfaces[portName+"/1"]
				if exists {
					port.Name += "/1"
				}
			}

			if exists {
				port.State = &state
			}
		}

		out.Ports = append(out.Ports, port)
	}

	if conn.Spec.VPCLoopback != nil {
		if len(switches) != 1 {
			return nil, errors.New("VPCLoopback connection must have exactly one switch")
		}

		agent, exist := agents[switches[0]]
		if !exist {
			return nil, errors.Errorf("failed to get Agent %s for VPCLoopback", switches[0])
		}

		out.LoopbackWorkarounds, err = loopbackWorkaroundInfo(ctx, kube, agent)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get loopback workaround info")
		}
	}

	if conn.Spec.External != nil {
		extAttaches := &vpcapi.ExternalAttachmentList{}
		if err := kube.List(ctx, extAttaches, kclient.MatchingLabels{
			wiringapi.LabelConnection: conn.Name,
		}); err != nil {
			return nil, errors.Wrap(err, "cannot list ExternalAttachments")
		}

		for _, extAttach := range extAttaches.Items {
			out.ExternalAttachments[extAttach.Name] = pointer.To(extAttach.Spec)
		}
	}

	return out, nil
}

func loopbackWorkaroundInfo(ctx context.Context, kube kclient.Reader, agent *agentapi.Agent) (map[string]*OutLoopbackWorkaround, error) {
	out := map[string]*OutLoopbackWorkaround{}

	for workaround, link := range agent.Spec.Catalog.LooopbackWorkaroundLinks {
		loWo, exists := out[link]
		if !exists {
			ports := strings.Split(link, "--")
			if len(ports) != 2 {
				return nil, errors.Errorf("invalid switch link %s for workaround %s", link, workaround)
			}

			loWo = &OutLoopbackWorkaround{
				Link: wiringapi.SwitchToSwitchLink{
					Switch1: wiringapi.NewBasePortName(ports[0]),
					Switch2: wiringapi.NewBasePortName(ports[1]),
				},
				VPCPeerings:      map[string]*vpcapi.VPCPeeringSpec{},
				ExternalPeerings: map[string]*vpcapi.ExternalPeeringSpec{},
			}

			out[link] = loWo
		}

		if strings.HasPrefix(workaround, librarian.LoWorkaroundReqPrefixVPC) {
			vpcPeeringName := strings.TrimPrefix(workaround, librarian.LoWorkaroundReqPrefixVPC)

			vpcPeering := &vpcapi.VPCPeering{}
			if err := kube.Get(ctx, kclient.ObjectKey{Name: vpcPeeringName, Namespace: kmetav1.NamespaceDefault}, vpcPeering); err != nil {
				return nil, errors.Wrapf(err, "failed to get VPCPeering %s", vpcPeeringName)
			}

			loWo.VPCPeerings[vpcPeeringName] = &vpcPeering.Spec
		} else if strings.HasPrefix(workaround, librarian.LoWorkaroundReqPrefixExt) {
			extPeeringName := strings.TrimPrefix(workaround, librarian.LoWorkaroundReqPrefixExt)

			extPeering := &vpcapi.ExternalPeering{}
			if err := kube.Get(ctx, kclient.ObjectKey{Name: extPeeringName, Namespace: kmetav1.NamespaceDefault}, extPeering); err != nil {
				return nil, errors.Wrapf(err, "failed to get ExternalPeering %s", extPeeringName)
			}

			loWo.ExternalPeerings[extPeeringName] = &extPeering.Spec
		} else {
			return nil, errors.Errorf("invalid loopback workaround %s", workaround)
		}
	}

	return out, nil
}
