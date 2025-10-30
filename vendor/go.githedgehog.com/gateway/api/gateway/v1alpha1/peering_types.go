// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"time"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultStatefulIdleTimeout = 2 * time.Minute
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PeeringSpec defines the desired state of Peering.
type PeeringSpec struct {
	// Peerings is a map of peering entries for each VPC participating in the peering (keyed by VPC name)
	Peering map[string]*PeeringEntry `json:"peering,omitempty"`
}

type PeeringStatefulNAT struct {
	// Time since the last packet after which flows are removed from the connection state table
	IdleTimeout kmetav1.Duration `json:"idleTimeout,omitempty"`
}

type PeeringStatelessNAT struct{}

type PeeringNAT struct {
	// Use connection state tracking when performing NAT
	Stateful *PeeringStatefulNAT `json:"stateful,omitempty"`
	// Use connection state tracking when performing NAT, use stateful NAT if omitted
	Stateless *PeeringStatelessNAT `json:"stateless,omitempty"`
}

type PeeringEntryExpose struct {
	IPs []PeeringEntryIP `json:"ips,omitempty"`
	As  []PeeringEntryAs `json:"as,omitempty"`
	NAT *PeeringNAT      `json:"nat,omitempty"`
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

	for _, peering := range p.Spec.Peering {
		// This will modify p.Spec.Peering because peering is *Peering
		for i := range peering.Expose {
			peering.Expose[i].Default()
		}
	}
}

func (e *PeeringEntryExpose) Default() {
	if len(e.As) != 0 {
		if e.NAT == nil {
			e.NAT = &PeeringNAT{}
		}
		e.NAT.Default()
	}
}

func (n *PeeringNAT) Default() {
	if n.Stateful == nil && n.Stateless == nil {
		n.Stateless = &PeeringStatelessNAT{}
	}

	if n.Stateful != nil {
		n.Stateful.Default()
	}
}

func (s *PeeringStatefulNAT) Default() {
	if s.IdleTimeout.Duration == 0 {
		s.IdleTimeout.Duration = DefaultStatefulIdleTimeout
	}
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
			if len(expose.IPs) == 0 {
				return fmt.Errorf("at least one IP block must be specified in peering expose of VPC %s", name) //nolint:goerr113
			}
			for _, ip := range expose.IPs {
				nonnil := 0
				if ip.CIDR != "" {
					if _, err := netip.ParsePrefix(ip.CIDR); err != nil {
						return fmt.Errorf("invalid CIDR %s in peering expose IPs of VPC %s: %w", ip.CIDR, name, err)
					}
					nonnil++
				}
				if ip.Not != "" {
					if _, err := netip.ParsePrefix(ip.Not); err != nil {
						return fmt.Errorf("invalid Not CIDR %s in peering expose IPs of VPC %s: %w", ip.Not, name, err)
					}
					nonnil++
				}
				if ip.VPCSubnet != "" {
					nonnil++
				}
				if nonnil != 1 {
					return fmt.Errorf("exactly one of cidr, not or vpcSubnet must be set in peering expose IPs of VPC %s", name) //nolint:goerr113
				}
			}
			for _, as := range expose.As {
				nonnil := 0
				if as.CIDR != "" {
					if _, err := netip.ParsePrefix(as.CIDR); err != nil {
						return fmt.Errorf("invalid CIDR %s in peering expose AS of VPC %s: %w", as.CIDR, name, err)
					}
					nonnil++
				}
				if as.Not != "" {
					if _, err := netip.ParsePrefix(as.Not); err != nil {
						return fmt.Errorf("invalid Not CIDR %s in peering expose AS of VPC %s: %w", as.Not, name, err)
					}
					nonnil++
				}
				if nonnil != 1 {
					return fmt.Errorf("exactly one of cidr or not must be set in peering expose AS of VPC %s", name) //nolint:goerr113
				}
			}

			if (len(expose.As) == 0) != (expose.NAT == nil) {
				return fmt.Errorf("expose.As and expose.NAT must both be set or both be empty in peering expose of VPC %s", name) //nolint:goerr113
			}

			if expose.NAT != nil {
				nonnil := 0
				if expose.NAT.Stateless != nil {
					// TODO(mvachhar) validate that stateless NAT has the same number of IPs in the ips and as blocks
					nonnil++
				}

				if expose.NAT.Stateful != nil {
					nonnil++
				}

				if nonnil == 0 {
					return fmt.Errorf("expose.NAT must have at least one of stateful or stateless set in peering expose of VPC %s", name) //nolint:goerr113
				}

				if nonnil > 1 {
					return fmt.Errorf("only one of statefulNat or statelessNat can be set in peering expose of VPC %s", name) //nolint:goerr113
				}
			}
		}
	}

	return nil
}
