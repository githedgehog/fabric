// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/netip"
	"slices"
	"strconv"
	"strings"
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
	// GatewayGroup is the name of the gateway group that should process the peering
	GatewayGroup string `json:"gatewayGroup,omitempty"`
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
	// CIDR to include, only one of cidr, not, vpcSubnet can be set
	CIDR string `json:"cidr,omitempty"`
	// CIDR to exclude, only one of cidr, not, vpcSubnet can be set
	Not string `json:"not,omitempty"`
	// CIDR by VPC subnet name to include, only one of cidr, not, vpcSubnet can be set
	VPCSubnet string `json:"vpcSubnet,omitempty"`
	// Port ranges (e.g. "80, 443, 3000-3100"), used together with exactly one of cidr, not, vpcSubnet
	Ports string `json:"ports,omitempty"`
}

type PeeringEntryAs struct {
	// CIDR to include, only one of cidr, not can be set
	CIDR string `json:"cidr,omitempty"`
	// CIDR to exclude, only one of cidr, not can be set
	Not string `json:"not,omitempty"`
	// Port ranges (e.g. "80, 443, 3000-3100"), used together with exactly one of cidr, not
	Ports string `json:"ports,omitempty"`
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
// +kubebuilder:printcolumn:name="GatewayGroup",type=string,JSONPath=`.spec.gatewayGroup`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
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

	if p.Spec.GatewayGroup == "" {
		p.Spec.GatewayGroup = DefaultGatewayGroup
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

func (p *Peering) Validate(ctx context.Context, kube kclient.Reader) error {
	if p.Spec.GatewayGroup == "" {
		return fmt.Errorf("gateway group must be specified %s", p.Name) //nolint:goerr113
	}

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

				if err := validatePorts(ip.Ports); err != nil {
					return fmt.Errorf("invalid ports %s in peering expose IPs of VPC %s: %w", ip.Ports, name, err)
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

				if err := validatePorts(as.Ports); err != nil {
					return fmt.Errorf("invalid ports %s in peering expose AS of VPC %s: %w", as.Ports, name, err)
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

	if kube != nil {
		gwGroup := &GatewayGroup{}
		if err := kube.Get(ctx, kclient.ObjectKey{Name: p.Spec.GatewayGroup, Namespace: p.Namespace}, gwGroup); err != nil {
			// TODO enable validation back after it's supplied by the fabricator
			slog.Warn("Failed to get Gateway group", "name", p.Spec.GatewayGroup, "error", err.Error())
			// if kapierrors.IsNotFound(err) {
			// 	return fmt.Errorf("gateway group %s not found", p.Spec.GatewayGroup) //nolint:err113
			// }

			// return fmt.Errorf("failed to get gateway group %s: %w", p.Spec.GatewayGroup, err)
		}
	}

	return nil
}

func validatePorts(in string) error {
	if in == "" {
		return nil
	}

	// TODO probably normalize in Default() and check for overlapping ranges/duplicates

	for ports := range strings.SplitSeq(in, ",") {
		ports = strings.TrimSpace(ports)

		switch {
		case ports == "":
			return fmt.Errorf("port entry should not be empty") //nolint:err113
		case !strings.Contains(ports, "-"):
			if port, err := strconv.Atoi(ports); err != nil {
				return fmt.Errorf("invalid port %s: %w", ports, err)
			} else if port < 1 || port > 65535 {
				return fmt.Errorf("invalid port %d: port should be between 1 and 65535", port) //nolint:err113
			}
		default:
			parts := strings.Split(ports, "-")
			if len(parts) != 2 {
				return fmt.Errorf("invalid port range %s: should be in format start-end", ports) //nolint:err113
			}

			parts[0] = strings.TrimSpace(parts[0])
			parts[1] = strings.TrimSpace(parts[1])
			if parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("invalid port range %s: both start and end should not be empty", ports) //nolint:err113
			}

			start, err := strconv.Atoi(parts[0])
			if err != nil {
				return fmt.Errorf("invalid start port %s: %w", parts[0], err)
			} else if start < 1 || start > 65535 {
				return fmt.Errorf("invalid start port %d: port should be between 1 and 65535", start) //nolint:err113
			}

			end, err := strconv.Atoi(parts[1])
			if err != nil {
				return fmt.Errorf("invalid end port %s: %w", parts[1], err)
			} else if end < 1 || end > 65535 {
				return fmt.Errorf("invalid end port %d: port should be between 1 and 65535", end) //nolint:err113
			}

			if start > end {
				return fmt.Errorf("invalid port range %s: start port %d is greater than end port %d", ports, start, end) //nolint:err113
			}
		}
	}

	return nil
}
