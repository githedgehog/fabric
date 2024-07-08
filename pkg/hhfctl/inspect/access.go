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
