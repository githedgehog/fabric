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

type SwitchProfileLimits struct {
	VPC    int `json:"vpc,omitempty"`
	Policy int `json:"policy,omitempty"`
}

type SwitchProfilePort struct {
	ID         int    `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Management bool   `json:"management,omitempty"`
}

// SwitchProfileSpec defines the desired state of SwitchProfile
type SwitchProfileSpec struct {
	Limits SwitchProfileLimits `json:"limits,omitempty"`
	Ports  []SwitchProfilePort `json:"ports,omitempty"`
}

// SwitchProfileStatus defines the observed state of SwitchProfile
type SwitchProfileStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:categories=hedgehog;wiring

// SwitchProfile is currently not used/implemented in the Fabric API
type SwitchProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SwitchProfileSpec   `json:"spec,omitempty"`
	Status SwitchProfileStatus `json:"status,omitempty"`
}

const KindSwitchProfile = "SwitchProfile"

//+kubebuilder:object:root=true

// SwitchProfileList contains a list of SwitchProfile
type SwitchProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SwitchProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SwitchProfile{}, &SwitchProfileList{})
}

var (
	_ meta.Object     = (*SwitchProfile)(nil)
	_ meta.ObjectList = (*SwitchProfileList)(nil)
)

func (spList *SwitchProfileList) GetItems() []meta.Object {
	items := make([]meta.Object, len(spList.Items))
	for i := range spList.Items {
		items[i] = &spList.Items[i]
	}

	return items
}

func (sp *SwitchProfile) Default() {
	meta.DefaultObjectMetadata(sp)
}

func (sp *SwitchProfile) Validate(ctx context.Context, kube client.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(sp); err != nil {
		return nil, err
	}

	return nil, nil
}
