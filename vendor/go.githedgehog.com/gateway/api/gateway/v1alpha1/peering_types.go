// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultMasqueradeIdleTimeout  = 2 * time.Minute
	DefaultPortForwardIdleTimeout = 2 * time.Minute
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PeeringSpec defines the desired state of Peering.
type PeeringSpec struct {
	// GatewayGroup is the name of the gateway group that should process the peering
	GatewayGroup string `json:"gatewayGroup,omitempty"`
	// Peerings is a map of peering entries for each VPC participating in the peering (keyed by VPC name)
	Peering map[string]*PeeringEntry `json:"peering,omitempty"`
}

type PeeringNATMasquerade struct {
	// Time since the last packet after which flows are removed from the connection state table
	IdleTimeout kmetav1.Duration `json:"idleTimeout,omitempty"`
}

// +kubebuilder:validation:Enum=tcp;udp;""
type PeeringNATProtocol string

const (
	// Any protocol by default
	PeeringNATProtocolAny PeeringNATProtocol = ""
	// TCP only
	PeeringNATProtocolTCP PeeringNATProtocol = "tcp"
	// UDP only
	PeeringNATProtocolUDP PeeringNATProtocol = "udp"
)

var PeeringNATProtocols = []PeeringNATProtocol{
	PeeringNATProtocolAny,
	PeeringNATProtocolTCP,
	PeeringNATProtocolUDP,
}

type PeeringNATPortForwardEntry struct {
	Protocol PeeringNATProtocol `json:"proto,omitempty"`
	Port     string             `json:"port,omitempty"`
	As       string             `json:"as,omitempty"`
}

type PeeringNATPortForward struct {
	// Time since the last packet after which flows are removed from the connection state table
	IdleTimeout kmetav1.Duration             `json:"idleTimeout,omitempty"`
	Ports       []PeeringNATPortForwardEntry `json:"ports,omitempty"`
}

type PeeringNATStatic struct{}

type PeeringNAT struct {
	Masquerade  *PeeringNATMasquerade  `json:"masquerade,omitempty"`
	PortForward *PeeringNATPortForward `json:"portForward,omitempty"`
	Static      *PeeringNATStatic      `json:"static,omitempty"`
}

type PeeringEntryExpose struct {
	IPs                []PeeringEntryIP `json:"ips,omitempty"`
	As                 []PeeringEntryAs `json:"as,omitempty"`
	NAT                *PeeringNAT      `json:"nat,omitempty"`
	DefaultDestination bool             `json:"default,omitempty"`
}

type PeeringEntry struct {
	Expose []PeeringEntryExpose `json:"expose,omitempty"`
}

type PeeringEntryIP struct {
	// CIDR to include, only one of cidr, not, vpcSubnet can be set
	CIDR string `json:"cidr,omitempty"`
	// CIDR to exclude, only one of cidr, not, vpcSubnet can be set
	Not string `json:"not,omitempty"`
	// CIDR by VPC subnet name to include, only one of cidr, not, vpcSubnet can be set
	VPCSubnet string `json:"vpcSubnet,omitempty"`
}

type PeeringEntryAs struct {
	// CIDR to include, only one of cidr, not can be set
	CIDR string `json:"cidr,omitempty"`
	// CIDR to exclude, only one of cidr, not can be set
	Not string `json:"not,omitempty"`
}

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
		for idx := range peering.Expose {
			expose := &peering.Expose[idx]
			nat := expose.NAT
			if nat != nil {
				if nat.Masquerade != nil {
					if nat.Masquerade.IdleTimeout.Duration == 0 {
						nat.Masquerade.IdleTimeout.Duration = DefaultMasqueradeIdleTimeout
					}
				}

				if nat.PortForward != nil {
					if nat.PortForward.IdleTimeout.Duration == 0 {
						nat.PortForward.IdleTimeout.Duration = DefaultPortForwardIdleTimeout
					}
				}
			}
		}
	}

	if p.Spec.GatewayGroup == "" {
		p.Spec.GatewayGroup = DefaultGatewayGroup
	}
}

func (p *Peering) Validate(ctx context.Context, kube kclient.Reader) error {
	if p.Spec.GatewayGroup == "" {
		return fmt.Errorf("gateway group must be specified %s", p.Name) //nolint:err113
	}

	vpcs := slices.Collect(maps.Keys(p.Spec.Peering))
	if len(vpcs) != 2 {
		return fmt.Errorf("peering must have exactly 2 VPCs, got %d", len(vpcs)) //nolint:err113
	}
	for name, vpc := range p.Spec.Peering {
		if vpc == nil {
			continue
		}
		for _, expose := range vpc.Expose {
			if expose.DefaultDestination && (len(expose.IPs) > 0 || len(expose.As) > 0 || expose.NAT != nil) {
				return fmt.Errorf("default flag should be the only thing set in expose of VPC %s", name) //nolint:err113
			}
			if len(expose.IPs) == 0 && !expose.DefaultDestination {
				return fmt.Errorf("at least one IP block must be specified in peering expose of VPC %s", name) //nolint:err113
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
					return fmt.Errorf("exactly one of cidr, not or vpcSubnet must be set in peering expose IPs of VPC %s", name) //nolint:err113
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
					return fmt.Errorf("exactly one of cidr or not must be set in peering expose AS of VPC %s", name) //nolint:err113
				}
			}

			if (len(expose.As) == 0) != (expose.NAT == nil) {
				return fmt.Errorf("expose.As and expose.NAT must both be set or both be empty in peering expose of VPC %s", name) //nolint:err113
			}

			if expose.NAT != nil {
				nonNils := 0
				if expose.NAT.Static != nil {
					nonNils++
				}
				if expose.NAT.Masquerade != nil {
					nonNils++
				}
				if expose.NAT.PortForward != nil {
					nonNils++
				}

				if nonNils != 1 {
					return fmt.Errorf("exactly one of masquerade, static, or portForward must be set in NAT section for peering expose of VPC %s", name) //nolint:err113
				}

				if expose.NAT.PortForward != nil {
					if len(expose.NAT.PortForward.Ports) == 0 {
						return fmt.Errorf("at least one port forwarding rule must be set in NAT section for peering expose of VPC %s", name) //nolint:err113
					}

					for idx, entry := range expose.NAT.PortForward.Ports {
						if err := validatePort(entry.Port); err != nil {
							return fmt.Errorf("invalid port %q in port forwarding rule %d in NAT section for peering expose of VPC %s: %w", entry.Port, idx, name, err)
						}

						if err := validatePort(entry.As); err != nil {
							return fmt.Errorf("invalid as %q in port forwarding rule %d in NAT section for peering expose of VPC %s: %w", entry.As, idx, name, err)
						}

						if !slices.Contains(PeeringNATProtocols, entry.Protocol) {
							return fmt.Errorf("invalid protocol %q in port forwarding rule %d in NAT section for peering expose of VPC %s", entry.Protocol, idx, name) //nolint:err113
						}
					}
				}
			}
		}
	}

	if kube != nil {
		gwGroup := &GatewayGroup{}
		if err := kube.Get(ctx, kclient.ObjectKey{Name: p.Spec.GatewayGroup, Namespace: p.Namespace}, gwGroup); err != nil {
			if kapierrors.IsNotFound(err) {
				return fmt.Errorf("gateway group %s not found", p.Spec.GatewayGroup) //nolint:err113
			}

			return fmt.Errorf("failed to get gateway group %s: %w", p.Spec.GatewayGroup, err)
		}
		// check for overlaps of exposed IPs towards either of the VPCs in the peering we are validating
		peeringVPCs := maps.Keys(p.Spec.Peering)
		for originVPC, ourEntry := range p.Spec.Peering {
			ourCIDRs := []string{}
			existingCIDRs := []string{}
			var targetVPC string
			for vpc := range peeringVPCs {
				if vpc == originVPC {
					continue
				}
				targetVPC = vpc
			}

			ourCIDRs = collectExposedCIDRs(ourEntry, ourCIDRs)
			if len(ourCIDRs) == 0 {
				continue
			}
			peeringList := &PeeringList{}
			if err := kube.List(ctx, peeringList, kclient.MatchingLabels{ListLabelVPC(targetVPC): ListLabelValue}); err != nil {
				return fmt.Errorf("failed to list peerings for VPC %s: %w", targetVPC, err)
			}
			for _, other := range peeringList.Items {
				if other.Name == p.Name {
					continue
				}
				for otherOriginVPC, otherEntry := range other.Spec.Peering {
					if otherOriginVPC == targetVPC {
						continue
					}
					existingCIDRs = collectExposedCIDRs(otherEntry, existingCIDRs)
				}
			}
			if len(existingCIDRs) == 0 {
				continue
			}
			for _, ourCIDR := range ourCIDRs {
				ourP, err := netip.ParsePrefix(ourCIDR)
				if err != nil {
					return fmt.Errorf("failed to parse exposed CIDR %s: %w", ourCIDR, err)
				}
				for _, otherCIDR := range existingCIDRs {
					otherP, err := netip.ParsePrefix(otherCIDR)
					if err != nil {
						return fmt.Errorf("failed to parse existing exposed CIDR %s: %w", otherCIDR, err)
					}
					if ourP.Overlaps(otherP) {
						return fmt.Errorf("overlap between existing exposed CIDR %s and new exposed CIDR %s", otherCIDR, ourCIDR) //nolint:err113
					}
				}
			}
		}
	}

	return nil
}

func collectExposedCIDRs(entry *PeeringEntry, cidrs []string) []string {
	for _, expose := range entry.Expose {
		if expose.DefaultDestination {
			continue
		}
		if len(expose.As) == 0 {
			for _, ip := range expose.IPs {
				// TODO: account for NOTs?
				cidrs = append(cidrs, ip.CIDR)
			}
		} else {
			for _, as := range expose.As {
				// TODO: account for NOTs?
				cidrs = append(cidrs, as.CIDR)
			}
		}
	}

	return cidrs
}

func validatePort(in string) error {
	if strings.TrimSpace(in) != in {
		return fmt.Errorf("invalid port %q: should not contain leading or trailing whitespace", in) //nolint:err113
	}

	if strings.Contains(in, ",") {
		return fmt.Errorf("invalid port %q: should be a single port or range", in) //nolint:err113
	}

	switch {
	case in == "":
		return fmt.Errorf("port entry should not be empty") //nolint:err113
	case !strings.Contains(in, "-"):
		if port, err := strconv.Atoi(in); err != nil {
			return fmt.Errorf("invalid port %q: %w", in, err)
		} else if port < 1 || port > 65535 {
			return fmt.Errorf("invalid port %d: port should be between 1 and 65535", port) //nolint:err113
		}
	default:
		parts := strings.Split(in, "-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid port range %s: should be in format start-end", in) //nolint:err113
		}

		parts[0] = strings.TrimSpace(parts[0])
		parts[1] = strings.TrimSpace(parts[1])
		if parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid port range %s: both start and end should not be empty", in) //nolint:err113
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
			return fmt.Errorf("invalid port range %s: start port %d is greater than end port %d", in, start, end) //nolint:err113
		}
	}

	return nil
}
