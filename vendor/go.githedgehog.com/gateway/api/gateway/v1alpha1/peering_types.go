// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PeeringSpec defines the desired state of Peering.
type PeeringSpec struct {
	// Peerings is a map of peering entries for each VPC participating in the peering (keyed by VPC name)
	Peering map[string]*PeeringEntry `json:"peering,omitempty"`
}

type PeeringEntryExpose struct {
	IPs []PeeringEntryIP `json:"ips,omitempty"`
	As  []PeeringEntryAs `json:"as,omitempty"`
}

type PeeringEntry struct {
	Expose  []PeeringEntryExpose  `json:"expose,omitempty"`
	Ingress []PeeringEntryIngress `json:"ingress,omitempty"`
	// TODO add natType: stateful # as there are not enough IPs in the "as" pool
	// TODO add metric: 0 # add 0 to the advertised route metrics
}

type PeeringEntryIP struct {
	CIDR      string `json:"cidr,omitempty"`
	Not       string `json:"not,omitempty"`
	VPCSubnet string `json:"vpcSubnet,omitempty"`
}

type PeeringEntryAs struct {
	CIDR string `json:"cidr,omitempty"`
	Not  string `json:"not,omitempty"`
}

type PeeringEntryIngress struct {
	Allow *PeeringEntryIngressAllow `json:"allow,omitempty"`
	// TODO add deny?
}

type PeeringEntryIngressAllow struct {
	// TODO add actual fields
	// stateless: true
	// 	 tcp:
	//     srcPort: 443
}

// PeeringStatus defines the observed state of Peering.
type PeeringStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;hedgehog-gateway,shortName=peer
// Peering is the Schema for the peerings API.
type Peering struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PeeringSpec   `json:"spec,omitempty"`
	Status PeeringStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PeeringList contains a list of Peering.
type PeeringList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []Peering `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Peering{}, &PeeringList{})
}

func (p *Peering) Default() {
	// TODO add defaulting logic
}

func (p *Peering) Validate(_ context.Context, _ kclient.Reader) error {
	// TODO add validation logic
	return nil
}
