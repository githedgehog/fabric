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

	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultMasqueradeIdleTimeout  = 2 * time.Minute
	DefaultPortForwardIdleTimeout = 2 * time.Minute
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewayPeeringTopology is where a GatewayPeering sits in the fabric topology
type GatewayPeeringTopology struct {
	// Fabric is the name of the Fabric this GatewayPeering belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
}

// PeeringSpec defines the desired state of Peering.
type PeeringSpec struct {
	// Topology is where the GatewayPeering sits in the fabric topology
	Topology GatewayPeeringTopology `json:"topology,omitempty"`
	// GatewayGroup is the name of the gateway group that should process the peering
	GatewayGroup string `json:"gatewayGroup,omitempty"`
	// Peerings is a map of peering entries for each VPC participating in the peering (keyed by VPC name)
	Peering map[string]*PeeringEntry `json:"peering,omitempty"`
	// ACL is an optional, peering-scoped ACL
	ACL *PeeringACL `json:"acl,omitempty"`
	// NoMultiPath is an optional flag to disable multi-path for stateless peerings (handling by all gateways in a group)
	NoMultiPath bool `json:"noMultiPath,omitempty"`
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

// TCP flags to match. For each flag, true requires the flag to be set, false requires it to be
// clear, omit to accept both
type PeeringACLMatchTCPFlags struct {
	// FIN flag
	FIN *bool `json:"fin,omitempty"`
	// SYN flag
	SYN *bool `json:"syn,omitempty"`
	// RST flag
	RST *bool `json:"rst,omitempty"`
	// PSH flag
	PSH *bool `json:"psh,omitempty"`
	// ACK flag
	ACK *bool `json:"ack,omitempty"`
	// URG flag
	URG *bool `json:"urg,omitempty"`
	// ECE flag
	ECE *bool `json:"ece,omitempty"`
	// CWR flag
	CWR *bool `json:"cwr,omitempty"`
}

// TCP-specific match filters
type PeeringACLMatchTCPFilters struct {
	// TCP flags to match, omit to match all segments
	Flags *PeeringACLMatchTCPFlags `json:"flags,omitempty"`
}

type PeeringACLMatch struct {
	// From-side native addresses and/or source ports
	Source []PeeringACLMatchEndpoint `json:"src,omitempty"`
	// To-side advertised addresses and/or destination ports
	Destination []PeeringACLMatchEndpoint `json:"dst,omitempty"`
	// Protocol to match ("tcp", "udp", or numeric), omit to match any protocol
	Protocol ACLMatchProtocol `json:"proto,omitempty"`
	// TCP-specific filters, only valid with protocol "tcp" and scope "packet"
	TCPFilters *PeeringACLMatchTCPFilters `json:"tcpFilters,omitempty"`
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
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
// +kubebuilder:printcolumn:name="Group",type=string,JSONPath=`.spec.gatewayGroup`,priority=0
// +kubebuilder:printcolumn:name="VPCs",type=string,JSONPath=`.metadata.annotations.gateway\.githedgehog\.com/vpcs`,priority=0
// +kubebuilder:printcolumn:name="NAT",type=string,JSONPath=`.metadata.annotations.gateway\.githedgehog\.com/nat`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
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
	if p.Annotations == nil {
		p.Annotations = map[string]string{}
	}

	if p.Spec.Topology.Fabric == "" {
		p.Spec.Topology.Fabric = wiringapi.DefaultFabric
	}

	wiringapi.CleanupFabricLabels(p.Labels)

	p.Labels[wiringapi.ListLabelFabric(p.Spec.Topology.Fabric)] = ListLabelValue

	vpcs := slices.Sorted(maps.Keys(p.Spec.Peering))
	if len(vpcs) != 2 {
		return
	}

	p.Labels[ListLabelVPC(vpcs[0])] = ListLabelValue
	p.Labels[ListLabelVPC(vpcs[1])] = ListLabelValue
	p.Annotations[LabelVPCs] = strings.Join(vpcs, ",")

	nats := []string{}
	for _, peering := range p.Spec.Peering {
		if peering == nil {
			continue
		}
		for idx := range peering.Expose {
			expose := &peering.Expose[idx]
			nat := expose.NAT
			if nat != nil {
				if nat.Masquerade != nil {
					if nat.Masquerade.IdleTimeout.Duration == 0 {
						nat.Masquerade.IdleTimeout.Duration = DefaultMasqueradeIdleTimeout
					}
					nats = append(nats, "masquerade")
				}

				if nat.PortForward != nil {
					if nat.PortForward.IdleTimeout.Duration == 0 {
						nat.PortForward.IdleTimeout.Duration = DefaultPortForwardIdleTimeout
					}

					nats = append(nats, "port-forward")
				}

				if nat.Static != nil {
					nats = append(nats, "static")
				}
			}
		}
	}
	slices.Sort(nats)
	if len(nats) > 0 {
		p.Annotations[LabelNAT] = strings.Join(slices.Compact(nats), ",")
	} else {
		delete(p.Annotations, LabelNAT)
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
	spec := field.NewPath("spec")
	if fabricCfg != nil && !fabricCfg.EnableGateway {
		return field.Forbidden(spec, "gateway support is not enabled")
	}
	if err := meta.ValidateObjectMetadata(p); err != nil {
		return err //nolint:wrapcheck
	}
	if p.Spec.GatewayGroup == "" {
		return field.Required(spec.Child("gatewayGroup"), "")
	}

	if err := wiringapi.CheckFabricExists(ctx, kube, p.Namespace, p.Spec.Topology.Fabric); err != nil {
		return field.Invalid(spec.Child("topology", "fabric"), p.Spec.Topology.Fabric, err.Error())
	}

	vpcs := slices.Collect(maps.Keys(p.Spec.Peering))
	if len(vpcs) != 2 {
		return field.Invalid(spec.Child("peering"), field.OmitValueType{}, fmt.Sprintf("peering must have exactly 2 VPCs, got %d", len(vpcs)))
	}
	for name, vpc := range p.Spec.Peering {
		if vpc == nil {
			continue
		}
		for exposeIdx, expose := range vpc.Expose {
			exposePath := spec.Child("peering").Key(name).Child("expose").Index(exposeIdx)
			if expose.DefaultDestination && (len(expose.IPs) > 0 || len(expose.As) > 0 || expose.NAT != nil) {
				return field.Invalid(exposePath, field.OmitValueType{}, "default flag should be the only thing set")
			}
			for ipIdx, ip := range expose.IPs {
				ipPath := exposePath.Child("ips").Index(ipIdx)
				nonnil := 0
				if ip.CIDR != "" {
					if _, err := netip.ParsePrefix(ip.CIDR); err != nil {
						return field.Invalid(ipPath.Child("cidr"), ip.CIDR, err.Error())
					}
					nonnil++
				}
				if ip.Not != "" {
					if _, err := netip.ParsePrefix(ip.Not); err != nil {
						return field.Invalid(ipPath.Child("not"), ip.Not, err.Error())
					}
					nonnil++
				}
				if ip.VPCSubnet != "" {
					if extName, isExternal := strings.CutPrefix(name, v1beta1.VPCInfoExtPrefix); isExternal {
						return field.Forbidden(ipPath.Child("vpcSubnet"), fmt.Sprintf("external %s cannot have an IP block with VPC Subnets specified", extName))
					}
					nonnil++
				}
				if nonnil != 1 {
					return field.Invalid(ipPath, field.OmitValueType{}, "exactly one of cidr, not or vpcSubnet must be set")
				}
			}
			for asIdx, as := range expose.As {
				asPath := exposePath.Child("as").Index(asIdx)
				nonnil := 0
				if as.CIDR != "" {
					if _, err := netip.ParsePrefix(as.CIDR); err != nil {
						return field.Invalid(asPath.Child("cidr"), as.CIDR, err.Error())
					}
					nonnil++
				}
				if as.Not != "" {
					if _, err := netip.ParsePrefix(as.Not); err != nil {
						return field.Invalid(asPath.Child("not"), as.Not, err.Error())
					}
					nonnil++
				}
				if nonnil != 1 {
					return field.Invalid(asPath, field.OmitValueType{}, "exactly one of cidr or not must be set")
				}
			}

			if (len(expose.As) == 0) != (expose.NAT == nil) {
				return field.Invalid(exposePath, field.OmitValueType{}, "as and nat must both be set or both be empty")
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

				natPath := exposePath.Child("nat")
				if nonNils != 1 {
					return field.Invalid(natPath, field.OmitValueType{}, "exactly one of masquerade, static, or portForward must be set")
				}

				if expose.NAT.PortForward != nil {
					portsPath := natPath.Child("portForward", "ports")
					if len(expose.NAT.PortForward.Ports) == 0 {
						return field.Required(portsPath, "at least one port forwarding rule must be set")
					}

					for idx, entry := range expose.NAT.PortForward.Ports {
						if err := validatePort(entry.Port); err != nil {
							return field.Invalid(portsPath.Index(idx).Child("port"), entry.Port, err.Error())
						}

						if err := validatePort(entry.As); err != nil {
							return field.Invalid(portsPath.Index(idx).Child("as"), entry.As, err.Error())
						}

						if !slices.Contains(PeeringNATProtocols, entry.Protocol) {
							return field.NotSupported(portsPath.Index(idx).Child("proto"), entry.Protocol, PeeringNATProtocols)
						}
					}
				}
			}
		}
	}

	if acl := p.Spec.ACL; acl != nil {
		aclPath := spec.Child("acl")
		if !slices.Contains(ACLDefaultActions, acl.Default) {
			return field.NotSupported(aclPath.Child("default"), acl.Default, ACLDefaultActions)
		}
		for i, rule := range acl.Rules {
			rulePath := aclPath.Child("rules").Index(i)
			if rule.Name != "" {
				if errs := kvalidation.IsDNS1123Subdomain(rule.Name); len(errs) > 0 {
					return field.Invalid(rulePath.Child("name"), rule.Name, strings.Join(errs, ", "))
				}
				if len(rule.Name) > 64 {
					return field.TooLong(rulePath.Child("name"), rule.Name, 64)
				}
			}
			if !slices.Contains(ACLActions, rule.Action) {
				return field.NotSupported(rulePath.Child("action"), rule.Action, ACLActions)
			}
			if !slices.Contains(ACLScopes, rule.Scope) {
				return field.NotSupported(rulePath.Child("scope"), rule.Scope, ACLScopes)
			}
			if rule.From == "" && rule.To == "" {
				return field.Required(rulePath, "at least one of from and to must be specified")
			}
			if rule.From != "" && !slices.Contains(vpcs, rule.From) {
				return field.NotSupported(rulePath.Child("from"), rule.From, vpcs)
			}
			if rule.To != "" && !slices.Contains(vpcs, rule.To) {
				return field.NotSupported(rulePath.Child("to"), rule.To, vpcs)
			}
			for j, src := range rule.Match.Source {
				srcPath := rulePath.Child("match", "src").Index(j)
				if src.CIDR != "" && src.VPCSubnet != "" {
					return field.Invalid(srcPath, field.OmitValueType{}, "at most one of cidr and vpcSubnet can be specified")
				}
				if src.CIDR != "" {
					if _, err := netip.ParsePrefix(src.CIDR); err != nil {
						return field.Invalid(srcPath.Child("cidr"), src.CIDR, err.Error())
					}
				}
				for k, port := range src.Ports {
					if err := validatePort(port); err != nil {
						return field.Invalid(srcPath.Child("ports").Index(k), port, err.Error())
					}
				}
			}
			for j, dst := range rule.Match.Destination {
				dstPath := rulePath.Child("match", "dst").Index(j)
				if dst.CIDR != "" && dst.VPCSubnet != "" {
					return field.Invalid(dstPath, field.OmitValueType{}, "at most one of cidr and vpcSubnet can be specified")
				}
				if dst.CIDR != "" {
					if _, err := netip.ParsePrefix(dst.CIDR); err != nil {
						return field.Invalid(dstPath.Child("cidr"), dst.CIDR, err.Error())
					}
				}
				for k, port := range dst.Ports {
					if err := validatePort(port); err != nil {
						return field.Invalid(dstPath.Child("ports").Index(k), port, err.Error())
					}
				}
			}
			if !slices.Contains(ACLMatchProtocols, rule.Match.Protocol) {
				if _, err := strconv.Atoi(string(rule.Match.Protocol)); err != nil {
					return field.Invalid(rulePath.Child("match", "proto"), rule.Match.Protocol, fmt.Sprintf("must be one of %q or a protocol number", ACLMatchProtocols))
				}
			}
			// "6" is the protocol number for TCP
			if rule.Match.TCPFilters != nil && rule.Match.Protocol != ACLMatchProtocolTCP && rule.Match.Protocol != "6" {
				return field.Forbidden(rulePath.Child("match", "tcpFilters"), fmt.Sprintf("TCP filters require protocol %q", ACLMatchProtocolTCP))
			}
		}
	}
	if kube != nil {
		gwGroup := &GatewayGroup{}
		if err := kube.Get(ctx, kclient.ObjectKey{Name: p.Spec.GatewayGroup, Namespace: p.Namespace}, gwGroup); err != nil {
			if kapierrors.IsNotFound(err) {
				return field.NotFound(spec.Child("gatewayGroup"), p.Spec.GatewayGroup)
			}

			return fmt.Errorf("failed to get gateway group %s: %w", p.Spec.GatewayGroup, err)
		}

		peeringFabric := p.Spec.Topology.Fabric
		if groupFabric := gwGroup.Spec.Topology.Fabric; groupFabric != peeringFabric {
			return field.Invalid(spec.Child("gatewayGroup"), p.Spec.GatewayGroup, fmt.Sprintf("peering is in fabric %s but the gateway group is in fabric %s", peeringFabric, groupFabric))
		}
		// the gateways handling the peering are reachable only from leaves in their domain
		groupDomain := gwGroup.Spec.Topology.Domain
		// a stored gateway group the refresh on fabric-ctrl initialization had to leave alone may have none, and an
		// external without one too would then look like it's in the same domain
		if groupDomain == "" {
			return field.Invalid(spec.Child("gatewayGroup"), p.Spec.GatewayGroup, "gateway group has no domain")
		}

		if fabricCfg != nil && fabricCfg.ExtraValidators.Peering != nil {
			if err := fabricCfg.ExtraValidators.Peering(ctx, kube, p); err != nil {
				return err //nolint:wrapcheck
			}
		}

		peeringVPCs := make(map[string]*v1beta1.VPC, len(p.Spec.Peering))
		// check that the exposed CIDRs actually belong to the VPCs the peering is for
		for vpcName, peering := range p.Spec.Peering {
			entryPath := spec.Child("peering").Key(vpcName)
			// A GatewayPeering could be with an external too; in this case, the name will start with
			// the VPCInfoExtPrefix prefix (currently "ext.")
			if extName, isExt := strings.CutPrefix(vpcName, v1beta1.VPCInfoExtPrefix); isExt {
				var external v1beta1.External
				if err := kube.Get(ctx, ktypes.NamespacedName{Name: extName, Namespace: kmetav1.NamespaceDefault}, &external); err != nil {
					if kapierrors.IsNotFound(err) {
						return field.Invalid(entryPath, field.OmitValueType{}, fmt.Sprintf("external %s not found", extName))
					}

					return fmt.Errorf("failed to get External %s: %w", extName, err)
				}

				if extFabric := external.Spec.Topology.Fabric; extFabric != peeringFabric {
					return field.Invalid(entryPath, field.OmitValueType{}, fmt.Sprintf("peering is in fabric %s but external %s is in fabric %s", peeringFabric, extName, extFabric))
				}
				if extDomain := external.Spec.Topology.Domain; extDomain != groupDomain {
					return field.Invalid(entryPath, field.OmitValueType{}, fmt.Sprintf("gateway group %s is in domain %s but external %s is in domain %s", p.Spec.GatewayGroup, groupDomain, extName, extDomain))
				}

				// checking whether the prefix is part of the external is possible only if the external
				// is static, as we know exactly which prefixes are reachable in that case. For BGP speaking
				// externals there is no way to know that. For simplicity, I'm just skipping the check for both.
				continue
			}
			var vpc v1beta1.VPC
			if err := kube.Get(ctx, ktypes.NamespacedName{Name: vpcName, Namespace: kmetav1.NamespaceDefault}, &vpc); err != nil {
				if kapierrors.IsNotFound(err) {
					return field.Invalid(entryPath, field.OmitValueType{}, fmt.Sprintf("VPC %s not found", vpcName))
				}

				return fmt.Errorf("failed to get VPC %s: %w", vpcName, err)
			}
			if vpcFabric := vpc.Spec.Topology.Fabric; vpcFabric != peeringFabric {
				return field.Invalid(entryPath, field.OmitValueType{}, fmt.Sprintf("peering is in fabric %s but vpc %s is in fabric %s", peeringFabric, vpcName, vpcFabric))
			}
			if vpcDomains := vpc.Spec.Topology.Domains; !slices.Contains(vpcDomains, groupDomain) {
				return field.Invalid(entryPath, field.OmitValueType{}, fmt.Sprintf("gateway group %s is in domain %s but vpc %s is in domains %v", p.Spec.GatewayGroup, groupDomain, vpcName, vpcDomains))
			}

			peeringVPCs[vpcName] = &vpc
			// only the exposes are optional: a nil entry still names a VPC, which must be in this fabric
			if peering == nil {
				continue
			}
			for exposeIdx, expose := range peering.Expose {
				if expose.DefaultDestination {
					continue
				}
				for ipIdx, ip := range expose.IPs {
					ipPath := entryPath.Child("expose").Index(exposeIdx).Child("ips").Index(ipIdx)
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
							return field.Invalid(ipPath.Child("cidr"), ip.CIDR, "not part of VPC "+vpcName)
						}
					}
					if ip.VPCSubnet != "" {
						if _, ok := vpc.Spec.Subnets[ip.VPCSubnet]; !ok {
							return field.NotFound(ipPath.Child("vpcSubnet"), ip.VPCSubnet)
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

				matchPath := spec.Child("acl", "rules").Index(i).Child("match")
				for j, src := range rule.Match.Source {
					if src.VPCSubnet != "" {
						srcPath := matchPath.Child("src").Index(j).Child("vpcSubnet")
						if peeringVPCs[from] == nil {
							return field.Invalid(srcPath, src.VPCSubnet, fmt.Sprintf("the corresponding peering entry %q is not a VPC", from))
						}
						if _, ok := peeringVPCs[from].Spec.Subnets[src.VPCSubnet]; !ok {
							return field.Invalid(srcPath, src.VPCSubnet, "does not exist in VPC "+from)
						}
					}
				}
				for j, dst := range rule.Match.Destination {
					if dst.VPCSubnet != "" {
						dstPath := matchPath.Child("dst").Index(j).Child("vpcSubnet")
						if peeringVPCs[to] == nil {
							return field.Invalid(dstPath, dst.VPCSubnet, fmt.Sprintf("the corresponding peering entry %q is not a VPC", to))
						}
						if _, ok := peeringVPCs[to].Spec.Subnets[dst.VPCSubnet]; !ok {
							return field.Invalid(dstPath, dst.VPCSubnet, "does not exist in VPC "+to)
						}
					}
				}
			}
		}
	}

	return nil
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
