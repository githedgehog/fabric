package inspect

import (
	"context"
	"log/slog"
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

type ConnectionIn struct {
	Name string
}

type ConnectionOut struct {
	Spec           wiringapi.ConnectionSpec             `json:"spec,omitempty"`
	Ports          []*ConnectionOutPort                 `json:"ports,omitempty"`
	VPCAttachments map[string]*vpcapi.VPCAttachmentSpec `json:"vpcAttachments,omitempty"`
	AttachedVPCs   map[string]*vpcapi.VPCSpec           `json:"attachedVPCs,omitempty"`

	// TODO if VPCLoopback show VPCPeerings and ExtPeerings
	// TODO if External show ExternalAttachments
}

type ConnectionOutPort struct {
	Name  string                         `json:"name,omitempty"`
	State *agentapi.SwitchStateInterface `json:"state,omitempty"`
}

func (out *ConnectionOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO
}

var _ Func[ConnectionIn, *ConnectionOut] = Connection

func Connection(ctx context.Context, kube client.Reader, in ConnectionIn) (*ConnectionOut, error) {
	if in.Name == "" {
		return nil, errors.New("connection name is required")
	}

	out := &ConnectionOut{
		VPCAttachments: map[string]*vpcapi.VPCAttachmentSpec{},
		AttachedVPCs:   map[string]*vpcapi.VPCSpec{},
	}

	conn := &wiringapi.Connection{}
	if err := kube.Get(ctx, client.ObjectKey{Name: in.Name, Namespace: metav1.NamespaceDefault}, conn); err != nil {
		return nil, errors.Wrap(err, "cannot get connection")
	}

	out.Spec = conn.Spec

	vpcAttches := &vpcapi.VPCAttachmentList{}
	if err := kube.List(ctx, vpcAttches, client.MatchingLabels{
		wiringapi.LabelConnection: in.Name,
	}); err != nil {
		return nil, errors.Wrap(err, "cannot list VPCAttachments")
	}

	for _, vpcAttach := range vpcAttches.Items {
		out.VPCAttachments[vpcAttach.Name] = pointer.To(vpcAttach.Spec)

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

	_, _, ports, _, err := conn.Spec.Endpoints()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get connection %s endpoints", conn.Name)
	}

	agents := map[string]*agentapi.AgentStatus{}
	for _, port := range ports {
		parts := strings.SplitN(port, "/", 2)
		swName := parts[0]
		portName := parts[1]

		slog.Warn("Port name", "name", port, "swName", swName, "portName", portName)

		agentStatus, exists := agents[swName]
		if !exists {
			agent := &agentapi.Agent{}
			if err := kube.Get(ctx, client.ObjectKey{Name: swName, Namespace: metav1.NamespaceDefault}, agent); err != nil {
				if !apierrors.IsNotFound(err) {
					return nil, errors.Wrapf(err, "failed to get Agent %s", swName)
				}

				continue
			}

			agents[swName] = &agent.Status
			agentStatus = agents[swName]
		}

		port := &ConnectionOutPort{
			Name: port,
		}

		if agentStatus.State.Interfaces != nil {
			state, exists := agentStatus.State.Interfaces[portName]
			if !exists {
				state, exists = agentStatus.State.Interfaces[portName+"/1"]
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

	return out, nil
}
