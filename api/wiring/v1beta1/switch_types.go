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
	"maps"
	"net"
	"net/netip"
	"slices"
	"strings"
	"time"

	"go.githedgehog.com/fabric/api/meta"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	AnnotationSwitchRouteSumm       = "fabric.githedgehog.com/route-summ"
	DefaultLinkFlapThreshold        = 3
	DefaultLinkFlapSamplingInterval = 30
	DefaultLinkFlapRecoveryInterval = 300

	// PortLocatorDefaultExpire is the port locator LED expire time used if it's not specified
	PortLocatorDefaultExpire = 5 * time.Minute
	// PortLocatorMaxExpire is the maximum port locator LED expire time, longer ones are clamped to it
	PortLocatorMaxExpire = 20 * time.Minute
	// PortLocatorAllPorts is the port locator name applying to all ports of the switch, it supersedes any per-port entries
	PortLocatorAllPorts = "*"
)

// +kubebuilder:validation:Enum=spine;server-leaf;border-leaf;mixed-leaf;virtual-edge
// SwitchRole is the role of the switch, could be spine, server-leaf or border-leaf or mixed-leaf
type SwitchRole string

const (
	SwitchRoleSpine      SwitchRole = "spine"
	SwitchRoleServerLeaf SwitchRole = "server-leaf"
	SwitchRoleBorderLeaf SwitchRole = "border-leaf"
	SwitchRoleMixedLeaf  SwitchRole = "mixed-leaf"
)

var SwitchRoles = []SwitchRole{
	SwitchRoleSpine,
	SwitchRoleServerLeaf,
	SwitchRoleBorderLeaf,
	SwitchRoleMixedLeaf,
}

func (r SwitchRole) IsSpine() bool {
	return r == SwitchRoleSpine
}

func (r SwitchRole) IsLeaf() bool {
	return r == SwitchRoleServerLeaf || r == SwitchRoleBorderLeaf || r == SwitchRoleMixedLeaf
}

// +kubebuilder:validation:Enum=rs;fc;auto;disabled
// PortFECMode is the FEC mode for a port
type PortFECMode string

const (
	PortFECModeRS       PortFECMode = "rs"
	PortFECModeFC       PortFECMode = "fc"
	PortFECModeAuto     PortFECMode = "auto"
	PortFECModeDisabled PortFECMode = "disabled"
)

// SwitchRedundancy is the switch redundancy configuration which includes name of the redundancy group switch belongs
// to and its type, used for ESLAG connections. It defines how redundancy will be configured and handled
// on the switch as well as which connection types will be available. If not specified, switch will not be part of any
// redundancy group. If name isn't empty, type must be specified as well and name should be the same as one of the
// SwitchGroup objects.
type SwitchRedundancy struct {
	// Group is the name of the redundancy group switch belongs to
	Group string `json:"group,omitempty"`
	// Type is the type of the redundancy group, although only eslag is supported now
	Type meta.RedundancyType `json:"type,omitempty"`
}

// SwitchBoot identifies the switch when it's booted to install the NOS, by serial number or MAC address. Each of them
// must be unique across switches.
type SwitchBoot struct {
	// Identify switch by serial number, compared case-insensitively
	Serial string `json:"serial,omitempty"`
	// Identify switch by MAC address of the management port, accepted in any common format and normalized to lower case
	// colon-separated form, e.g. 0c:20:12:fe:01:00
	MAC string `json:"mac,omitempty"`
}

// NormalizeBootSerial normalizes a serial number used to identify a switch during the bootstrap, so it matches
// regardless of surrounding whitespace and case
func NormalizeBootSerial(serial string) string {
	return strings.ToLower(strings.TrimSpace(serial))
}

// NormalizeBootMAC parses a MAC address used to identify a switch during the bootstrap in any format supported by
// net.ParseMAC and returns it in the canonical form, so it matches regardless of how it's written. Empty stays empty.
func NormalizeBootMAC(mac string) (string, error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return "", nil
	}

	hw, err := net.ParseMAC(mac)
	if err != nil {
		return "", fmt.Errorf("parsing MAC %q: %w", mac, err)
	}
	if len(hw) != 6 {
		return "", fmt.Errorf("MAC %q is not a 48-bit MAC address", mac) //nolint:err113
	}

	return hw.String(), nil
}

// SwitchTopology is where a Switch sits in the fabric topology
type SwitchTopology struct {
	// Fabric is the name of the Fabric this switch belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
	// Domains is the list of the Fabric domains (spine layers) this switch belongs to (if not specified, "default" is used).
	// A spine belongs to exactly one, a leaf to several if it uplinks to the spines of several. Immutable,
	// and the switches of a redundancy group must be in the same domains
	Domains []string `json:"domains,omitempty"`
}

// SwitchSpec defines the desired state of Switch
type SwitchSpec struct {
	// +kubebuilder:validation:Required
	// Role is the role of the switch, could be spine, server-leaf or border-leaf or mixed-leaf
	Role SwitchRole `json:"role,omitempty"`
	// Description is a description of the switch
	Description string `json:"description,omitempty"`
	// Profile is the profile of the switch, name of the SwitchProfile object to be used for this switch, currently not used by the Fabric
	Profile string `json:"profile,omitempty"`
	// Topology is where the switch sits in the fabric topology
	Topology SwitchTopology `json:"topology,omitempty"`
	// Groups is a list of switch groups the switch belongs to
	Groups []string `json:"groups,omitempty"`
	// Redundancy is the switch redundancy configuration including name of the redundancy group switch belongs to and its type, used for ESLAG connections
	Redundancy SwitchRedundancy `json:"redundancy,omitempty"`
	// VLANNamespaces is a list of VLAN namespaces the switch is part of, their VLAN ranges could not overlap
	VLANNamespaces []string `json:"vlanNamespaces,omitempty"`
	// ASN is the ASN of the switch
	ASN uint32 `json:"asn,omitempty"`
	// IP is the IP of the switch that could be used to access it from other switches and control nodes in the Fabric
	IP string `json:"ip,omitempty"`
	// VTEPIP is the VTEP IP of the switch
	VTEPIP string `json:"vtepIP,omitempty"`
	// ProtocolIP is used as BGP Router ID for switch configuration
	ProtocolIP string `json:"protocolIP,omitempty"`
	// PortGroupSpeeds is a map of port group speeds, key is the port group name, value is the speed, such as '"2": 10G'
	PortGroupSpeeds map[string]string `json:"portGroupSpeeds,omitempty"`
	// PortSpeeds is a map of port speeds, key is the port name, value is the speed
	PortSpeeds map[string]string `json:"portSpeeds,omitempty"`
	// PortBreakouts is a map of port breakouts, key is the port name, value is the breakout configuration, such as "1/55: 4x25G"
	PortBreakouts map[string]string `json:"portBreakouts,omitempty"`
	// PortAutoNegs is a map of port auto negotiation, key is the port name, value is true or false
	PortAutoNegs map[string]bool `json:"portAutoNegs,omitempty"`
	// PortFECs is a map of port FEC modes, key is the port name, value is the FEC mode (rs/fc/auto/disabled).
	// Use only as last resort: removing a value from the map does NOT reset the port's FEC to its default,
	// instead that value persists on the device until a full config reset or a new explicit config
	PortFECs map[string]PortFECMode `json:"portFECs,omitempty"`
	// PortLocators is a map of port locator LED expire times, key is the port name such as E1/1 (incl. breakout
	// sub-ports such as E1/53/1) or "*" to apply to all ports of the switch, value is a duration relative to now
	// such as 10m, an exact expire time in the "2026-08-05 18:45:47" (UTC) format or empty for the default 5 minutes.
	// Values are normalized to the exact expire time and clamped to at most 20 minutes from now, already expired
	// entries are removed as well as all per-port ones if "*" is present
	PortLocators map[string]string `json:"portLocators,omitempty"`
	// Boot is the boot/provisioning information of the switch
	Boot SwitchBoot `json:"boot,omitempty"`
	// EnableAllPorts is a flag to enable all ports on the switch regardless of them being used or not
	EnableAllPorts bool `json:"enableAllPorts,omitempty"`
	// RoCE is a flag to enable RoCEv2 support on the switch which includes lossless queues and QoS configuration
	RoCE bool `json:"roce,omitempty"`
	// ECMP is the ECMP configuration for the switch
	ECMP SwitchECMP `json:"ecmp,omitempty"`
	// LinkFlapErrDisable, if set, enables link-flap errdisable protection on all fabric-facing ports.
	// When a port exceeds FlapThreshold link-down events within SamplingInterval seconds it is
	// disabled; RecoveryInterval controls how long before it is automatically re-enabled (0 = never).
	LinkFlapErrDisable *SwitchLinkFlapErrDisable `json:"linkFlapErrDisable,omitempty"`
}

// SwitchECMP is a struct that defines the ECMP configuration for the switch
type SwitchECMP struct {
	// RoCEQPN is a flag to enable RoCE QPN hashing
	RoCEQPN bool `json:"roceQPN,omitempty"`
}

// SwitchLinkFlapErrDisable configures link-flap errdisable on fabric-facing ports.
// Presence of this struct enables the feature; omitting it leaves the ports unprotected.
type SwitchLinkFlapErrDisable struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50
	// +kubebuilder:default=3
	// FlapThreshold is the number of link-down events within SamplingInterval that triggers errdisable.
	// Defaults to 3.
	FlapThreshold *uint8 `json:"flapThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=30
	// SamplingInterval is the observation window in seconds for counting flap events.
	// Defaults to 30 s.
	SamplingInterval *uint32 `json:"samplingInterval,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65534
	// +kubebuilder:default=300
	// RecoveryInterval is how long in seconds before the port is automatically re-enabled.
	// 0 means the port is never automatically re-enabled and must be recovered manually.
	// Defaults to 300 s.
	RecoveryInterval *uint32 `json:"recoveryInterval,omitempty"`
}

// SwitchStatus defines the observed state of Switch
type SwitchStatus struct {
	// Applied ApplyStatus `json:"applied,omitempty"`
	// TODO: add port status fields
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=sw
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
// +kubebuilder:printcolumn:name="Domains",type=string,JSONPath=`.spec.topology.domains`,priority=0
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profile`,priority=0
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`,priority=0
// +kubebuilder:printcolumn:name="Descr",type=string,JSONPath=`.spec.description`,priority=0
// +kubebuilder:printcolumn:name="Boot",type=string,JSONPath=`.spec.boot`,priority=0
// +kubebuilder:printcolumn:name="VTEPIP",type=string,JSONPath=`.spec.vtepIP`,priority=0
// +kubebuilder:printcolumn:name="Redundancy",type=string,JSONPath=`.spec.redundancy.type`,priority=1
// +kubebuilder:printcolumn:name="Groups",type=string,JSONPath=`.spec.groups`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
// Switch is the Schema for the switches API
type Switch struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is desired state of the switch
	Spec SwitchSpec `json:"spec,omitempty"`
	// Status is the observed state of the switch
	Status SwitchStatus `json:"status,omitempty"`
}

const KindSwitch = "Switch"

// +kubebuilder:object:root=true

// SwitchList contains a list of Switch
type SwitchList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []Switch `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &Switch{}, &SwitchList{})

		return nil
	})
}

var (
	_ meta.Object     = (*Switch)(nil)
	_ meta.ObjectList = (*SwitchList)(nil)
)

func (swList *SwitchList) GetItems() []meta.Object {
	items := make([]meta.Object, len(swList.Items))
	for i := range swList.Items {
		items[i] = &swList.Items[i]
	}

	return items
}

func (sw *Switch) Default() {
	meta.DefaultObjectMetadata(sw)

	if sw.Labels == nil {
		sw.Labels = map[string]string{}
	}

	CleanupFabricLabels(sw.Labels)

	// the case is kept as written, it's compared case-insensitively
	sw.Spec.Boot.Serial = strings.TrimSpace(sw.Spec.Boot.Serial)
	// an invalid one is rejected by the validation
	if mac, err := NormalizeBootMAC(sw.Spec.Boot.MAC); err == nil {
		sw.Spec.Boot.MAC = mac
	}

	for name, value := range sw.Spec.PortGroupSpeeds {
		sw.Spec.PortGroupSpeeds[name], _ = strings.CutPrefix(value, "SPEED_")
	}

	for name, value := range sw.Spec.PortSpeeds {
		sw.Spec.PortSpeeds[name], _ = strings.CutPrefix(value, "SPEED_")
	}

	if _, exists := sw.Spec.PortLocators[PortLocatorAllPorts]; exists {
		// the all-ports entry supersedes the per-port ones
		for name := range sw.Spec.PortLocators {
			if name != PortLocatorAllPorts {
				delete(sw.Spec.PortLocators, name)
			}
		}
	}

	for name, value := range sw.Spec.PortLocators {
		ok, expire, err := NormalizePortLocator(value)
		if err != nil {
			// leave unparseable values as-is, validation will reject them
			continue
		}

		if !ok {
			delete(sw.Spec.PortLocators, name)

			continue
		}

		sw.Spec.PortLocators[name] = expire
	}

	if len(sw.Spec.VLANNamespaces) == 0 {
		sw.Spec.VLANNamespaces = []string{"default"}
	}

	if sw.Spec.Topology.Fabric == "" {
		sw.Spec.Topology.Fabric = DefaultFabric
	}

	if len(sw.Spec.Topology.Domains) == 0 {
		sw.Spec.Topology.Domains = []string{DefaultFabricDomain}
	}

	if sw.Spec.Redundancy.Group != "" && !slices.Contains(sw.Spec.Groups, sw.Spec.Redundancy.Group) {
		sw.Spec.Groups = append(sw.Spec.Groups, sw.Spec.Redundancy.Group)
	}

	if sw.Spec.LinkFlapErrDisable != nil {
		if sw.Spec.LinkFlapErrDisable.FlapThreshold == nil {
			sw.Spec.LinkFlapErrDisable.FlapThreshold = new(uint8(DefaultLinkFlapThreshold))
		}
		if sw.Spec.LinkFlapErrDisable.SamplingInterval == nil {
			sw.Spec.LinkFlapErrDisable.SamplingInterval = new(uint32(DefaultLinkFlapSamplingInterval))
		}
		if sw.Spec.LinkFlapErrDisable.RecoveryInterval == nil {
			sw.Spec.LinkFlapErrDisable.RecoveryInterval = new(uint32(DefaultLinkFlapRecoveryInterval))
		}
	}

	for _, group := range sw.Spec.Groups {
		sw.Labels[ListLabelSwitchGroup(group)] = ListLabelValue
	}

	for _, vlanNs := range sw.Spec.VLANNamespaces {
		sw.Labels[ListLabelVLANNamespace(vlanNs)] = ListLabelValue
	}

	sw.Labels[ListLabelFabric(sw.Spec.Topology.Fabric)] = ListLabelValue

	for _, domain := range sw.Spec.Topology.Domains {
		// validation rejects an empty name, but as a label key it would be refused first with a vaguer error
		if domain != "" {
			sw.Labels[ListLabelDomain(domain)] = ListLabelValue
		}
	}

	slices.Sort(sw.Spec.Groups)
	slices.Sort(sw.Spec.VLANNamespaces)
	slices.Sort(sw.Spec.Topology.Domains)

	sw.Labels[LabelProfile] = sw.Spec.Profile
}

func (sw *Switch) HydrationValidation(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) error {
	if kube == nil {
		return fmt.Errorf("kube client is required for hydration validations") //nolint:err113
	}

	switches := &SwitchList{}
	if err := kube.List(ctx, switches); err != nil {
		return fmt.Errorf("failed to list switches for hydration validation: %w", err)
	}

	leafASNs := map[uint32]bool{}
	VTEPs := map[string]bool{}
	protocolIPs := map[string]bool{}
	mgmtIPs := map[netip.Addr]bool{}
	vtepSubnet, err := netip.ParsePrefix(fabricCfg.VTEPSubnet)
	if err != nil {
		return fmt.Errorf("failed to parse Fabric VTEP subnet %s: %w", fabricCfg.VTEPSubnet, err)
	}
	protocolSubnet, err := netip.ParsePrefix(fabricCfg.ProtocolSubnet)
	if err != nil {
		return fmt.Errorf("failed to parse Fabric protocol subnet %s: %w", fabricCfg.ProtocolSubnet, err)
	}
	controlVIP, err := netip.ParsePrefix(fabricCfg.ControlVIP)
	if err != nil {
		return fmt.Errorf("failed to parse Fabric control VIP %s: %w", fabricCfg.ControlVIP, err)
	}
	mgmtSubnet, err := netip.ParsePrefix(fabricCfg.ManagementSubnet)
	if err != nil {
		return fmt.Errorf("failed to parse Fabric management subnet %s: %w", fabricCfg.ManagementSubnet, err)
	}
	mgmtDHCPStart, err := netip.ParseAddr(fabricCfg.ManagementDHCPStart)
	if err != nil {
		return fmt.Errorf("failed to parse Fabric management DHCP start %s: %w", fabricCfg.ManagementDHCPStart, err)
	}
	mgmtIPs[controlVIP.Addr()] = true

	spec := field.NewPath("spec")

	// switches are identified by them during the bootstrap, compared normalized as the stored ones may not be yet
	bootSerials := map[string]string{}
	bootMACs := map[string]string{}

	for _, other := range switches.Items {
		if other.Name == sw.Name && other.Namespace == sw.Namespace {
			continue
		}
		if serial := NormalizeBootSerial(other.Spec.Boot.Serial); serial != "" {
			bootSerials[serial] = other.Name
		}
		if mac, err := NormalizeBootMAC(other.Spec.Boot.MAC); err == nil && mac != "" {
			bootMACs[mac] = other.Name
		}
		if other.Spec.ASN != 0 && other.Spec.Role.IsLeaf() {
			leafASNs[other.Spec.ASN] = true
		}
		if other.Spec.VTEPIP != "" {
			VTEPs[other.Spec.VTEPIP] = true
		}
		if other.Spec.ProtocolIP != "" {
			protocolIPs[other.Spec.ProtocolIP] = true
		}
		if other.Spec.IP != "" {
			if ip, err := netip.ParsePrefix(other.Spec.IP); err == nil {
				mgmtIPs[ip.Addr()] = true
			}
		}
	}

	// collect gateways as well for VTEP and protocol IP uniqueness checks
	// use unstructured list to avoid import dependency cycle
	if fabricCfg != nil && fabricCfg.EnableGateway {
		gateways := &unstructured.UnstructuredList{}
		gateways.SetGroupVersionKind(schema.GroupVersionKind{
			Group: "gateway.githedgehog.com", Version: "v1alpha1", Kind: "GatewayList",
		})
		if err := kube.List(ctx, gateways); err != nil {
			return fmt.Errorf("failed to list gateways for hydration validation: %w", err)
		}

		for _, gw := range gateways.Items {
			spec, ok := gw.Object["spec"].(map[string]any)
			if !ok {
				continue
			}
			if vtepIP, ok := spec["vtepIP"].(string); ok && vtepIP != "" {
				VTEPs[vtepIP] = true
			}
			if protoIP, ok := spec["protocolIP"].(string); ok && protoIP != "" {
				protocolIPs[protoIP] = true
			}
		}
	}

	if serial := NormalizeBootSerial(sw.Spec.Boot.Serial); serial != "" {
		if other, exist := bootSerials[serial]; exist {
			return field.Invalid(spec.Child("boot", "serial"), sw.Spec.Boot.Serial, "already used by switch "+other)
		}
	}
	mac, err := NormalizeBootMAC(sw.Spec.Boot.MAC)
	if err != nil {
		return field.Invalid(spec.Child("boot", "mac"), sw.Spec.Boot.MAC, err.Error())
	}
	if other, exist := bootMACs[mac]; exist && mac != "" {
		return field.Invalid(spec.Child("boot", "mac"), sw.Spec.Boot.MAC, "already used by switch "+other)
	}

	if sw.Spec.IP != "" {
		swIP, err := netip.ParsePrefix(sw.Spec.IP)
		if err != nil {
			return field.Invalid(spec.Child("ip"), sw.Spec.IP, err.Error())
		}

		if !mgmtSubnet.Contains(swIP.Addr()) {
			return field.Invalid(spec.Child("ip"), sw.Spec.IP, "not in the management subnet "+mgmtSubnet.String())
		}
		if swIP.Addr().Compare(mgmtDHCPStart) >= 0 {
			return field.Invalid(spec.Child("ip"), sw.Spec.IP, "in the management DHCP range starting at "+mgmtDHCPStart.String())
		}
		if _, exist := mgmtIPs[swIP.Addr()]; exist {
			return field.Invalid(spec.Child("ip"), sw.Spec.IP, "already in use")
		}
	}

	if sw.Spec.ProtocolIP != "" {
		swProtoIP, err := netip.ParsePrefix(sw.Spec.ProtocolIP)
		if err != nil {
			return field.Invalid(spec.Child("protocolIP"), sw.Spec.ProtocolIP, err.Error())
		}
		if swProtoIP.Bits() != 32 {
			return field.Invalid(spec.Child("protocolIP"), sw.Spec.ProtocolIP, "must be a /32")
		}

		if !protocolSubnet.Contains(swProtoIP.Addr()) {
			return field.Invalid(spec.Child("protocolIP"), sw.Spec.ProtocolIP, "not in the protocol subnet "+protocolSubnet.String())
		}

		if _, exist := protocolIPs[sw.Spec.ProtocolIP]; exist {
			return field.Invalid(spec.Child("protocolIP"), sw.Spec.ProtocolIP, "already in use")
		}
	}

	// check leaf ASN uniqueness
	if sw.Spec.Role.IsLeaf() {
		// TODO: remove MCLAG exception once we don't need to worry about upgrading systems which still support it
		if _, exist := leafASNs[sw.Spec.ASN]; exist && sw.Spec.Redundancy.Type != meta.RedundancyTypeMCLAG {
			return field.Invalid(spec.Child("asn"), sw.Spec.ASN, "already in use")
		}
	}

	fabric, err := GetFabricSpec(ctx, kube, sw.Namespace, sw.Spec.Topology.Fabric)
	if err != nil {
		return field.Invalid(spec.Child("topology", "fabric"), sw.Spec.Topology.Fabric, err.Error())
	}
	if sw.Spec.Role.IsLeaf() && (sw.Spec.ASN < fabric.LeafASNStart || sw.Spec.ASN > fabric.LeafASNEnd) {
		return field.Invalid(spec.Child("asn"), sw.Spec.ASN, fmt.Sprintf("not within the fabric leaf ASN range %d-%d", fabric.LeafASNStart, fabric.LeafASNEnd))
	}
	if sw.Spec.Role.IsSpine() {
		// a spine is in exactly one domain, which Validate checks too
		if len(sw.Spec.Topology.Domains) != 1 {
			return field.Invalid(spec.Child("topology", "domains"), sw.Spec.Topology.Domains, fmt.Sprintf("spine must be in exactly one domain, found %d", len(sw.Spec.Topology.Domains)))
		}
		domainName := sw.Spec.Topology.Domains[0]
		domain, exists := fabric.Domains[domainName]
		if !exists {
			return field.Invalid(spec.Child("topology", "domains"), domainName, "domain not found in fabric")
		}
		if sw.Spec.ASN != domain.SpineASN {
			return field.Invalid(spec.Child("asn"), sw.Spec.ASN, fmt.Sprintf("not the spine ASN %d of domain %s", domain.SpineASN, domainName))
		}
	}

	// leaf vtep IP uniqueness
	if sw.Spec.Role.IsLeaf() {
		swVTEPIP, err := netip.ParsePrefix(sw.Spec.VTEPIP)
		if err != nil {
			return field.Invalid(spec.Child("vtepIP"), sw.Spec.VTEPIP, err.Error())
		}
		if swVTEPIP.Bits() != 32 {
			return field.Invalid(spec.Child("vtepIP"), sw.Spec.VTEPIP, "must be a /32")
		}

		if !vtepSubnet.Contains(swVTEPIP.Addr()) {
			return field.Invalid(spec.Child("vtepIP"), sw.Spec.VTEPIP, "not in the VTEP subnet "+vtepSubnet.String())
		}

		// TODO: remove MCLAG exception once we don't need to worry about upgrading systems which still support it
		if _, exist := VTEPs[sw.Spec.VTEPIP]; exist && sw.Spec.Redundancy.Type != meta.RedundancyTypeMCLAG {
			return field.Invalid(spec.Child("vtepIP"), sw.Spec.VTEPIP, "already in use")
		}
	}

	return nil
}

func (sw *Switch) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	var warnings admission.Warnings

	if err := meta.ValidateObjectMetadata(sw); err != nil {
		return nil, err //nolint:wrapcheck
	}

	spec := field.NewPath("spec")
	domainsPath, profilePath := spec.Child("topology", "domains"), spec.Child("profile")

	if len(sw.Spec.VLANNamespaces) == 0 {
		return nil, field.Required(spec.Child("vlanNamespaces"), "at least one VLAN namespace required")
	}
	domains := slices.Sorted(slices.Values(sw.Spec.Topology.Domains))
	if len(domains) == 0 {
		return nil, field.Required(domainsPath, "at least one domain is required")
	}
	if sw.Spec.Role.IsSpine() && len(domains) != 1 {
		return nil, field.Invalid(domainsPath, sw.Spec.Topology.Domains, fmt.Sprintf("spine must be in exactly one domain, found %d", len(domains)))
	}
	if slices.Contains(domains, "") {
		return nil, field.Invalid(domainsPath, sw.Spec.Topology.Domains, "domain name cannot be empty")
	}
	if len(slices.Compact(slices.Clone(domains))) != len(domains) {
		return nil, field.Invalid(domainsPath, sw.Spec.Topology.Domains, "domains must be unique")
	}

	if err := CheckFabricExists(ctx, kube, sw.Namespace, sw.Spec.Topology.Fabric); err != nil {
		return nil, err
	}
	if sw.Spec.ASN == 0 {
		return nil, field.Required(spec.Child("asn"), "")
	}
	if sw.Spec.IP == "" {
		return nil, field.Required(spec.Child("ip"), "")
	}
	if sw.Spec.ProtocolIP == "" {
		return nil, field.Required(spec.Child("protocolIP"), "")
	}
	if sw.Spec.Role.IsLeaf() && fabricCfg != nil && fabricCfg.FabricMode == meta.FabricModeSpineLeaf && sw.Spec.VTEPIP == "" {
		return nil, field.Required(spec.Child("vtepIP"), "required for leaf switches in spine-leaf mode")
	}
	if sw.Spec.Role.IsSpine() && sw.Spec.VTEPIP != "" {
		return nil, field.Forbidden(spec.Child("vtepIP"), "not allowed for spine switches")
	}

	if sw.Spec.Profile == "" {
		return nil, field.Required(profilePath, "")
	}

	if _, err := NormalizeBootMAC(sw.Spec.Boot.MAC); err != nil {
		return nil, field.Invalid(spec.Child("boot", "mac"), sw.Spec.Boot.MAC, err.Error())
	}

	redundancyPath := spec.Child("redundancy")
	if !slices.Contains(meta.RedundancyTypes, sw.Spec.Redundancy.Type) {
		return nil, field.NotSupported(redundancyPath.Child("type"), sw.Spec.Redundancy.Type, meta.RedundancyTypes)
	}
	if sw.Spec.Redundancy.Group != "" && sw.Spec.Redundancy.Type == meta.RedundancyTypeNone {
		return nil, field.Required(redundancyPath.Child("type"), "redundancy group specified without type")
	}
	if sw.Spec.Redundancy.Group == "" && sw.Spec.Redundancy.Type != meta.RedundancyTypeNone {
		return nil, field.Required(redundancyPath.Child("group"), "redundancy type specified without group")
	}

	if sw.Spec.LinkFlapErrDisable != nil {
		flapPath := spec.Child("linkFlapErrDisable")
		if sw.Spec.LinkFlapErrDisable.FlapThreshold == nil {
			return nil, field.Required(flapPath.Child("flapThreshold"), "")
		}
		if sw.Spec.LinkFlapErrDisable.SamplingInterval == nil {
			return nil, field.Required(flapPath.Child("samplingInterval"), "")
		}
		if sw.Spec.LinkFlapErrDisable.RecoveryInterval == nil {
			return nil, field.Required(flapPath.Child("recoveryInterval"), "")
		}
	}

	if kube != nil {
		namespaces := &VLANNamespaceList{}
		err := kube.List(ctx, namespaces)
		if err != nil {
			return nil, fmt.Errorf("failed to get VLAN namespaces: %w", err) // TODO replace with some internal error to not expose to the user
		}

		ranges := []meta.VLANRange{}

		for idx, ns := range sw.Spec.VLANNamespaces {
			found := false
			for _, other := range namespaces.Items {
				if ns == other.Name {
					found = true
					ranges = append(ranges, other.Spec.Ranges...)

					break
				}
			}
			if !found {
				return nil, field.NotFound(spec.Child("vlanNamespaces").Index(idx), ns)
			}
		}

		if err := meta.CheckVLANRangesOverlap(ranges); err != nil {
			return nil, field.Invalid(spec.Child("vlanNamespaces"), sw.Spec.VLANNamespaces, err.Error())
		}

		fabric, err := GetFabricSpec(ctx, kube, sw.Namespace, sw.Spec.Topology.Fabric)
		if err != nil {
			return nil, err
		}
		swFabric := sw.Spec.Topology.Fabric
		for _, domain := range domains {
			if _, exists := fabric.Domains[domain]; !exists {
				return nil, field.Invalid(domainsPath, domain, fmt.Sprintf("domain %s not found in fabric %s", domain, swFabric))
			}
		}

		// switches in an ESLAG group that don't share a domain never exchange the EVPN routes
		// multihoming needs for DF election and split horizon
		if group := sw.Spec.Redundancy.Group; group != "" {
			switches := &SwitchList{}
			if err := kube.List(ctx, switches, kclient.InNamespace(sw.Namespace), kclient.MatchingLabels{ListLabelSwitchGroup(group): ListLabelValue}); err != nil {
				return nil, fmt.Errorf("failed to list switches: %w", err) // TODO replace with some internal error to not expose to the user
			}
			for _, other := range switches.Items {
				if other.Name == sw.Name || other.Spec.Redundancy.Group != group {
					continue
				}
				if otherDomains := slices.Sorted(slices.Values(other.Spec.Topology.Domains)); !slices.Equal(domains, otherDomains) {
					return nil, field.Invalid(domainsPath, sw.Spec.Topology.Domains, fmt.Sprintf("switches of redundancy group %s must be in the same domains, switch %s is in domains %v", group, other.Name, otherDomains))
				}
			}
		}

		for idx, group := range sw.Spec.Groups {
			groupPath := spec.Child("groups").Index(idx)
			if group == "" {
				return nil, field.Required(groupPath, "")
			}

			sg := &SwitchGroup{}
			err = kube.Get(ctx, ktypes.NamespacedName{Name: group, Namespace: sw.Namespace}, sg)
			if err != nil {
				if kapierrors.IsNotFound(err) {
					return nil, field.NotFound(groupPath, group)
				}

				return nil, fmt.Errorf("failed to get switch group %s: %w", group, err) // TODO replace with some internal error to not expose to the user
			}

			if sgFabric := sg.Spec.Topology.Fabric; sgFabric != swFabric {
				return nil, field.Invalid(groupPath, group, fmt.Sprintf("switch is in fabric %s but the switch group is in fabric %s", swFabric, sgFabric))
			}
		}

		sp := &SwitchProfile{}
		err = kube.Get(ctx, ktypes.NamespacedName{Name: sw.Spec.Profile, Namespace: sw.Namespace}, sp)
		if err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, field.NotFound(profilePath, sw.Spec.Profile)
			}

			return nil, fmt.Errorf("failed to get switch profile %s: %w", sw.Spec.Profile, err) // TODO replace with some internal error to not expose to the user
		}

		for name, speed := range sw.Spec.PortGroupSpeeds {
			path := spec.Child("portGroupSpeeds").Key(name)
			group, exists := sp.Spec.PortGroups[name]
			if !exists {
				return nil, field.Invalid(spec.Child("portGroupSpeeds"), name, "port group not found in switch profile")
			}

			profile, exists := sp.Spec.PortProfiles[group.Profile]
			if !exists {
				return nil, field.Invalid(path, speed, fmt.Sprintf("port profile %s not found in switch profile", group.Profile))
			}

			if profile.Speed == nil {
				return nil, field.Invalid(path, speed, fmt.Sprintf("port profile %s has no supported speeds", group.Profile))
			}

			if !slices.Contains(profile.Speed.Supported, speed) {
				return nil, field.NotSupported(path, speed, profile.Speed.Supported)
			}
		}

		for name, speed := range sw.Spec.PortSpeeds {
			path := spec.Child("portSpeeds").Key(name)
			port, exists := sp.Spec.Ports[name]
			if !exists {
				return nil, field.Invalid(spec.Child("portSpeeds"), name, "port not found in switch profile")
			}

			profile, exists := sp.Spec.PortProfiles[port.Profile]
			if !exists {
				return nil, field.Invalid(path, speed, fmt.Sprintf("port profile %s not found in switch profile", port.Profile))
			}

			if profile.Speed == nil {
				return nil, field.Invalid(path, speed, fmt.Sprintf("port profile %s has no supported speeds", port.Profile))
			}

			if !slices.Contains(profile.Speed.Supported, speed) {
				return nil, field.NotSupported(path, speed, profile.Speed.Supported)
			}
		}

		for name, breakout := range sw.Spec.PortBreakouts {
			path := spec.Child("portBreakouts").Key(name)
			port, exists := sp.Spec.Ports[name]
			if !exists {
				return nil, field.Invalid(spec.Child("portBreakouts"), name, "port not found in switch profile")
			}

			profile, exists := sp.Spec.PortProfiles[port.Profile]
			if !exists {
				return nil, field.Invalid(path, breakout, fmt.Sprintf("port profile %s not found in switch profile", port.Profile))
			}

			if profile.Breakout == nil {
				return nil, field.Invalid(path, breakout, fmt.Sprintf("port profile %s has no supported breakouts", port.Profile))
			}

			if _, exists := profile.Breakout.Supported[breakout]; !exists {
				return nil, field.NotSupported(path, breakout, slices.Sorted(maps.Keys(profile.Breakout.Supported)))
			}
		}

		autoNegAllowed, _, err := sp.Spec.GetAutoNegsDefaultsFor(&sw.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to get auto negotiation defaults: %w", err)
		}

		for name := range sw.Spec.PortAutoNegs {
			if !autoNegAllowed[name] {
				return nil, field.Invalid(spec.Child("portAutoNegs"), name, "port does not support configuring auto negotiation")
			}
		}

		fecPorts, err := sp.Spec.GetFECConfigurablePorts(&sw.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to get FEC-configurable ports: %w", err)
		}

		for name := range sw.Spec.PortFECs {
			if !fecPorts[name] {
				return nil, field.Invalid(spec.Child("portFECs"), name, "port does not support configuring FEC "+
					"(a broken-out port must be configured per sub-port, e.g. E1/53/1)")
			}
		}

		apiPorts, err := sp.Spec.GetAvailableAPIPorts(&sw.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to get available API ports: %w", err)
		}

		if len(sw.Spec.PortLocators) > 0 && !sp.Spec.Features.PortLocator {
			return nil, field.Forbidden(spec.Child("portLocators"), "not supported on switch profile "+sw.Spec.Profile)
		}

		for name, expire := range sw.Spec.PortLocators {
			if name != PortLocatorAllPorts && !apiPorts[name] {
				return nil, field.Invalid(spec.Child("portLocators"), name, "not a valid port for port locators")
			}

			if _, _, err := NormalizePortLocator(expire); err != nil {
				return nil, field.Invalid(spec.Child("portLocators").Key(name), expire, err.Error())
			}
		}

		if sw.Spec.RoCE && !sp.Spec.Features.RoCE {
			return nil, field.Forbidden(spec.Child("roce"), "RoCEv2 is not supported on switch profile "+sw.Spec.Profile)
		}

		if sw.Spec.ECMP.RoCEQPN && !sp.Spec.Features.ECMPRoCEQPN {
			return nil, field.Forbidden(spec.Child("ecmp", "roceQPN"), "ECMP RoCE QPN hashing is not supported on switch profile "+sw.Spec.Profile)
		}

		switch sw.Spec.Redundancy.Type {
		case meta.RedundancyTypeNone:
			// No redundancy, nothing to check
		case meta.RedundancyTypeMCLAG:
			warnings = append(warnings, "MCLAG is deprecated and no longer supported")
		case meta.RedundancyTypeESLAG:
			if !sp.Spec.Features.ESLAG {
				return nil, field.Invalid(redundancyPath.Child("type"), sw.Spec.Redundancy.Type, "ESLAG is not supported on switch profile "+sw.Spec.Profile)
			}
		}

		totalPorts := uint16(0)
		pipelinePorts := map[string]uint16{}
		pipelinePortNames := map[string][]string{}
		for name, port := range sp.Spec.Ports {
			if port.Management {
				continue
			}

			ports := uint16(1)
			if port.Group == "" {
				if port.Profile == "" {
					return warnings, field.Invalid(profilePath, sw.Spec.Profile, fmt.Sprintf("port %s has no group or profile", name))
				}
				profile, ok := sp.Spec.PortProfiles[port.Profile]
				if !ok {
					return warnings, field.Invalid(profilePath, sw.Spec.Profile, fmt.Sprintf("port %s has invalid profile %s", name, port.Profile))
				}

				if profile.Breakout != nil {
					mode := profile.Breakout.Default
					if sw.Spec.PortBreakouts != nil {
						if override, ok := sw.Spec.PortBreakouts[name]; ok {
							mode = override
						}
					}

					if breakout, ok := profile.Breakout.Supported[mode]; ok {
						ports = uint16(len(breakout.Offsets)) //nolint:gosec
					} else {
						return warnings, field.Invalid(spec.Child("portBreakouts"), field.OmitValueType{}, fmt.Sprintf("port %s has invalid breakout mode %s", name, mode))
					}
				}
			}

			totalPorts += ports
			if port.Pipeline != "" {
				pipelinePorts[port.Pipeline] += ports
				pipelinePortNames[port.Pipeline] = append(pipelinePortNames[port.Pipeline], name)
			}
		}

		for _, ports := range pipelinePortNames {
			slices.Sort(ports)
		}

		if sp.Spec.MaxPorts > 0 && totalPorts > sp.Spec.MaxPorts {
			return warnings, field.Invalid(spec.Child("portBreakouts"), field.OmitValueType{}, fmt.Sprintf("switch profile %s maximum ports exceeded: %d > %d", sp.Name, totalPorts, sp.Spec.MaxPorts))
		}

		if sp.Spec.Pipelines != nil {
			for pipeline, ports := range pipelinePorts {
				if portPipeline, ok := sp.Spec.Pipelines[pipeline]; ok {
					if ports > portPipeline.MaxPorts {
						return nil, field.Invalid(spec.Child("portBreakouts"), field.OmitValueType{}, fmt.Sprintf("pipeline %s (ports %s) has exceeded maximum ports: %d > %d",
							pipeline, strings.Join(pipelinePortNames[pipeline], ", "), ports, portPipeline.MaxPorts))
					}
				} else {
					return warnings, field.Invalid(profilePath, sw.Spec.Profile, fmt.Sprintf("unknown port pipeline %s reference", pipeline))
				}
			}
		}

		if err := sw.HydrationValidation(ctx, kube, fabricCfg); err != nil {
			return warnings, err
		}
	}

	return warnings, nil
}

// NormalizePortLocator interprets a port locator expire value which could be an empty string (meaning the default
// expire), a duration relative to now such as "10m" or an exact UTC time in the time.DateTime format. It returns
// false if the resulting expire time is already in the past, otherwise it returns the expire time clamped to the
// [now, now+PortLocatorMaxExpire] range and formatted as an exact UTC time.
func NormalizePortLocator(in string) (bool, string, error) {
	now := time.Now().UTC()

	var expire time.Time
	if in == "" {
		expire = now.Add(PortLocatorDefaultExpire)
	} else if dur, err := time.ParseDuration(in); err == nil {
		expire = now.Add(dur)
	} else if exact, err := time.ParseInLocation(time.DateTime, in, time.UTC); err == nil {
		expire = exact.UTC()
	} else {
		return false, "", fmt.Errorf("expected an empty value, a duration such as 10m or an exact UTC time in the %q format: %w", time.DateTime, err)
	}

	if expire.Before(now.Add(1 * time.Second)) {
		return false, "", nil
	}

	if latest := now.Add(PortLocatorMaxExpire); expire.After(latest) {
		expire = latest
	}

	return true, expire.Format(time.DateTime), nil
}
