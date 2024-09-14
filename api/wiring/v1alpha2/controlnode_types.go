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

// ControlNodeSpec defines configuration for the ControlNode
type ControlNodeSpec struct {
	TargetDevice string `json:"targetDevice,omitempty"` // TODO need to support some soft raid?

	MgmtIface string `json:"mgmtIface,omitempty"` // TODO need to support bond?
	MgmtIP    string `json:"mgmtIP,omitempty"`

	ExtIface string `json:"extIface,omitempty"` // TODO need to support bond?
	ExtIP    string `json:"extIP,omitempty"`    // TODO accept DHCP as well, installer should check the ip on the interface and add to the tls-san
}

// ControlNodeStatus defines the observed state of ControlNode
type ControlNodeStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ControlNode is the Schema for the controlnodes API
type ControlNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ControlNodeSpec   `json:"spec,omitempty"`
	Status ControlNodeStatus `json:"status,omitempty"`
}

const KindControlNode = "ControlNode"

// +kubebuilder:object:root=true

// ControlNodeList contains a list of ControlNode
type ControlNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ControlNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ControlNode{}, &ControlNodeList{})
}

var (
	_ meta.Object     = (*ControlNode)(nil)
	_ meta.ObjectList = (*ControlNodeList)(nil)
)

func (controlList *ControlNodeList) GetItems() []meta.Object {
	items := make([]meta.Object, len(controlList.Items))
	for i := range controlList.Items {
		items[i] = &controlList.Items[i]
	}

	return items
}

func (control *ControlNode) Default() {
	// TODO
}

func (control *ControlNode) Validate(_ context.Context, _ client.Reader, _ *meta.FabricConfig) (admission.Warnings, error) {
	// TODO

	return nil, nil
}
