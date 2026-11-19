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

package bcm

import (
	"context"

	gnmiproto "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmic/pkg/api"
	"github.com/openconfig/ygot/ygot"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
)

// GNMICClient abstracts the subset of *gnmi.Client used inside this package
// so tests can swap in a mock backed by an in-memory OC tree.
type GNMICClient interface {
	Set(ctx context.Context, req *gnmiproto.SetRequest) error
	Get(ctx context.Context, path string, dest ygot.ValidatedGoStruct, opts ...api.GNMIOption) error
	CallOperation(ctx context.Context, name string, body []byte) ([]byte, error)
}

var _ GNMICClient = (*gnmi.Client)(nil)
