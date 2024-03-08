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

package v1alpha2

import (
	"context"

	"go.githedgehog.com/fabric/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RackPosition defines the geopraphical position of the rack in a datacenter
type RackPosition struct {
	Location string `json:"location,omitempty"`
	Aisle    string `json:"aisle,omitempty"`
	Row      string `json:"row,omitempty"`
}

// RackSpec defines the properties of a rack which we are modelling
type RackSpec struct {
	NumServers       uint32       `json:"numServers,omitempty"`
	HasControlNode   bool         `json:"hasControlNode,omitempty"`
	HasConsoleServer bool         `json:"hasConsoleServer,omitempty"`
	Position         RackPosition `json:"position,omitempty"`
}

// RackStatus defines the observed state of Rack
type RackStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric
// Rack is the Schema for the racks API
type Rack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RackSpec   `json:"spec,omitempty"`
	Status RackStatus `json:"status,omitempty"`
}

const KindRack = "Rack"

//+kubebuilder:object:root=true

// RackList contains a list of Rack
type RackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rack `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Rack{}, &RackList{})
}

var _ meta.Object = (*Rack)(nil)

func (rack *Rack) Default() {
	meta.DefaultObjectMetadata(rack)
}

func (rack *Rack) Validate(ctx context.Context, kube client.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(rack); err != nil {
		return nil, err
	}

	return nil, nil
}
