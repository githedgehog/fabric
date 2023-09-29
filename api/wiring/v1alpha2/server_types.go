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
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:validation:Enum=control;
type ServerType string

const (
	ServerTypeControl ServerType = "control"
	ServerTypeDefault ServerType = "" // or nil - just a server
)

// ServerSpec defines the desired state of Server
type ServerSpec struct {
	Type    ServerType `json:"type,omitempty"`
	Profile string     `json:"profile,omitempty"`
}

// ServerStatus defines the observed state of Server
type ServerStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=srv
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`,priority=0
// +kubebuilder:printcolumn:name="Rack",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/rack`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// Server is the Schema for the servers API
type Server struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerSpec   `json:"spec,omitempty"`
	Status ServerStatus `json:"status,omitempty"`
}

const KindServer = "Server"

//+kubebuilder:object:root=true

// ServerList contains a list of Server
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Server `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}

func (s *Server) IsControl() bool {
	return s.Spec.Type == ServerTypeControl
}

func (s *ServerSpec) Labels() map[string]string {
	return map[string]string{
		LabelServerType: string(s.Type),
	}
}

func (server *Server) Default() {
	if server.Labels == nil {
		server.Labels = map[string]string{}
	}

	CleanupFabricLabels(server.Labels)

	maps.Copy(server.Labels, server.Spec.Labels())
}

func (server *Server) Validate() (admission.Warnings, error) {
	return nil, nil
}
