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
	"go.githedgehog.com/fabric/pkg/util/testutil"
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

func TestDefaultNormalizesRanges(t *testing.T) {
	ns := vlanNSGen("ns-1", []meta.VLANRange{{From: 1100, To: 1200}, {From: 1000, To: 2000}, {From: 10}})
	ns.Default()
	require.Equal(t, []meta.VLANRange{{From: 10, To: 10}, {From: 1000, To: 2000}}, ns.Spec.Ranges)
}

func TestValidate(t *testing.T) {
	cfg := &meta.FabricConfig{
		VPCIRBVLANRanges:       []meta.VLANRange{{From: 3000, To: 3199}},
		TH5WorkaroundVLANRange: []meta.VLANRange{{From: 3900, To: 3999}},
	}
	for _, tt := range []struct {
		name   string
		vlanNS *wiringapi.VLANNamespace
		err    string
	}{
		{
			name:   "fabric-collision",
			vlanNS: vlanNSGen("ns-1", []meta.VLANRange{{From: 2900, To: 3000}}),
			err:    "ranges overlap with Fabric reserved VLANs",
		},
		{
			name:   "th5-collision",
			vlanNS: vlanNSGen("ns-1", []meta.VLANRange{{From: 3500, To: 3950}}),
			err:    "ranges overlap with VLANs reserved for the TH5 workaround",
		},
		{
			name:   "no-collision",
			vlanNS: vlanNSGen("ns-1", []meta.VLANRange{{From: 10, To: 1500}}),
		},
		{
			name:   "no-ranges",
			vlanNS: vlanNSGen("ns-1", nil),
			err:    "spec.ranges: Required value",
		},
		{
			name: "too-many-ranges",
			vlanNS: vlanNSGen("ns-1", func() []meta.VLANRange {
				ranges := make([]meta.VLANRange, 0, 21)
				for idx := range uint16(21) {
					ranges = append(ranges, meta.VLANRange{From: 10 + 2*idx})
				}

				return ranges
			}()),
			err: "spec.ranges: Too many: 21",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.vlanNS.Validate(t.Context(), nil, cfg)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				testutil.RequireFieldError(t, err, tt.vlanNS)

				return
			}
			require.NoError(t, err)
		})
	}
}
