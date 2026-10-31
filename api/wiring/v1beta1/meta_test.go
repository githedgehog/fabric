// Copyright 2026 Hedgehog
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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

func TestTopologySummary(t *testing.T) {
	for _, tt := range []struct {
		name    string
		fabric  string
		domains []string
		want    string
	}{
		{name: "default-fabric-only", fabric: "default"},
		{name: "default-domain", fabric: "default", domains: []string{"default"}},
		{name: "fabric-only", fabric: "f1", want: "f1"},
		{name: "fabric-default-domain", fabric: "f1", domains: []string{"default"}, want: "f1/default"},
		{name: "default-fabric-domains", fabric: "default", domains: []string{"d1", "d2"}, want: "default/d1,d2"},
		{name: "fabric-domain", fabric: "f1", domains: []string{"d1"}, want: "f1/d1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, wiringapi.TopologySummary(tt.fabric, tt.domains...))
		})
	}
}

func TestDefaultTopologyAnnotation(t *testing.T) {
	sw := withName("sw-1", &wiringapi.Switch{})
	sw.Spec.Topology.Fabric = "f1"
	sw.Spec.Topology.Domains = []string{"d2", "d1"}
	sw.Default()
	require.Equal(t, "f1/d1,d2", sw.Annotations[wiringapi.AnnotationTopology])

	sw.Spec.Topology.Fabric = ""
	sw.Spec.Topology.Domains = nil
	sw.Default()
	require.NotContains(t, sw.Annotations, wiringapi.AnnotationTopology)
}
