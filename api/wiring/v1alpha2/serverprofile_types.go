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

type ServerProfileNICPort struct {
	Name string `json:"name,omitempty"`
}

type ServerProfileNIC struct {
	Name  string                 `json:"name,omitempty"`
	Ports []ServerProfileNICPort `json:"ports,omitempty"`
}

// ServerProfileSpec defines the desired state of ServerProfile
type ServerProfileSpec struct {
	NICs []ServerProfileNIC `json:"nics,omitempty"`
}

// ServerProfileStatus defines the observed state of ServerProfile
type ServerProfileStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:categories=hedgehog;wiring

// ServerProfile is currently not used/implemented in the Fabric API
type ServerProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerProfileSpec   `json:"spec,omitempty"`
	Status ServerProfileStatus `json:"status,omitempty"`
}

const KindServerProfile = "ServerProfile"

//+kubebuilder:object:root=true

// ServerProfileList contains a list of ServerProfile
type ServerProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerProfile{}, &ServerProfileList{})
}

var _ meta.Object = (*ServerProfile)(nil)

func (sp *ServerProfile) Default() {
	meta.DefaultObjectMetadata(sp)
}

func (sp *ServerProfile) Validate(ctx context.Context, kube client.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(sp); err != nil {
		return nil, err
	}

	return nil, nil
}
