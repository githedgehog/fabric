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

package v1beta1

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
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
//+kubebuilder:resource:categories=hedgehog;wiring;fabric

// ServerProfile is currently not used/implemented in the Fabric API
type ServerProfile struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerProfileSpec   `json:"spec,omitempty"`
	Status ServerProfileStatus `json:"status,omitempty"`
}

const KindServerProfile = "ServerProfile"

//+kubebuilder:object:root=true

// ServerProfileList contains a list of ServerProfile
type ServerProfileList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []ServerProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerProfile{}, &ServerProfileList{})
}

var (
	_ meta.Object     = (*ServerProfile)(nil)
	_ meta.ObjectList = (*ServerProfileList)(nil)
)

func (spList *ServerProfileList) GetItems() []meta.Object {
	items := make([]meta.Object, len(spList.Items))
	for i := range spList.Items {
		items[i] = &spList.Items[i]
	}

	return items
}

func (sp *ServerProfile) Default() {
	meta.DefaultObjectMetadata(sp)
}

func (sp *ServerProfile) Validate(_ context.Context, _ kclient.Reader, _ *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(sp); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	return nil, nil
}
