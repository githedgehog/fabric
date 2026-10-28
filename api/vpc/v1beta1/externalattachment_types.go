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
	"fmt"
	"net"
	"net/netip"
	"slices"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type (
	ACLProtocol string
	ACLAction   string
)

const (
	ACLProtocolIP    ACLProtocol = "ip"
	ACLProtocolTCP   ACLProtocol = "tcp"
	ACLProtocolUDP   ACLProtocol = "udp"
	ACLProtocolICMP  ACLProtocol = "icmp"
	ACLAny                       = "any"
	ACLActionPermit  ACLAction   = "permit"
	ACLActionDeny    ACLAction   = "deny"
	ACLActionDiscard ACLAction   = "discard"
	ACLActionTransit ACLAction   = "transit"
	// ACLUserMinSeq is the minimum sequence number allowed for user-defined ACL rules.
	// Sequence numbers below this are reserved for hardcoded security rules.
	ACLUserMinSeq = 10
	// BFD interval and detection multiplier bounds accepted by FRR
	BFDMinIntervalMS = 10
	BFDMaxIntervalMS = 60000
	BFDMinMultiplier = 2
)

var ACLProtocols = []ACLProtocol{
	ACLProtocolIP,
	ACLProtocolTCP,
	ACLProtocolUDP,
	ACLProtocolICMP,
}

var ACLActions = []ACLAction{
	ACLActionDeny,
	ACLActionDiscard,
	ACLActionPermit,
	ACLActionTransit,
}

type ACLTCPFilters struct {
	Established bool `json:"established,omitempty"`
	Fin         bool `json:"fin,omitempty"`
	NotFin      bool `json:"notFin,omitempty"`
	Syn         bool `json:"syn,omitempty"`
	NotSyn      bool `json:"notSyn,omitempty"`
	Rst         bool `json:"rst,omitempty"`
	NotRst      bool `json:"notRst,omitempty"`
	Psh         bool `json:"psh,omitempty"`
	NotPsh      bool `json:"notPsh,omitempty"`
	Ack         bool `json:"ack,omitempty"`
	NotAck      bool `json:"notAck,omitempty"`
	Urg         bool `json:"urg,omitempty"`
	NotUrg      bool `json:"notUrg,omitempty"`
}

type ACLStatement struct {
	Seq            uint16         `json:"seq"`
	Action         ACLAction      `json:"action"`
	Protocol       ACLProtocol    `json:"protocol"`
	SrcPrefix      string         `json:"srcPrefix"`
	DstPrefix      string         `json:"dstPrefix"`
	PortRangeBegin uint16         `json:"portRangeBegin,omitempty"`
	PortRangeEnd   uint16         `json:"portRangeEnd,omitempty"`
	TCPFilters     *ACLTCPFilters `json:"tcpFilters,omitempty"`
	ICMPType       *uint8         `json:"icmpType,omitempty"`
	ICMPCode       *uint8         `json:"icmpCode,omitempty"`
}

// ACLSpec defines an IPv4 Access Control List applied to inbound traffic on an external attachment.
type ACLSpec struct {
	Statements []ACLStatement `json:"statements,omitempty"`
}

func (spec *ACLSpec) Validate(path *field.Path) (admission.Warnings, error) {
	if spec == nil {
		return nil, nil
	}

	seqNos := map[uint16]bool{}
	for idx, stmt := range spec.Statements {
		stmtPath := path.Child("statements").Index(idx)
		if stmt.Seq < ACLUserMinSeq {
			return nil, field.Invalid(stmtPath.Child("seq"), stmt.Seq, fmt.Sprintf("reserved for security rules (user-defined rules must use seq >= %d)", ACLUserMinSeq))
		}
		if seqNos[stmt.Seq] {
			return nil, field.Duplicate(stmtPath.Child("seq"), stmt.Seq)
		}
		seqNos[stmt.Seq] = true

		if !slices.Contains(ACLActions, stmt.Action) {
			return nil, field.NotSupported(stmtPath.Child("action"), stmt.Action, ACLActions)
		}

		if !slices.Contains(ACLProtocols, stmt.Protocol) {
			return nil, field.NotSupported(stmtPath.Child("protocol"), stmt.Protocol, ACLProtocols)
		}

		if stmt.SrcPrefix == "" {
			return nil, field.Required(stmtPath.Child("srcPrefix"), "")
		}
		if stmt.SrcPrefix != ACLAny {
			if _, _, err := net.ParseCIDR(stmt.SrcPrefix); err != nil {
				return nil, field.Invalid(stmtPath.Child("srcPrefix"), stmt.SrcPrefix, err.Error())
			}
		}
		if stmt.DstPrefix == "" {
			return nil, field.Required(stmtPath.Child("dstPrefix"), "")
		}
		if stmt.DstPrefix != ACLAny {
			if _, _, err := net.ParseCIDR(stmt.DstPrefix); err != nil {
				return nil, field.Invalid(stmtPath.Child("dstPrefix"), stmt.DstPrefix, err.Error())
			}
		}

		if stmt.TCPFilters != nil {
			tcpPath := stmtPath.Child("tcpFilters")
			if stmt.Protocol != ACLProtocolTCP {
				return nil, field.Forbidden(tcpPath, "can only be used with tcp protocol")
			}
			if stmt.TCPFilters.Established && (stmt.TCPFilters.Ack || stmt.TCPFilters.NotAck ||
				stmt.TCPFilters.Fin || stmt.TCPFilters.NotFin ||
				stmt.TCPFilters.Syn || stmt.TCPFilters.NotSyn ||
				stmt.TCPFilters.Rst || stmt.TCPFilters.NotRst ||
				stmt.TCPFilters.Psh || stmt.TCPFilters.NotPsh ||
				stmt.TCPFilters.Urg || stmt.TCPFilters.NotUrg) {
				return nil, field.Invalid(tcpPath, field.OmitValueType{}, "established is mutually exclusive with individual TCP flags")
			}
			if (stmt.TCPFilters.Fin && stmt.TCPFilters.NotFin) ||
				(stmt.TCPFilters.Syn && stmt.TCPFilters.NotSyn) ||
				(stmt.TCPFilters.Rst && stmt.TCPFilters.NotRst) ||
				(stmt.TCPFilters.Psh && stmt.TCPFilters.NotPsh) ||
				(stmt.TCPFilters.Ack && stmt.TCPFilters.NotAck) ||
				(stmt.TCPFilters.Urg && stmt.TCPFilters.NotUrg) {
				return nil, field.Invalid(tcpPath, field.OmitValueType{}, "contradictory TCP flags (flag and its negation)")
			}
		}
		if (stmt.ICMPType != nil || stmt.ICMPCode != nil) && stmt.Protocol != ACLProtocolICMP {
			return nil, field.Forbidden(stmtPath, "icmpType/icmpCode can only be used with icmp protocol")
		}
		if stmt.PortRangeEnd > 0 || stmt.PortRangeBegin > 0 {
			if stmt.Protocol != ACLProtocolTCP && stmt.Protocol != ACLProtocolUDP {
				return nil, field.Forbidden(stmtPath, "portRange can only be used with tcp or udp protocol")
			}
			if stmt.PortRangeBegin == 0 {
				return nil, field.Invalid(stmtPath.Child("portRangeBegin"), stmt.PortRangeBegin, "must be >= 1")
			}
			if stmt.PortRangeBegin > stmt.PortRangeEnd {
				return nil, field.Invalid(stmtPath.Child("portRangeBegin"), stmt.PortRangeBegin, "must be <= portRangeEnd")
			}
		}
	}

	return nil, nil
}

// ExternalAttachmentTopology is where a ExternalAttachment sits in the fabric topology
type ExternalAttachmentTopology struct {
	// Fabric is the name of the Fabric this ExternalAttachment belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
}

// ExternalAttachmentSpec defines the desired state of ExternalAttachment
type ExternalAttachmentSpec struct {
	// Topology is where the ExternalAttachment sits in the fabric topology
	Topology ExternalAttachmentTopology `json:"topology,omitempty"`
	// External is the name of the External object this attachment belongs to
	External string `json:"external,omitempty"`
	// Connection is the name of the Connection object this attachment belongs to (essentially the name of the switch/port)
	Connection string `json:"connection,omitempty"`
	// Switch is the switch port configuration for the external attachment in case of a BGP external
	Switch ExternalAttachmentSwitch `json:"switch"`
	// Neighbor is the BGP neighbor configuration for the external attachment in case of a BGP external
	Neighbor ExternalAttachmentNeighbor `json:"neighbor"`
	// Static contains parameters specific to a static external attachment
	// +optional
	Static *ExternalAttachmentStatic `json:"static,omitempty"`
	// InboundACL defines the ACL statements to apply to inbound traffic on this external attachment
	// +optional
	InboundACL *ACLSpec `json:"inboundACL,omitempty"`
	// BFD (optional) enables BFD for the BGP session of this external attachment, an empty object
	// uses the fabric defaults
	// +optional
	BFD *ExternalAttachmentBFD `json:"bfd,omitempty"`
}

// ExternalAttachmentBFD configures BFD for the BGP session of an external attachment.
// Unset values fall back to the fabric defaults.
type ExternalAttachmentBFD struct {
	// MinRX is the minimum interval in ms at which we accept BFD control packets from the peer (default 300)
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=60000
	// +optional
	MinRX uint32 `json:"minRX,omitempty"`
	// MinTX is the desired interval in ms at which we send BFD control packets (default 300)
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=60000
	// +optional
	MinTX uint32 `json:"minTX,omitempty"`
	// Multiplier is how many missed packets bring the session down (default 3)
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=255
	// +optional
	Multiplier uint8 `json:"multiplier,omitempty"`
	// Passive makes us wait for the peer to initiate the session instead of initiating it
	// ourselves, for the rare ISP that requires it. Both ends passive means no session at all.
	// +optional
	Passive bool `json:"passive,omitempty"`
}

// ExternalAttachmentSwitch defines the switch port configuration for the external attachment
type ExternalAttachmentSwitch struct {
	// VLAN (optional) is the VLAN ID used for the subinterface on a switch port specified in the connection, set to 0 if no VLAN is used
	VLAN uint16 `json:"vlan,omitempty"`
	// IP is the IP address of the subinterface on a switch port specified in the connection, it should include the prefix length.
	// Leave it and neighbor.ip empty for a BGP unnumbered session over IPv6 link-local addresses
	IP string `json:"ip,omitempty"`
}

// ExternalAttachmentNeighbor defines the BGP neighbor configuration for the external attachment
type ExternalAttachmentNeighbor struct {
	// ASN (optional) is the ASN of the BGP neighbor, if not set any ASN other than the switch's own is accepted
	ASN uint32 `json:"asn,omitempty"`
	// IP is the IP address of the BGP neighbor to peer with (without prefix length).
	// Leave it and switch.ip empty for a BGP unnumbered session over IPv6 link-local addresses
	IP string `json:"ip,omitempty"`
	// PrefixLimit (optional) limits the prefixes accepted from the neighbor
	// +optional
	PrefixLimit *ExternalAttachmentPrefixLimit `json:"prefixLimit,omitempty"`
}

// ExternalAttachmentPrefixLimit limits the prefixes accepted from a BGP neighbor, counted after the inbound filters
type ExternalAttachmentPrefixLimit struct {
	// Max is the most prefixes accepted. Above it the session is closed, unless warningOnly is set
	// +kubebuilder:validation:Minimum=1
	Max uint32 `json:"max"`
	// RestartMinutes (optional) is how long the session stays closed after going above max, 5 by default.
	// Raising max doesn't open the session earlier, removing the limit does. Can't be set with warningOnly
	// +kubebuilder:validation:Minimum=1
	// +optional
	RestartMinutes uint16 `json:"restartMinutes,omitempty"`
	// WarningThresholdPct (optional) is the percentage of max from which the switch logs a warning, 75 by default
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	WarningThresholdPct uint8 `json:"warningThresholdPct,omitempty"`
	// WarningOnly (optional) keeps the session open above max, so that going above it is only logged
	// +optional
	WarningOnly bool `json:"warningOnly,omitempty"`
}

// ExternalAttachmentStatic defines parameters used for staticexternal attachments
type ExternalAttachmentStatic struct {
	// RemoteIP is the IP address of the external, which will be used as nexthop for prefixes reachable via this external attachment
	RemoteIP string `json:"remoteIP"`
	// VLAN (optional) is the VLAN ID used for the subinterface on a switch port specified in the connection, set to 0 if no VLAN is required
	VLAN uint16 `json:"vlan,omitempty"`
	// IP is the IP address (with prefix length) to be configured on the switch when not using Proxy mode
	IP string `json:"ip,omitempty"`
	// Proxy is a flag to enable proxy-ARP, used in conjunction with Gateway peering
	Proxy bool `json:"proxy,omitempty"`
}

// ExternalAttachmentStatus defines the observed state of ExternalAttachment
type ExternalAttachmentStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric;external,shortName=extattach
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
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

// +kubebuilder:object:root=true

// ExternalAttachmentList contains a list of ExternalAttachment
type ExternalAttachmentList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []ExternalAttachment `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &ExternalAttachment{}, &ExternalAttachmentList{})

		return nil
	})
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

	if attach.Spec.Topology.Fabric == "" {
		attach.Spec.Topology.Fabric = wiringapi.DefaultFabric
	}

	if attach.Labels == nil {
		attach.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(attach.Labels)

	attach.Labels[wiringapi.LabelConnection] = attach.Spec.Connection
	attach.Labels[LabelExternal] = attach.Spec.External
	attach.Labels[wiringapi.ListLabelFabric(attach.Spec.Topology.Fabric)] = ListLabelValue
}

// asnCollision returns what asn collides with in a fabric, if anything
func asnCollision(fabric *wiringapi.FabricSpec, asn uint32) string {
	if asn >= fabric.LeafASNStart && asn <= fabric.LeafASNEnd {
		return fmt.Sprintf("within the leaf ASN range %d-%d", fabric.LeafASNStart, fabric.LeafASNEnd)
	}
	for name, domain := range fabric.Domains {
		if asn == domain.SpineASN || asn == domain.GatewayASN {
			return fmt.Sprintf("the spine or gateway ASN of domain %s", name)
		}
	}

	return ""
}

func (attach *ExternalAttachment) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	var warns admission.Warnings
	if err := meta.ValidateObjectMetadata(attach); err != nil {
		return nil, err //nolint:wrapcheck
	}

	spec := field.NewPath("spec")
	extPath, connPath := spec.Child("external"), spec.Child("connection")
	neighborPath, staticPath := spec.Child("neighbor"), spec.Child("static")

	if err := wiringapi.CheckFabricExists(ctx, kube, attach.Namespace, attach.Spec.Topology.Fabric); err != nil {
		return nil, err //nolint:wrapcheck
	}

	if attach.Spec.External == "" {
		return nil, field.Required(extPath, "")
	}
	if attach.Spec.Connection == "" {
		return nil, field.Required(connPath, "")
	}
	if attach.Spec.Static == nil {
		// no IPs on either side means BGP unnumbered
		if (attach.Spec.Switch.IP == "") != (attach.Spec.Neighbor.IP == "") {
			return nil, field.Invalid(spec, field.OmitValueType{}, "switch.ip and neighbor.ip must be either both set or both empty (unnumbered)")
		}
		if attach.Spec.Switch.IP != "" {
			if _, _, err := net.ParseCIDR(attach.Spec.Switch.IP); err != nil {
				return nil, field.Invalid(spec.Child("switch", "ip"), attach.Spec.Switch.IP, "not a valid IP CIDR")
			}
			if ip := net.ParseIP(attach.Spec.Neighbor.IP); ip == nil {
				return nil, field.Invalid(neighborPath.Child("ip"), attach.Spec.Neighbor.IP, "not a valid IP address")
			}
		}
		if limit := attach.Spec.Neighbor.PrefixLimit; limit != nil {
			limitPath := neighborPath.Child("prefixLimit")
			if limit.Max == 0 {
				return nil, field.Invalid(limitPath.Child("max"), limit.Max, "must be at least 1")
			}
			if limit.WarningThresholdPct > 100 {
				return nil, field.Invalid(limitPath.Child("warningThresholdPct"), limit.WarningThresholdPct, "must be at most 100")
			}
			// the switch refuses a restart timer on a session it never closes
			if limit.WarningOnly && limit.RestartMinutes != 0 {
				return nil, field.Forbidden(limitPath.Child("restartMinutes"), "must not be set with warningOnly")
			}
		}

		// the border leaf drops external routes carrying its fabric's spine or gateway ASN, and a
		// leaf drops those carrying its own ASN through BGP loop detection. Without an ASN the
		// session accepts any external one, so there is nothing to check.
		if attach.Spec.Neighbor.ASN != 0 && fabricCfg != nil && kube != nil {
			fabric, err := wiringapi.GetFabricSpec(ctx, kube, attach.Namespace, attach.Spec.Topology.Fabric)
			if err != nil {
				return nil, fmt.Errorf("failed to get fabric: %w", err)
			}
			if what := asnCollision(fabric, attach.Spec.Neighbor.ASN); what != "" {
				return nil, field.Invalid(neighborPath.Child("asn"), attach.Spec.Neighbor.ASN, fmt.Sprintf("is %s of its own fabric", what))
			}
		}
		// the same happens in another fabric only if the two exchange routes through externals
		if attach.Spec.Neighbor.ASN != 0 && kube != nil {
			fabrics := &wiringapi.FabricList{}
			if err := kube.List(ctx, fabrics, kclient.InNamespace(attach.Namespace)); err != nil {
				return nil, fmt.Errorf("failed to list fabrics: %w", err) // TODO hide internal error
			}
			for _, fabric := range fabrics.Items {
				if fabric.Name == attach.Spec.Topology.Fabric {
					continue
				}
				if what := asnCollision(&fabric.Spec, attach.Spec.Neighbor.ASN); what != "" {
					warns = append(warns, fmt.Sprintf("neighbor.asn %d is %s of fabric %s, which will drop its routes if they reach it", attach.Spec.Neighbor.ASN, what, fabric.Name))
				}
			}
		}
	} else {
		if attach.Spec.Switch.IP != "" || attach.Spec.Switch.VLAN != 0 {
			return nil, field.Forbidden(spec.Child("switch"), "must not be set for static external attachment")
		}
		if attach.Spec.Neighbor.ASN != 0 || attach.Spec.Neighbor.IP != "" || attach.Spec.Neighbor.PrefixLimit != nil {
			return nil, field.Forbidden(neighborPath, "must not be set for static external attachment")
		}
		if attach.Spec.Static.RemoteIP == "" {
			return nil, field.Required(staticPath.Child("remoteIP"), "")
		}
		_, err := netip.ParseAddr(attach.Spec.Static.RemoteIP)
		if err != nil {
			return nil, field.Invalid(staticPath.Child("remoteIP"), attach.Spec.Static.RemoteIP, "not a valid IP address")
		}
		if (attach.Spec.Static.IP == "" && !attach.Spec.Static.Proxy) || (attach.Spec.Static.IP != "" && attach.Spec.Static.Proxy) {
			return nil, field.Invalid(staticPath, field.OmitValueType{}, "either static.ip or static.proxy must be set")
		}
		if attach.Spec.Static.IP != "" {
			if _, _, err := net.ParseCIDR(attach.Spec.Static.IP); err != nil {
				return nil, field.Invalid(staticPath.Child("ip"), attach.Spec.Static.IP, "not a valid IP CIDR")
			}
		}
		// SONiC's FRR cannot attach BFD to a static route, only to a BGP session
		if attach.Spec.BFD != nil {
			return nil, field.Forbidden(spec.Child("bfd"), "must not be set for static external attachment")
		}
	}

	if bfd := attach.Spec.BFD; bfd != nil {
		// disableBFD wins, and nothing downstream reports that it did: without this the session
		// silently runs on the FRR defaults of 60/180 instead of the sub-second detection asked for
		if fabricCfg != nil && kube != nil {
			fabric, err := wiringapi.GetFabricSpec(ctx, kube, attach.Namespace, attach.Spec.Topology.Fabric)
			if err != nil {
				return nil, fmt.Errorf("failed to get fabric: %w", err)
			}
			if fabric.DisableBFD {
				warns = append(warns, "bfd is ignored because disableBFD is set for the whole fabric")
			}
		}
		if bfd.MinRX != 0 && (bfd.MinRX < BFDMinIntervalMS || bfd.MinRX > BFDMaxIntervalMS) {
			return nil, field.Invalid(spec.Child("bfd", "minRX"), bfd.MinRX, fmt.Sprintf("must be between %d and %d ms", BFDMinIntervalMS, BFDMaxIntervalMS))
		}
		if bfd.MinTX != 0 && (bfd.MinTX < BFDMinIntervalMS || bfd.MinTX > BFDMaxIntervalMS) {
			return nil, field.Invalid(spec.Child("bfd", "minTX"), bfd.MinTX, fmt.Sprintf("must be between %d and %d ms", BFDMinIntervalMS, BFDMaxIntervalMS))
		}
		if bfd.Multiplier != 0 && bfd.Multiplier < BFDMinMultiplier {
			return nil, field.Invalid(spec.Child("bfd", "multiplier"), bfd.Multiplier, fmt.Sprintf("must be at least %d", BFDMinMultiplier))
		}
	}

	if _, err := attach.Spec.InboundACL.Validate(spec.Child("inboundACL")); err != nil {
		return nil, err
	}

	if kube != nil {
		ext := &External{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: attach.Spec.External, Namespace: attach.Namespace}, ext); err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, field.NotFound(extPath, attach.Spec.External)
			}

			return nil, fmt.Errorf("failed to read external %s: %w", attach.Spec.External, err) // TODO replace with some internal error to not expose to the user
		}
		// the reverse is allowed: an external with static prefixes may also have BGP attachments,
		// which is how a static uplink is migrated to BGP without an outage
		if attach.Spec.Static != nil && ext.Spec.Static == nil {
			return nil, field.Invalid(extPath, attach.Spec.External, "external attachment is static but the external has no static prefixes")
		}
		if attach.Spec.Static == nil && ext.Spec.LocalASN != 0 {
			// with remote-as external FRR only refuses a neighbor with the switch's own ASN, not with the
			// localASN it presents instead, so a neighbor with the localASN would be taken as external
			if attach.Spec.Neighbor.ASN == 0 {
				return nil, field.Required(neighborPath.Child("asn"), fmt.Sprintf("external %s has a localASN", attach.Spec.External))
			}
			if attach.Spec.Neighbor.ASN == ext.Spec.LocalASN {
				return nil, field.Invalid(neighborPath.Child("asn"), attach.Spec.Neighbor.ASN, "the localASN of external "+attach.Spec.External)
			}
		}

		conn := &wiringapi.Connection{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: attach.Spec.Connection, Namespace: attach.Namespace}, conn); err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, field.NotFound(connPath, attach.Spec.Connection)
			}

			return nil, fmt.Errorf("failed to read connection %s: %w", attach.Spec.Connection, err) // TODO replace with some internal error to not expose to the user
		}

		attachFabric := attach.Spec.Topology.Fabric
		if extFabric := ext.Spec.Topology.Fabric; extFabric != attachFabric {
			return nil, field.Invalid(extPath, attach.Spec.External, fmt.Sprintf("attachment is in fabric %s but the external is in fabric %s", attachFabric, extFabric))
		}
		if connFabric := conn.Spec.Topology.Fabric; connFabric != attachFabric {
			return nil, field.Invalid(connPath, attach.Spec.Connection, fmt.Sprintf("attachment is in fabric %s but the connection is in fabric %s", attachFabric, connFabric))
		}

		if conn.Spec.External == nil {
			return nil, field.Invalid(connPath, attach.Spec.Connection, "connection is not external")
		}

		// border leaves in different domains attaching one External build the same VRF, so a leaf in
		// both would merge the two domains' routes to it
		extSwitches, err := ConnectionSwitches(ctx, kube, attach.Namespace, []string{attach.Spec.Connection})
		if err != nil {
			return nil, err
		}
		extDomain := ext.Spec.Topology.Domain
		for name, sw := range extSwitches {
			if swDomains := sw.Spec.Topology.Domains; !slices.Contains(swDomains, extDomain) {
				return nil, field.Invalid(connPath, attach.Spec.Connection, fmt.Sprintf("external %s is in domain %s but switch %s is in domains %v", attach.Spec.External, extDomain, name, swDomains))
			}
		}

		// validate VLAN collision
		attaches := &ExternalAttachmentList{}
		if err := kube.List(ctx, attaches, kclient.MatchingLabels{wiringapi.LabelName("connection"): attach.Spec.Connection}); err != nil {
			return nil, fmt.Errorf("failed to list external attachments for %s: %w", attach.Spec.Connection, err) // TODO replace with some internal error to not expose to the user
		}
		ourVLAN := attach.Spec.Switch.VLAN
		if attach.Spec.Static != nil {
			ourVLAN = attach.Spec.Static.VLAN
		}
		for _, other := range attaches.Items {
			if other.Name == attach.Name {
				continue
			}
			otherVLAN := other.Spec.Switch.VLAN
			if other.Spec.Static != nil {
				otherVLAN = other.Spec.Static.VLAN
			}
			if otherVLAN == ourVLAN {
				return nil, field.Invalid(connPath, attach.Spec.Connection, fmt.Sprintf("connection already has external attachment %s with VLAN %d", other.Name, ourVLAN))
			}
		}
	}

	return warns, nil
}
