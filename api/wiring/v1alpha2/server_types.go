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

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:validation:Enum=control;
// ServerType is the type of server, could be control for control nodes or default (empty string) for everything else
type ServerType string

const (
	ServerTypeControl ServerType = "control"
	ServerTypeDefault ServerType = "" // or nil - just a server
)

// ServerSpec defines the desired state of Server
type ServerSpec struct {
	// Type is the type of server, could be control for control nodes or default (empty string) for everything else
	Type ServerType `json:"type,omitempty"`
	// Description is a description of the server
	Description string `json:"description,omitempty"`
	// Profile is the profile of the server, name of the ServerProfile object to be used for this server, currently not used by the Fabric
	Profile string `json:"profile,omitempty"`
}

// ServerStatus defines the observed state of Server
type ServerStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=srv
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`,priority=0
// +kubebuilder:printcolumn:name="Descr",type=string,JSONPath=`.spec.description`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// Server is the Schema for the servers API
type Server struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is desired state of the server
	Spec ServerSpec `json:"spec,omitempty"`
	// Status is the observed state of the server
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

var (
	_ meta.Object     = (*Server)(nil)
	_ meta.ObjectList = (*ServerList)(nil)
)

func (srvList *ServerList) GetItems() []meta.Object {
	items := make([]meta.Object, len(srvList.Items))
	for i := range srvList.Items {
		items[i] = &srvList.Items[i]
	}

	return items
}

func (server *Server) IsControl() bool {
	return server.Spec.Type == ServerTypeControl
}

func (serverSpec *ServerSpec) Labels() map[string]string {
	return map[string]string{
		LabelServerType: string(serverSpec.Type),
	}
}

func (server *Server) Default() {
	meta.DefaultObjectMetadata(server)

	if server.Labels == nil {
		server.Labels = map[string]string{}
	}

	CleanupFabricLabels(server.Labels)

	maps.Copy(server.Labels, server.Spec.Labels())
}

func (server *Server) Validate(_ context.Context, _ client.Reader, _ *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(server); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	return nil, nil
}
