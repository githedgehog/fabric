/*
Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha2

import (
	"context"

	"go.githedgehog.com/fabric/pkg/manager/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExternalPeeringSpec defines the desired state of ExternalPeering
type ExternalPeeringSpec struct{}

// ExternalPeeringStatus defines the observed state of ExternalPeering
type ExternalPeeringStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ExternalPeering is the Schema for the externalpeerings API
type ExternalPeering struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalPeeringSpec   `json:"spec,omitempty"`
	Status ExternalPeeringStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ExternalPeeringList contains a list of ExternalPeering
type ExternalPeeringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalPeering `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalPeering{}, &ExternalPeeringList{})
}

func (peering *ExternalPeering) Default() {
	// TODO
}

func (peering *ExternalPeering) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	return nil, nil // TODO
}
