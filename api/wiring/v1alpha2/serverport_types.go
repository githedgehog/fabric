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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ServerPortInfo struct {
	NicName  string   `json:"nicName,omitempty"`
	NicIndex uint8    `json:"nicIndex,omitempty"`
	Neighbor Neighbor `json:"neighbor,omitempty"`
}

type Bundled struct {
	ID      string           `json:"id,omitempty"`
	Type    BundleType       `json:"type,omitempty"`
	Members []ServerPortInfo `json:"members,omitempty"`
	Config  BundleConfig     `json:"config,omitempty"`
}

type CtrlMgmt struct {
	VLAN      uint16 `json:"vlan,omitempty"`
	IPAddress string `json:"ipAddress,omitempty"`
}

// ServerPortSpec defines the desired state of ServerPort
type ServerPortSpec struct {
	Bundled   *Bundled        `json:"bundled,omitempty"`
	Unbundled *ServerPortInfo `json:"unbundled,omitempty"`
	CtrlMgmt  *CtrlMgmt       `json:"ctrlMgmt,omitempty"`
}

// ServerPortStatus defines the observed state of ServerPort
type ServerPortStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ServerPort is the Schema for the serverports API
type ServerPort struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerPortSpec   `json:"spec,omitempty"`
	Status ServerPortStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServerPortList contains a list of ServerPort
type ServerPortList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerPort `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerPort{}, &ServerPortList{})
}
