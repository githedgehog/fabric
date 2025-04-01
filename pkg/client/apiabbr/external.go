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

package apiabbr

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ExternalPeeringSeparator = "~"
)

var (
	ExtPeeringParamSubnets  = []string{"subnets", "s"}
	ExtPeeringParamPrefixes = []string{"prefixes", "p"}

	ExtPeeringParams = [][]string{
		ExtPeeringParamSubnets,
		ExtPeeringParamPrefixes,
	}
)

func newExternalPeeringHandler(ignoreNotDefined bool) (*ObjectAbbrHandler[*vpcapi.ExternalPeering, *vpcapi.ExternalPeeringList], error) {
	return (&ObjectAbbrHandler[*vpcapi.ExternalPeering, *vpcapi.ExternalPeeringList]{
		AbbrType:          AbbrTypeExternalPeering,
		CleanupNotDefined: !ignoreNotDefined,
		AcceptedParams:    ExtPeeringParams,
		AcceptNoTypeFn:    func(abbr string) bool { return strings.Contains(abbr, ExternalPeeringSeparator) },
		NameFn: func(abbr string) string {
			return strings.ReplaceAll(abbr, ExternalPeeringSeparator, "--")
		},
		ParseObjectFn: func(name, abbr string, params AbbrParams) (*vpcapi.ExternalPeering, error) {
			permit := vpcapi.ExternalPeeringSpecPermit{}

			names := strings.Split(abbr, ExternalPeeringSeparator)
			if len(names) != 2 {
				return nil, errors.Errorf("invalid external peering abbreviation: %s", abbr)
			}
			permit.VPC.Name = names[0]
			permit.External.Name = names[1]

			for _, subnet := range params.GetStringSlice(ExtPeeringParamSubnets) {
				parts := strings.Split(subnet, ",")
				permit.VPC.Subnets = append(permit.VPC.Subnets, parts...)
			}

			for _, prefix := range params.GetStringSlice(ExtPeeringParamPrefixes) {
				parts := strings.Split(prefix, ",")
				for _, part := range parts {
					permit.External.Prefixes = append(permit.External.Prefixes, vpcapi.ExternalPeeringSpecPrefix{Prefix: part})
				}
			}

			return &vpcapi.ExternalPeering{
				TypeMeta:   kmetav1.TypeMeta{APIVersion: vpcapi.GroupVersion.String(), Kind: vpcapi.KindExternalPeering},
				ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
				Spec: vpcapi.ExternalPeeringSpec{
					Permit: permit,
				},
			}, nil
		},
		ObjectListFn: func(ctx context.Context, kube kclient.Client) (*vpcapi.ExternalPeeringList, error) {
			list := &vpcapi.ExternalPeeringList{}

			return list, kube.List(ctx, list)
		},
		CreateOrUpdateFn: func(ctx context.Context, kube kclient.Client, newObj *vpcapi.ExternalPeering) (ctrlutil.OperationResult, error) {
			extPeering := &vpcapi.ExternalPeering{ObjectMeta: newObj.ObjectMeta}

			return ctrlutil.CreateOrUpdate(ctx, kube, extPeering, func() error {
				extPeering.Spec = newObj.Spec

				return nil
			})
		},
	}).Init()
}
