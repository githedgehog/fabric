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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExternalAttachmentSpec defines the desired state of ExternalAttachment
type ExternalAttachmentSpec struct {
	// External is the name of the External object this attachment belongs to
	External string `json:"external,omitempty"`
	// Connection is the name of the Connection object this attachment belongs to (essentialy the name of the switch/port)
	Connection string `json:"connection,omitempty"`
	// Switch is the switch port configuration for the external attachment
	Switch ExternalAttachmentSwitch `json:"switch,omitempty"`
	// Neighbor is the BGP neighbor configuration for the external attachment
	Neighbor ExternalAttachmentNeighbor `json:"neighbor,omitempty"`
}

// ExternalAttachmentSwitch defines the switch port configuration for the external attachment
type ExternalAttachmentSwitch struct {
	// VLAN (optional) is the VLAN ID used for the subinterface on a switch port specified in the connection, set to 0 if no VLAN is used
	VLAN uint16 `json:"vlan,omitempty"`
	// IP is the IP address of the subinterface on a switch port specified in the connection
	IP string `json:"ip,omitempty"`
}

// ExternalAttachmentNeighbor defines the BGP neighbor configuration for the external attachment
type ExternalAttachmentNeighbor struct {
	// ASN is the ASN of the BGP neighbor
	ASN uint32 `json:"asn,omitempty"`
	// IP is the IP address of the BGP neighbor to peer with
	IP string `json:"ip,omitempty"`
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
// ExternalAttachment is a definition of how specific switch is connected with external system (External object).
// Effectively it represents BGP peering between the switch and external system including all needed configuration.
type ExternalAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the ExternalAttachment
	Spec ExternalAttachmentSpec `json:"spec,omitempty"`
	// Status is the observed state of the ExternalAttachment
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

var (
	_ meta.Object     = (*ExternalAttachment)(nil)
	_ meta.ObjectList = (*ExternalAttachmentList)(nil)
)

func (extAttachList *ExternalAttachmentList) GetItems() []meta.Object {
	items := make([]meta.Object, len(extAttachList.Items))
	for i := range extAttachList.Items {
		items[i] = &extAttachList.Items[i]
	}

	return items
}

func (attach *ExternalAttachment) Default() {
	meta.DefaultObjectMetadata(attach)

	if attach.Labels == nil {
		attach.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(attach.Labels)

	attach.Labels[wiringapi.LabelConnection] = attach.Spec.Connection
	attach.Labels[LabelExternal] = attach.Spec.External
}

func (attach *ExternalAttachment) Validate(ctx context.Context, kube client.Reader, _ *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(attach); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	if attach.Spec.External == "" {
		return nil, errors.Errorf("external is required")
	}
	if attach.Spec.Connection == "" {
		return nil, errors.Errorf("connection is required")
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

	if kube != nil {
		ext := &External{}
		if err := kube.Get(ctx, types.NamespacedName{Name: attach.Spec.External, Namespace: attach.Namespace}, ext); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("external %s not found", attach.Spec.External)
			}

			return nil, errors.Wrapf(err, "failed to read external %s", attach.Spec.External) // TODO replace with some internal error to not expose to the user
		}

		conn := &wiringapi.Connection{}
		if err := kube.Get(ctx, types.NamespacedName{Name: attach.Spec.Connection, Namespace: attach.Namespace}, conn); err != nil {
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
