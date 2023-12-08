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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExternalAttachmentSpec defines the desired state of ExternalAttachment
type ExternalAttachmentSpec struct {
	External   string                     `json:"external,omitempty"`
	Connection string                     `json:"connection,omitempty"`
	Switch     ExternalAttachmentSwitch   `json:"switch,omitempty"`
	Neighbor   ExternalAttachmentNeighbor `json:"neighbor,omitempty"`
}

type ExternalAttachmentSwitch struct {
	VLAN uint16 `json:"vlan,omitempty"`
	IP   string `json:"ip,omitempty"`
}

type ExternalAttachmentNeighbor struct {
	ASN uint32 `json:"asn,omitempty"`
	IP  string `json:"ip,omitempty"`
}

// ExternalAttachmentStatus defines the observed state of ExternalAttachment
type ExternalAttachmentStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric;external,shortName=extattach
// +kubebuilder:printcolumn:name="External",type=string,JSONPath=`.spec.external`,priority=0
// +kubebuilder:printcolumn:name="Connection",type=string,JSONPath=`.spec.connection`,priority=0
// +kubebuilder:printcolumn:name="SwVLAN",type=string,JSONPath=`.spec.switch.vlan`,priority=1
// +kubebuilder:printcolumn:name="SwIP",type=string,JSONPath=`.spec.switch.ip`,priority=1
// +kubebuilder:printcolumn:name="NeighASN",type=string,JSONPath=`.spec.neighbor.asn`,priority=1
// +kubebuilder:printcolumn:name="NeighIP",type=string,JSONPath=`.spec.neighbor.ip`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// ExternalAttachment is the Schema for the externalattachments API
type ExternalAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalAttachmentSpec   `json:"spec,omitempty"`
	Status ExternalAttachmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ExternalAttachmentList contains a list of ExternalAttachment
type ExternalAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalAttachment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalAttachment{}, &ExternalAttachmentList{})
}

func (attach *ExternalAttachment) Default() {
	if attach.Labels == nil {
		attach.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(attach.Labels)

	attach.Labels[wiringapi.LabelConnection] = attach.Spec.Connection
	attach.Labels[LabelExternal] = attach.Spec.External
}

func (attach *ExternalAttachment) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	if attach.Spec.External == "" {
		return nil, errors.Errorf("external is required")
	}
	if attach.Spec.Connection == "" {
		return nil, errors.Errorf("connection is required")
	}
	if attach.Spec.Switch.VLAN == 0 {
		return nil, errors.Errorf("switch.vlan is required")
	}
	if attach.Spec.Switch.IP == "" {
		return nil, errors.Errorf("switch.ip is required")
	}
	if attach.Spec.Neighbor.ASN == 0 {
		return nil, errors.Errorf("neighbor.asn is required")
	}
	if attach.Spec.Neighbor.IP == "" {
		return nil, errors.Errorf("neighbor.ip is required")
	}

	if client != nil {
		ext := &External{}
		if err := client.Get(ctx, types.NamespacedName{Name: attach.Spec.External, Namespace: attach.Namespace}, ext); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("external %s not found", attach.Spec.External)
			}

			return nil, errors.Wrapf(err, "failed to read external %s", attach.Spec.External) // TODO replace with some internal error to not expose to the user
		}

		conn := &wiringapi.Connection{}
		if err := client.Get(ctx, types.NamespacedName{Name: attach.Spec.Connection, Namespace: attach.Namespace}, conn); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("connection %s not found", attach.Spec.Connection)
			}

			return nil, errors.Wrapf(err, "failed to read connection %s", attach.Spec.Connection) // TODO replace with some internal error to not expose to the user
		}

		if conn.Spec.External == nil {
			return nil, errors.Errorf("connection %s is not external", attach.Spec.Connection)
		}

		// TODO validate IPs/ASNs/VLANs
	}

	return nil, nil
}
