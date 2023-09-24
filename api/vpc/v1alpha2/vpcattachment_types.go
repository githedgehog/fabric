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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VPCAttachmentSpec defines the desired state of VPCAttachment
type VPCAttachmentSpec struct {
	VPC        string `json:"vpc,omitempty"`
	Connection string `json:"connection,omitempty"`
}

// VPCAttachmentStatus defines the observed state of VPCAttachment
type VPCAttachmentStatus struct {
	// Ready bool `json:"ready,omitempty"` // TODO
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// VPCAttachment is the Schema for the vpcattachments API
type VPCAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCAttachmentSpec   `json:"spec,omitempty"`
	Status VPCAttachmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VPCAttachmentList contains a list of VPCAttachment
type VPCAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPCAttachment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPCAttachment{}, &VPCAttachmentList{})
}

func (s *VPCAttachmentSpec) Labels() map[string]string {
	return map[string]string{
		LabelVPC:                  s.VPC,
		wiringapi.LabelConnection: s.Connection,
	}
}

func (attach *VPCAttachment) Default() {
	if attach.Labels == nil {
		attach.Labels = map[string]string{}
	}

	maps.Copy(attach.Labels, attach.Spec.Labels())
}

func (attach *VPCAttachment) Validate() (warnings admission.Warnings, err error) {
	// TODO check vpc and connection exist

	return nil, nil
}
