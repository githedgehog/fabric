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

package wiring

import (
	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type NativeData struct {
	client.WithWatch
}

func NewNativeData() (*NativeData, error) {
	scheme := runtime.NewScheme()
	if err := wiringapi.AddToScheme(scheme); err != nil {
		return nil, errors.Wrap(err, "error adding fabricv1alpha1 to the scheme")
	}
	if err := vpcapi.AddToScheme(scheme); err != nil {
		return nil, errors.Wrap(err, "error adding vpcv1alpha1 to the scheme")
	}

	return &NativeData{
		fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects().
			Build(),
	}, nil
}
