// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"slices"

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
	Expose []PeeringEntryExpose `json:"expose,omitempty"`
	// Ingress []PeeringEntryIngress `json:"ingress,omitempty"`
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

// type PeeringEntryIngress struct {
// Allow *PeeringEntryIngressAllow `json:"allow,omitempty"`
// TODO add deny?
// }

// type PeeringEntryIngressAllow struct {
// TODO add actual fields
// stateless: true
// 	 tcp:
//     srcPort: 443
// }

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
	if p.Labels == nil {
		p.Labels = map[string]string{}
	}

	vpcs := slices.Collect(maps.Keys(p.Spec.Peering))
	if len(vpcs) != 2 {
		return
	}

	p.Labels[ListLabelVPC(vpcs[0])] = ListLabelValue
	p.Labels[ListLabelVPC(vpcs[1])] = ListLabelValue
}

func (p *Peering) Validate(_ context.Context, _ kclient.Reader) error {
	vpcs := slices.Collect(maps.Keys(p.Spec.Peering))
	if len(vpcs) != 2 {
		return fmt.Errorf("peering must have exactly 2 VPCs, got %d", len(vpcs)) //nolint:goerr113
	}
	for name, vpc := range p.Spec.Peering {
		if vpc == nil {
			continue
		}
		for _, expose := range vpc.Expose {
			for _, ip := range expose.IPs {
				if _, err := netip.ParsePrefix(ip.CIDR); err != nil {
					return fmt.Errorf("invalid CIDR %s in peering expose IPs of VPC %s: %w", ip.CIDR, name, err)
				}
				if ip.Not != "" {
					if _, err := netip.ParsePrefix(ip.Not); err != nil {
						return fmt.Errorf("invalid Not CIDR %s in peering expose IPs of VPC %s: %w", ip.Not, name, err)
					}
				}
			}
			for _, as := range expose.As {
				if _, err := netip.ParsePrefix(as.CIDR); err != nil {
					return fmt.Errorf("invalid CIDR %s in peering expose AS of VPC %s: %w", as.CIDR, name, err)
				}
				if as.Not != "" {
					if _, err := netip.ParsePrefix(as.Not); err != nil {
						return fmt.Errorf("invalid Not CIDR %s in peering expose AS of VPC %s: %w", as.Not, name, err)
					}
				}
			}
		}
	}

	return nil
}
