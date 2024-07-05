package inspect

import (
	"context"

	"github.com/davecgh/go-spew/spew"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AccessIn struct {
	// TODO Source/Dest: Server, IP, VPCSubnet, --IPSubnet
	// source should be only from VPC subnets aka IPv4namespace subnets
}

type AccessOut struct {
	// TODO if only source specified, show everything reachable from source
	// TODO within same subnet, within same VPC, between VPCs, external
}

func (out *AccessOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO implement marshal
}

var _ Func[AccessIn, *AccessOut] = Access

func Access(ctx context.Context, kube client.Reader, in AccessIn) (*AccessOut, error) {
	out := &AccessOut{}

	// TODO implement access inspection

	return out, nil
}
