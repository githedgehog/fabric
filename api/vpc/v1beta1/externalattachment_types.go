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
	"net"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExternalAttachmentSpec defines the desired state of ExternalAttachment
type ExternalAttachmentSpec struct {
	// External is the name of the External object this attachment belongs to
	External string `json:"external,omitempty"`
	// Connection is the name of the Connection object this attachment belongs to (essentially the name of the switch/port)
	Connection string `json:"connection,omitempty"`
	// Switch is the switch port configuration for the external attachment in case of a BGP external
	Switch ExternalAttachmentSwitch `json:"switch"`
	// Neighbor is the BGP neighbor configuration for the external attachment in case of a BGP external
	Neighbor ExternalAttachmentNeighbor `json:"neighbor"`
	// L2 contains parameters specific to an L2 external attachment
	// +optional
	L2 *ExternalAttachmentL2 `json:"l2,omitempty"`
}

// ExternalAttachmentSwitch defines the switch port configuration for the external attachment
type ExternalAttachmentSwitch struct {
	// VLAN (optional) is the VLAN ID used for the subinterface on a switch port specified in the connection, set to 0 if no VLAN is used
	VLAN uint16 `json:"vlan,omitempty"`
	// IP is the IP address of the subinterface on a switch port specified in the connection, it should include the prefix length
	IP string `json:"ip,omitempty"`
}

// ExternalAttachmentNeighbor defines the BGP neighbor configuration for the external attachment
type ExternalAttachmentNeighbor struct {
	// ASN is the ASN of the BGP neighbor
	ASN uint32 `json:"asn,omitempty"`
	// IP is the IP address of the BGP neighbor to peer with (without prefix length)
	IP string `json:"ip,omitempty"`
}

// ExternalAttachmentL2 defines parameters used for L2 external attachments
type ExternalAttachmentL2 struct {
	// IP is the actual IP address of the external, which will be used as nexthop for prefixes reachable via this external attachment
	IP string `json:"ip"`
	// VLAN (optional) is the VLAN ID used for the subinterface on a switch port specified in the connection, set to 0 if no VLAN is required
	VLAN uint16 `json:"vlan,omitempty"`
	// GatewayIPs is the list of IP addresses (with prefix length) which can be used for NAT on the fabric side for this L2 external attachment
	GatewayIPs []string `json:"gatewayIPs"`
	// FabricEdgeIP is an IP address (with prefix length) that will be configured on the fabric edge switch; it is needed for proxy-ARP
	FabricEdgeIP string `json:"fabricEdgeIP"`
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
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the ExternalAttachment
	Spec ExternalAttachmentSpec `json:"spec"`
	// Status is the observed state of the ExternalAttachment
	Status ExternalAttachmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ExternalAttachmentList contains a list of ExternalAttachment
type ExternalAttachmentList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []ExternalAttachment `json:"items"`
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

func (attach *ExternalAttachment) Validate(ctx context.Context, kube kclient.Reader, _ *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(attach); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	if attach.Spec.External == "" {
		return nil, errors.Errorf("external is required")
	}
	if attach.Spec.Connection == "" {
		return nil, errors.Errorf("connection is required")
	}
	if attach.Spec.L2 == nil {
		if attach.Spec.Switch.IP == "" {
			return nil, errors.Errorf("switch.ip is required")
		}
		if _, _, err := net.ParseCIDR(attach.Spec.Switch.IP); err != nil {
			return nil, errors.New("switch.ip is not a valid IP CIDR") //nolint: goerr113
		}
		if attach.Spec.Neighbor.ASN == 0 {
			return nil, errors.Errorf("neighbor.asn is required")
		}
		if attach.Spec.Neighbor.IP == "" {
			return nil, errors.Errorf("neighbor.ip is required")
		}
		if ip := net.ParseIP(attach.Spec.Neighbor.IP); ip == nil {
			return nil, errors.New("neighbor.ip is not a valid IP address") //nolint: goerr113
		}
	} else {
		if attach.Spec.Switch.IP != "" || attach.Spec.Switch.VLAN != 0 {
			return nil, errors.Errorf("switch parameters must not be set for L2 external attachment")
		}
		if attach.Spec.Neighbor.ASN != 0 || attach.Spec.Neighbor.IP != "" {
			return nil, errors.Errorf("neighbor parameters must not be set for L2 external attachment")
		}
		if attach.Spec.L2.IP == "" {
			return nil, errors.Errorf("l2.ip is required for L2 external attachment")
		}
		if ip := net.ParseIP(attach.Spec.L2.IP); ip == nil {
			return nil, errors.New("l2.ip is not a valid IP address") //nolint: goerr113
		}
		if len(attach.Spec.L2.GatewayIPs) == 0 {
			return nil, errors.Errorf("at least one l2.gatewayIPs is required for L2 external attachment")
		}
		for _, cidr := range attach.Spec.L2.GatewayIPs {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return nil, errors.Errorf("l2.gatewayIPs contains an invalid address (with prefix length): %s", cidr) //nolint: goerr113
			}
		}
		// TODO: make this optional and auto-assign from a pool?
		if attach.Spec.L2.FabricEdgeIP == "" {
			return nil, errors.Errorf("l2.fabricEdgeIP is required for L2 external attachment")
		}
		if _, _, err := net.ParseCIDR(attach.Spec.L2.FabricEdgeIP); err != nil {
			return nil, errors.Wrapf(err, "l2.fabricEdgeIP is not a valid IP address with prefix length")
		}
	}

	if kube != nil {
		ext := &External{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: attach.Spec.External, Namespace: attach.Namespace}, ext); err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, errors.Errorf("external %s not found", attach.Spec.External)
			}

			return nil, errors.Wrapf(err, "failed to read external %s", attach.Spec.External) // TODO replace with some internal error to not expose to the user
		}
		if attach.Spec.L2 != nil && ext.Spec.L2 == nil {
			return nil, errors.Errorf("external attachment is L2 but external %s is not", attach.Spec.External)
		}
		if attach.Spec.L2 == nil && ext.Spec.L2 != nil {
			return nil, errors.Errorf("external attachment is not L2 but external %s is", attach.Spec.External)
		}

		conn := &wiringapi.Connection{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: attach.Spec.Connection, Namespace: attach.Namespace}, conn); err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, errors.Errorf("connection %s not found", attach.Spec.Connection)
			}

			return nil, errors.Wrapf(err, "failed to read connection %s", attach.Spec.Connection) // TODO replace with some internal error to not expose to the user
		}

		if conn.Spec.External == nil {
			return nil, errors.Errorf("connection %s is not external", attach.Spec.Connection)
		}

		// validate VLAN collision
		attaches := &ExternalAttachmentList{}
		if err := kube.List(ctx, attaches, kclient.MatchingLabels{wiringapi.LabelName("connection"): attach.Spec.Connection}); err != nil {
			return nil, errors.Wrapf(err, "failed to list external attachments for %s", attach.Spec.Connection) // TODO replace with some internal error to not expose to the user
		}
		ourVLAN := attach.Spec.Switch.VLAN
		if attach.Spec.L2 != nil {
			ourVLAN = attach.Spec.L2.VLAN
		}
		for _, other := range attaches.Items {
			if other.Name == attach.Name {
				continue
			}
			otherVLAN := other.Spec.Switch.VLAN
			if other.Spec.L2 != nil {
				otherVLAN = other.Spec.L2.VLAN
			}
			if otherVLAN == ourVLAN {
				return nil, errors.Errorf("connection %s already has an external attachment with VLAN %d", attach.Spec.Connection, ourVLAN)
			}
		}
	}

	return nil, nil
}
