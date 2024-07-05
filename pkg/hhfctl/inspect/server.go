package inspect

import (
	"context"
	"log/slog"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServerIn struct {
	Name string
}

type ServerOut struct {
	Control             bool   `json:"control,omitempty"`
	ControlStateSummary string `json:"controlStateSummary,omitempty"`

	// TODO connections and attachments
}

func (out *ServerOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO
}

var _ Func[ServerIn, *ServerOut] = Server

func Server(ctx context.Context, kube client.Reader, in ServerIn) (*ServerOut, error) {
	if in.Name == "" {
		return nil, errors.New("server name is required")
	}

	out := &ServerOut{}

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

	return out, nil
}
