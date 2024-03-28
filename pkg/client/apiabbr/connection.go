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

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	FallbackParamDisable = []string{"disable"}

	FallbackParams = [][]string{
		FallbackParamDisable,
	}
)

func newConnectionFallbackHandler(ignoreNotDefined bool) (*ObjectAbbrHandler[*wiringapi.Connection, *wiringapi.ConnectionList], error) {
	return (&ObjectAbbrHandler[*wiringapi.Connection, *wiringapi.ConnectionList]{
		AbbrType:          AbbrTypeConnectionFallback,
		CleanupNotDefined: false,
		AcceptedParams:    FallbackParams,
		ParseObjectFn: func(name, _ string, params AbbrParams) (*wiringapi.Connection, error) {
			disableFallback, err := params.GetBool(FallbackParamDisable)
			if err != nil {
				return nil, err
			}

			return &wiringapi.Connection{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: metav1.NamespaceDefault},
				Spec: wiringapi.ConnectionSpec{
					MCLAG: &wiringapi.ConnMCLAG{
						Fallback: !disableFallback,
					},
					ESLAG: &wiringapi.ConnESLAG{
						Fallback: !disableFallback,
					},
				},
			}, nil
		},
		ObjectListFn: func(ctx context.Context, kube client.Client) (*wiringapi.ConnectionList, error) {
			list := &wiringapi.ConnectionList{}

			return list, kube.List(ctx, list)
		},
		PatchExistingFn: func(conn *wiringapi.Connection) bool {
			if ignoreNotDefined {
				return false
			}

			if conn.Spec.MCLAG != nil {
				orig := conn.Spec.MCLAG.Fallback
				conn.Spec.MCLAG.Fallback = false

				return orig
			}

			if conn.Spec.ESLAG != nil {
				orig := conn.Spec.ESLAG.Fallback
				conn.Spec.ESLAG.Fallback = false

				return orig
			}

			return false
		},
		CreateOrUpdateFn: func(ctx context.Context, kube client.Client, newObj *wiringapi.Connection) (ctrlutil.OperationResult, error) {
			conn := &wiringapi.Connection{ObjectMeta: newObj.ObjectMeta}

			return ctrlutil.CreateOrUpdate(ctx, kube, conn, func() error {
				if conn.Spec.MCLAG != nil {
					conn.Spec.MCLAG.Fallback = newObj.Spec.MCLAG.Fallback
				} else if conn.Spec.ESLAG != nil {
					conn.Spec.ESLAG.Fallback = newObj.Spec.ESLAG.Fallback
				} else {
					return errors.New("only existing MCLAG and ESLAG connections are supported for fallback enforcement")
				}

				return nil
			})
		},
	}).Init()
}
