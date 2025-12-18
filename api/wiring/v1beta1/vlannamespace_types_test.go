// Copyright 2025 Hedgehog
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

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func withName[T kclient.Object](name string, obj T) T {
	obj.SetName(name)
	obj.SetNamespace(kmetav1.NamespaceDefault)

	return obj
}

func vlanNSGen(name string, ranges []meta.VLANRange) *wiringapi.VLANNamespace {
	return withName(name, &wiringapi.VLANNamespace{
		Spec: wiringapi.VLANNamespaceSpec{
			Ranges: ranges,
		},
	})
}

func TestValidate(t *testing.T) {
	cfg := &meta.FabricConfig{
		VPCIRBVLANRanges:       []meta.VLANRange{{From: 3000, To: 3199}},
		TH5WorkaroundVLANRange: []meta.VLANRange{{From: 3900, To: 3999}},
	}
	for _, tt := range []struct {
		name   string
		vlanNS *wiringapi.VLANNamespace
		err    bool
	}{
		{
			name:   "fabric-collision",
			vlanNS: vlanNSGen("ns-1", []meta.VLANRange{{From: 2900, To: 3000}}),
			err:    true,
		},
		{
			name:   "th5-collision",
			vlanNS: vlanNSGen("ns-1", []meta.VLANRange{{From: 3500, To: 4500}}),
			err:    true,
		},
		{
			name:   "no-collision",
			vlanNS: vlanNSGen("ns-1", []meta.VLANRange{{From: 10, To: 1500}}),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.vlanNS.Validate(t.Context(), nil, cfg)
			if tt.err {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
		})
	}
}
