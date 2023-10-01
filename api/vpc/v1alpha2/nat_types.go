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

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NATSpec defines the desired state of NAT
type NATSpec struct {
	DNAT DNAT `json:"dnat"`
}

type DNAT struct {
	Pool []string `json:"pool"`
}

// NATStatus defines the observed state of NAT
type NATStatus struct {
	DNAT DNATStatus `json:"dnat"`
}

type DNATStatus struct {
	Available    int      `json:"available"`
	Assigned     int      `json:"assigned"`
	AssignedList []string `json:"assignedList"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric;wiring
// +kubebuilder:printcolumn:name="Subnet",type=string,JSONPath=`.spec.subnet`,priority=0
// +kubebuilder:printcolumn:name="DNAT",type=string,JSONPath=`.spec.dnat`,priority=0
// +kubebuilder:printcolumn:name="DNAT_AVAILABLE",type=boolean,JSONPath=`.status.dnat.available`,priority=0
// +kubebuilder:printcolumn:name="DNAT_ASSIGNED",type=boolean,JSONPath=`.status.dnat.assigned`,priority=0
// +kubebuilder:printcolumn:name="DNAT_ASSIGNED_L",type=boolean,JSONPath=`.status.dnat.assignedList`,priority=1
// +kubebuilder:printcolumn:name="Age",type=string,JSONPath=`.metadata.creationTimestamp`,priority=0
// NAT is the Schema for the nats API
type NAT struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NATSpec   `json:"spec,omitempty"`
	Status NATStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NATList contains a list of NAT
type NATList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NAT `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NAT{}, &NATList{})
}

func (nat *NAT) Default() {
}

func (nat *NAT) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	if nat.Name != "default" {
		return nil, errors.Errorf("NAT name must be default") // TODO support more than one NAT
	}

	return nil, nil
}
