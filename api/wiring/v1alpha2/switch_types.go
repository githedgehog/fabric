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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type LLDPConfig struct {
	HelloTimer        time.Duration `json:"helloTimer,omitempty"`
	SystemName        string        `json:"name,omitempty"`
	SystemDescription string        `json:"description,omitempty"`
}

// SwitchSpec defines the desired state of Switch
type SwitchSpec struct {
	Profile     string      `json:"profile,omitempty"`
	Location    Location    `json:"location,omitempty"`
	LocationSig LocationSig `json:"locationSig,omitempty"`
	LLDPConfig  LLDPConfig  `json:"lldp,omitempty"`
}

// SwitchStatus defines the observed state of Switch
type SwitchStatus struct {
	// TODO: add port status fields
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:categories=hedgehog;wiring

// Switch is the Schema for the switches API
//
// All switches should always have 1 labels defined: wiring.githedgehog.com/rack. It represents name of the rack it
// belongs to.
type Switch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SwitchSpec   `json:"spec,omitempty"`
	Status SwitchStatus `json:"status,omitempty"`
}

const KindSwitch = "Switch"

//+kubebuilder:object:root=true

// SwitchList contains a list of Switch
type SwitchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Switch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Switch{}, &SwitchList{})
}
