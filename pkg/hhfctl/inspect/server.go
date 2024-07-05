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

type ServerIn struct {
	Name string
}

type ServerOut struct {
	Control             bool                                 `json:"control,omitempty"`
	ControlStateSummary string                               `json:"controlStateSummary,omitempty"`
	Connections         map[string]*wiringapi.ConnectionSpec `json:"connections,omitempty"`
	VPCAttachments      map[string]*vpcapi.VPCAttachmentSpec `json:"vpcAttachments,omitempty"`
	AttachedVPCs        map[string]*vpcapi.VPCSpec           `json:"attachedVPCs,omitempty"`
}

func (out *ServerOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO implement marshal
}

var _ Func[ServerIn, *ServerOut] = Server

func Server(ctx context.Context, kube client.Reader, in ServerIn) (*ServerOut, error) {
	if in.Name == "" {
		return nil, errors.New("server name is required")
	}

	out := &ServerOut{
		Connections:    map[string]*wiringapi.ConnectionSpec{},
		VPCAttachments: map[string]*vpcapi.VPCAttachmentSpec{},
		AttachedVPCs:   map[string]*vpcapi.VPCSpec{},
	}

	srv := &wiringapi.Server{}
	if err := kube.Get(ctx, client.ObjectKey{Name: in.Name, Namespace: metav1.NamespaceDefault}, srv); err != nil {
		return nil, errors.Wrap(err, "cannot get server")
	}

	out.Control = srv.Spec.Type == wiringapi.ServerTypeControl

	if out.Control {
		skipActual := false
		agent := &agentapi.ControlAgent{}
		if err := kube.Get(ctx, client.ObjectKey{Name: in.Name, Namespace: metav1.NamespaceDefault}, agent); err != nil {
			if apierrors.IsNotFound(err) {
				skipActual = true
				slog.Warn("ControlAgent object not found", "name", in.Name)
			} else {
				return nil, errors.Wrapf(err, "failed to get ControlAgent %s", in.Name)
			}
		}

		if !skipActual {
			out.ControlStateSummary = controlStateSummary(agent)
		}
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
