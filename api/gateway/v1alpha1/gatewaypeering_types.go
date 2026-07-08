// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultMasqueradeIdleTimeout  = 2 * time.Minute
	DefaultPortForwardIdleTimeout = 2 * time.Minute
)

// TODO: deduplicate and expose from fabric meta package
var nameChecker = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PeeringSpec defines the desired state of Peering.
type PeeringSpec struct {
	// GatewayGroup is the name of the gateway group that should process the peering
	GatewayGroup string `json:"gatewayGroup,omitempty"`
	// Peerings is a map of peering entries for each VPC participating in the peering (keyed by VPC name)
	Peering map[string]*PeeringEntry `json:"peering,omitempty"`
	// ACL is an optional, peering-scoped ACL
	ACL *PeeringACL `json:"acl,omitempty"`
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

type ACLMatchProtocol string

// +kubebuilder:validation:Enum=deny;deny-unless-exposed;""
type ACLDefaultAction string

// +kubebuilder:validation:Enum=deny;allow
type ACLAction string

// +kubebuilder:validation:Enum=flow;packet;""
type ACLScope string

const (
	ACLMatchProtocolTCP         ACLMatchProtocol = "tcp"
	ACLMatchProtocolUDP         ACLMatchProtocol = "udp"
	ACLMatchProtocolAny         ACLMatchProtocol = ""
	ACLDefaultDenyUnlessExposed ACLDefaultAction = "deny-unless-exposed"
	ACLDefaultDeny              ACLDefaultAction = "deny"
	ACLActionDeny               ACLAction        = "deny"
	ACLActionAllow              ACLAction        = "allow"
	ACLScopeFlow                ACLScope         = "flow"
	ACLScopePacket              ACLScope         = "packet"
)

var ACLMatchProtocols = []ACLMatchProtocol{
	ACLMatchProtocolTCP,
	ACLMatchProtocolUDP,
	ACLMatchProtocolAny,
}

var ACLDefaultActions = []ACLDefaultAction{
	ACLDefaultDeny,
	ACLDefaultDenyUnlessExposed,
}

var ACLActions = []ACLAction{
	ACLActionAllow,
	ACLActionDeny,
}

var ACLScopes = []ACLScope{
	ACLScopeFlow,
	ACLScopePacket,
}

type PeeringACLMatchEndpoint struct {
	// CIDR to match, at most one of cidr and vpcSubnet can be set
	CIDR string `json:"cidr,omitempty"`
	// VPC subnet to match, at most one of cidr and vpcSubnet can be set
	VPCSubnet string `json:"vpcSubnet,omitempty"`
	// List of ports or port ranges to match, omit to match all ports
	Ports []string `json:"ports,omitempty"`
}

type PeeringACLMatch struct {
	// From-side native addresses and/or source ports
	Source []PeeringACLMatchEndpoint `json:"src,omitempty"`
	// To-side advertised addresses and/or destination ports
	Destination []PeeringACLMatchEndpoint `json:"dst,omitempty"`
	// Protocol to match ("tcp", "udp", or numeric), omit to match any protocol
	Protocol ACLMatchProtocol `json:"proto,omitempty"`
}

type PeeringACLRule struct {
	// Optional name for logs and diagnostics
	Name string `json:"name,omitempty"`
	// From has to match one of the peering's two VPCs if present, implicit from the "to" field otherwise
	From string `json:"from,omitempty"`
	// To has to match one of the peering's two VPCs if present, implicit from the "from" field otherwise
	To string `json:"to,omitempty"`
	// Action to execute if the rule matches, can be either "deny" or "allow"
	Action ACLAction `json:"action"`
	// What the rule should match against, omit to match everything in the rule's direction
	Match PeeringACLMatch `json:"match,omitempty"`
	// Scope of the rule, can be either "flow" (default if empty) or "packet"
	Scope ACLScope `json:"scope,omitempty"`
	Log   bool     `json:"log,omitempty"`
}

type PeeringACL struct {
	// Default action to execute if no rules matches, can be either "deny-unless-exposed" (default if empty) or "deny"
	Default ACLDefaultAction `json:"default"`
	// List of rules for this particular ACL
	Rules []PeeringACLRule `json:"rules,omitempty"`
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
// +kubebuilder:resource:categories=hedgehog;hedgehog-gateway,shortName=gwpeer
// +kubebuilder:printcolumn:name="GatewayGroup",type=string,JSONPath=`.spec.gatewayGroup`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// GatewayPeering is the Schema for the peerings API.
type GatewayPeering struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PeeringSpec   `json:"spec,omitempty"`
	Status PeeringStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayPeeringList contains a list of Peering.
type GatewayPeeringList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []GatewayPeering `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &GatewayPeering{}, &GatewayPeeringList{})

		return nil
	})
}

func (p *GatewayPeering) Default() {
	if p.Namespace == "" {
		p.Namespace = kmetav1.NamespaceDefault
	}
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

	if acl := p.Spec.ACL; acl != nil {
		if acl.Default == "" {
			acl.Default = ACLDefaultDenyUnlessExposed
		}
		for i := range acl.Rules {
			if acl.Rules[i].Scope == "" {
				acl.Rules[i].Scope = ACLScopeFlow
			}
		}
	}
}

func (p *GatewayPeering) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) error {
	if fabricCfg != nil && !fabricCfg.EnableGateway {
		return fmt.Errorf("gateway support is not enabled") //nolint:err113
	}
	if p.Namespace != kmetav1.NamespaceDefault {
		return fmt.Errorf("gatewaypeering namespace must be %s", kmetav1.NamespaceDefault) //nolint:err113
	}
	if p.Spec.GatewayGroup == "" {
		return fmt.Errorf("gateway group must be specified %s", p.Name) //nolint:err113
	}

	vpcs := slices.Collect(maps.Keys(p.Spec.Peering))
	if len(vpcs) != 2 {
		return fmt.Errorf("peering must have exactly 2 VPCs, got %d", len(vpcs)) //nolint:err113
	}
	// track the NAT on each side of the peering and disallow unsupported configurations
	vpcNAT := make(map[string]struct {
		Stateful  bool
		Stateless bool
	}, 2)
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
					if extName, isExternal := strings.CutPrefix(name, v1beta1.VPCInfoExtPrefix); isExternal {
						return fmt.Errorf("external %s cannot have an IP block with VPC Subnets specified", extName) //nolint:err113
					}
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
					vpcEntry := vpcNAT[name]
					vpcEntry.Stateless = true
					vpcNAT[name] = vpcEntry
				}
				if expose.NAT.Masquerade != nil {
					nonNils++
					vpcEntry := vpcNAT[name]
					vpcEntry.Stateful = true
					vpcNAT[name] = vpcEntry
				}
				if expose.NAT.PortForward != nil {
					nonNils++
					vpcEntry := vpcNAT[name]
					vpcEntry.Stateful = true
					vpcNAT[name] = vpcEntry
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
	if vpcNAT[vpcs[0]].Stateful && vpcNAT[vpcs[1]].Stateful {
		return fmt.Errorf("unsupported configuration, only one side of a peering can use stateful NAT (i.e. masquerade or portForward)") //nolint:err113
	}
	if (vpcNAT[vpcs[0]].Stateless && vpcNAT[vpcs[1]].Stateful) || (vpcNAT[vpcs[1]].Stateless && vpcNAT[vpcs[0]].Stateful) {
		return fmt.Errorf("unsupported configuration, one side of a peering using static NAT cannot peer with a side using stateful NAT") //nolint:err113
	}

	if acl := p.Spec.ACL; acl != nil {
		if !slices.Contains(ACLDefaultActions, acl.Default) {
			return fmt.Errorf("invalid default action %q in ACL", acl.Default) //nolint:err113
		}
		for i, rule := range acl.Rules {
			ruleBlob := ""
			if rule.Name != "" {
				if !nameChecker.MatchString(rule.Name) {
					return fmt.Errorf("invalid rule name %q in ACL rule %d", rule.Name, i) //nolint:err113
				}
				if len(rule.Name) > 64 {
					return fmt.Errorf("rule name %q in ACL rule %d is too long, must be 64 characters or less", rule.Name, i) //nolint:err113
				}
				ruleBlob = fmt.Sprintf(" (%s)", rule.Name)
			}
			if !slices.Contains(ACLActions, rule.Action) {
				return fmt.Errorf("invalid action %q in ACL rule %d%s", rule.Action, i, ruleBlob) //nolint:err113
			}
			if rule.From == "" && rule.To == "" {
				return fmt.Errorf("at least one of from and to must be specified in ACL rule %d%s", i, ruleBlob) //nolint:err113
			}
			if rule.From != "" && !slices.Contains(vpcs, rule.From) {
				return fmt.Errorf("invalid from %q in ACL rule %d%s, it has to match one of the two VPCs of the peering: %q", rule.From, i, ruleBlob, vpcs) //nolint:err113
			}
			if rule.To != "" && !slices.Contains(vpcs, rule.To) {
				return fmt.Errorf("invalid to %q in ACL rule %d%s, it has to match one of the two VPCs of the peering: %q", rule.To, i, ruleBlob, vpcs) //nolint:err113
			}
			for _, src := range rule.Match.Source {
				if src.CIDR != "" && src.VPCSubnet != "" {
					return fmt.Errorf("at most one of cidr and vpcSubnet can be specified in source match for rule %d%s", i, ruleBlob) //nolint:err113
				}
				if src.CIDR != "" {
					if _, err := netip.ParsePrefix(src.CIDR); err != nil {
						return fmt.Errorf("invalid source CIDR %q in ACL rule %d%s: %w", src.CIDR, i, ruleBlob, err)
					}
				}
				for _, port := range src.Ports {
					if err := validatePort(port); err != nil {
						return fmt.Errorf("invalid source port %q in ACL rule %d%s: %w", port, i, ruleBlob, err)
					}
				}
			}
			for _, dst := range rule.Match.Destination {
				if dst.CIDR != "" && dst.VPCSubnet != "" {
					return fmt.Errorf("at most one of cidr and vpcSubnet can be specified in destination match for rule %d%s", i, ruleBlob) //nolint:err113
				}
				if dst.CIDR != "" {
					if _, err := netip.ParsePrefix(dst.CIDR); err != nil {
						return fmt.Errorf("invalid destination CIDR %q in ACL rule %d%s: %w", dst.CIDR, i, ruleBlob, err)
					}
				}
				for _, port := range dst.Ports {
					if err := validatePort(port); err != nil {
						return fmt.Errorf("invalid destination port %q in ACL rule %d%s: %w", port, i, ruleBlob, err)
					}
				}
			}
			if !slices.Contains(ACLMatchProtocols, rule.Match.Protocol) {
				if _, err := strconv.Atoi(string(rule.Match.Protocol)); err != nil {
					return fmt.Errorf("invalid protocol %q in ACL rule %d%s: %w", rule.Match.Protocol, i, ruleBlob, err)
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
		peeringVPCs := make(map[string]*v1beta1.VPC, len(p.Spec.Peering))
		for originVPC, ourEntry := range p.Spec.Peering {
			ourCIDRs := []string{}
			existingCIDRs := []string{}
			var targetVPC string
			for vpc := range maps.Keys(p.Spec.Peering) {
				if vpc == originVPC {
					continue
				}
				targetVPC = vpc
			}

			ourCIDRs = collectExposedCIDRs(ourEntry, ourCIDRs)
			if len(ourCIDRs) == 0 {
				continue
			}
			peeringList := &GatewayPeeringList{}
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

		// check that the exposed CIDRs actually belong to the VPCs the peering is for
		for vpcName, peering := range p.Spec.Peering {
			if peering == nil {
				continue
			}
			// A GatewayPeering could be with an external too; in this case, the name will start with
			// the VPCInfoExtPrefix prefix (currently "ext.")
			if extName, isExt := strings.CutPrefix(vpcName, v1beta1.VPCInfoExtPrefix); isExt {
				var external v1beta1.External
				if err := kube.Get(ctx, ktypes.NamespacedName{Name: extName, Namespace: kmetav1.NamespaceDefault}, &external); err != nil {
					if kapierrors.IsNotFound(err) {
						return fmt.Errorf("external %s not found", extName) //nolint:err113
					}

					return fmt.Errorf("failed to get External %s: %w", extName, err)
				}

				// checking whether the prefix is part of the external is possible only if the external
				// is static, as we know exactly which prefixes are reachable in that case. For BGP speaking
				// externals there is no way to know that. For simplicity, I'm just skipping the check for both.
				continue
			}
			var vpc v1beta1.VPC
			if err := kube.Get(ctx, ktypes.NamespacedName{Name: vpcName, Namespace: kmetav1.NamespaceDefault}, &vpc); err != nil {
				if kapierrors.IsNotFound(err) {
					return fmt.Errorf("VPC %s not found", vpcName) //nolint:err113
				}

				return fmt.Errorf("failed to get VPC %s: %w", vpcName, err)
			}
			peeringVPCs[vpcName] = &vpc
			for _, expose := range peering.Expose {
				if expose.DefaultDestination {
					continue
				}
				for _, ip := range expose.IPs {
					if ip.CIDR != "" {
						exposePrefix, err := netip.ParsePrefix(ip.CIDR)
						if err != nil {
							return fmt.Errorf("failed to parse prefix of exposed CIDR %s for VPC %s: %w", ip.CIDR, vpcName, err)
						}
						found := false
						for subnetName, subnet := range vpc.Spec.Subnets {
							subnetPrefix, err := netip.ParsePrefix(subnet.Subnet)
							if err != nil {
								return fmt.Errorf("failed to parse prefix of subnet %s of VPC %s: %w", subnetName, vpcName, err)
							}
							if iputil.IsSubset(exposePrefix, subnetPrefix) {
								found = true

								break
							}
						}
						if !found {
							return fmt.Errorf("CIDR %s is not part of VPC %s", ip.CIDR, vpcName) //nolint:err113
						}
					}
					if ip.VPCSubnet != "" {
						if _, ok := vpc.Spec.Subnets[ip.VPCSubnet]; !ok {
							return fmt.Errorf("VPC subnet %s referenced in peering expose does not exist in VPC %s", ip.VPCSubnet, vpcName) //nolint:err113
						}
					}
				}
			}
		}
		if acl := p.Spec.ACL; acl != nil {
			for i, rule := range acl.Rules {
				from := rule.From
				to := rule.To
				if from == "" {
					if vpcs[0] == to {
						from = vpcs[1]
					} else {
						from = vpcs[0]
					}
				} else if to == "" {
					if vpcs[0] == from {
						to = vpcs[1]
					} else {
						to = vpcs[0]
					}
				}

				for _, src := range rule.Match.Source {
					if src.VPCSubnet != "" {
						if peeringVPCs[from] == nil {
							return fmt.Errorf("source VPC subnet %s referenced in ACL rule %d but the corresponding peering entry %q is not a VPC", src.VPCSubnet, i, from) //nolint:err113
						}
						if _, ok := peeringVPCs[from].Spec.Subnets[src.VPCSubnet]; !ok {
							return fmt.Errorf("source VPC subnet %s referenced in ACL rule %d does not exist in VPC %s", src.VPCSubnet, i, from) //nolint:err113
						}
					}
				}
				for _, dst := range rule.Match.Destination {
					if dst.VPCSubnet != "" {
						if peeringVPCs[to] == nil {
							return fmt.Errorf("destination VPC subnet %s referenced in ACL rule %d but the corresponding peering entry %q is not a VPC", dst.VPCSubnet, i, to) //nolint:err113
						}
						if _, ok := peeringVPCs[to].Spec.Subnets[dst.VPCSubnet]; !ok {
							return fmt.Errorf("destination VPC subnet %s referenced in ACL rule %d does not exist in VPC %s", dst.VPCSubnet, i, to) //nolint:err113
						}
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
