package inspect

import (
	"context"

	"github.com/davecgh/go-spew/spew"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AccessIn struct {
	// TODO Source/Dest: Server, IP, VPCSubnet, IPSubnet
}

type AccessOut struct{}

func (out *AccessOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO
}

var _ Func[AccessIn, *AccessOut] = Access

func Access(ctx context.Context, kube client.Reader, in AccessIn) (*AccessOut, error) {
	out := &AccessOut{}

	// TODO impl

	return out, nil
}
