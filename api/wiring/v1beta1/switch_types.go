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
	"net/netip"
	"slices"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

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

// SwitchRedundancy is the switch redundancy configuration which includes name of the redundancy group switch belongs
// to and its type, used both for MCLAG and ESLAG connections. It defines how redundancy will be configured and handled
// on the switch as well as which connection types will be available. If not specified, switch will not be part of any
// redundancy group. If name isn't empty, type must be specified as well and name should be the same as one of the
// SwitchGroup objects.
type SwitchRedundancy struct {
	// Group is the name of the redundancy group switch belongs to
	Group string `json:"group,omitempty"`
	// Type is the type of the redundancy group, could be mclag or eslag
	Type meta.RedundancyType `json:"type,omitempty"`
}

type SwitchBoot struct {
	// Identify switch by serial number
	Serial string `json:"serial,omitempty"`
	// Identify switch by MAC address of the management port
	MAC string `json:"mac,omitempty"`
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
	// Groups is a list of switch groups the switch belongs to
	Groups []string `json:"groups,omitempty"`
	// Redundancy is the switch redundancy configuration including name of the redundancy group switch belongs to and its type, used both for MCLAG and ESLAG connections
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
	// Boot is the boot/provisioning information of the switch
	Boot SwitchBoot `json:"boot,omitempty"`
	// EnableAllPorts is a flag to enable all ports on the switch regardless of them being used or not
	EnableAllPorts bool `json:"enableAllPorts,omitempty"`
	// RoCE is a flag to enable RoCEv2 support on the switch which includes lossless queues and QoS configuration
	RoCE bool `json:"roce,omitempty"`
	// ECMP is the ECMP configuration for the switch
	ECMP SwitchECMP `json:"ecmp,omitempty"`
}

// SwitchECMP is a struct that defines the ECMP configuration for the switch
type SwitchECMP struct {
	// RoCEQPN is a flag to enable RoCE QPN hashing
	RoCEQPN bool `json:"roceQPN,omitempty"`
}

// SwitchStatus defines the observed state of Switch
type SwitchStatus struct {
	// Applied ApplyStatus `json:"applied,omitempty"`
	// TODO: add port status fields
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=sw
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profile`,priority=0
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`,priority=0
// +kubebuilder:printcolumn:name="Descr",type=string,JSONPath=`.spec.description`,priority=0
// +kubebuilder:printcolumn:name="Groups",type=string,JSONPath=`.spec.groups`,priority=0
// +kubebuilder:printcolumn:name="Redundancy",type=string,JSONPath=`.spec.redundancy`,priority=1
// +kubebuilder:printcolumn:name="Boot",type=string,JSONPath=`.spec.boot`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
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

//+kubebuilder:object:root=true

// SwitchList contains a list of Switch
type SwitchList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []Switch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Switch{}, &SwitchList{})
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

	for name, value := range sw.Spec.PortGroupSpeeds {
		sw.Spec.PortGroupSpeeds[name], _ = strings.CutPrefix(value, "SPEED_")
	}

	for name, value := range sw.Spec.PortSpeeds {
		sw.Spec.PortSpeeds[name], _ = strings.CutPrefix(value, "SPEED_")
	}

	if len(sw.Spec.VLANNamespaces) == 0 {
		sw.Spec.VLANNamespaces = []string{"default"}
	}

	if sw.Spec.Redundancy.Group != "" && !slices.Contains(sw.Spec.Groups, sw.Spec.Redundancy.Group) {
		sw.Spec.Groups = append(sw.Spec.Groups, sw.Spec.Redundancy.Group)
	}

	for _, group := range sw.Spec.Groups {
		sw.Labels[ListLabelSwitchGroup(group)] = ListLabelValue
	}

	for _, vlanNs := range sw.Spec.VLANNamespaces {
		sw.Labels[ListLabelVLANNamespace(vlanNs)] = ListLabelValue
	}

	sort.Strings(sw.Spec.Groups)
	sort.Strings(sw.Spec.VLANNamespaces)

	sw.Labels[LabelProfile] = sw.Spec.Profile
}

func (sw *Switch) HydrationValidation(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) error {
	if kube == nil {
		return errors.Errorf("kube client is required for hydration validations")
	}

	switches := &SwitchList{}
	if err := kube.List(ctx, switches); err != nil {
		return errors.Wrapf(err, "failed to list switches for hydration validation")
	}
	// TODO: collect gateways as well for VTEP and protocol IP uniqueness checks
	// (cannot be done now as gateway webhook would create a circular dependency)

	leafASNs := map[uint32]bool{}
	VTEPs := map[string]bool{}
	protocolIPs := map[string]bool{}
	mgmtIPs := map[netip.Addr]bool{}
	var mclagPeer *Switch
	vtepSubnet, err := netip.ParsePrefix(fabricCfg.VTEPSubnet)
	if err != nil {
		return errors.Wrapf(err, "failed to parse Fabric VTEP subnet %s", fabricCfg.VTEPSubnet)
	}
	protocolSubnet, err := netip.ParsePrefix(fabricCfg.ProtocolSubnet)
	if err != nil {
		return errors.Wrapf(err, "failed to parse Fabric protocol subnet %s", fabricCfg.ProtocolSubnet)
	}
	controlVIP, err := netip.ParsePrefix(fabricCfg.ControlVIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse Fabric control VIP %s", fabricCfg.ControlVIP)
	}
	mgmtIPs[controlVIP.Addr()] = true

	for _, other := range switches.Items {
		if other.Name == sw.Name && other.Namespace == sw.Namespace {
			continue
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
		if sw.Spec.Redundancy.Type == meta.RedundancyTypeMCLAG && other.Spec.Redundancy.Type == meta.RedundancyTypeMCLAG &&
			other.Spec.Redundancy.Group == sw.Spec.Redundancy.Group {
			mclagPeer = &other
		}
	}

	if sw.Spec.IP != "" {
		swIP, err := netip.ParsePrefix(sw.Spec.IP)
		if err != nil {
			return errors.Wrapf(err, "parsing switch %s IP %s", sw.Name, sw.Spec.IP)
		}

		// FIXME: management subnet validation is not possible as it's defined in fabricator
		if _, exist := mgmtIPs[swIP.Addr()]; exist {
			return errors.Errorf("switch %s (management) IP %s is already in use", sw.Name, swIP) //nolint:goerr113
		}
	}

	if sw.Spec.ProtocolIP != "" {
		swProtoIP, err := netip.ParsePrefix(sw.Spec.ProtocolIP)
		if err != nil {
			return errors.Wrapf(err, "parsing switch %s protocol IP %s", sw.Name, sw.Spec.ProtocolIP)
		}
		if swProtoIP.Bits() != 32 {
			return errors.Errorf("switch %s protocol IP %s must be a /32", sw.Name, swProtoIP) //nolint:goerr113
		}

		if !protocolSubnet.Contains(swProtoIP.Addr()) {
			return errors.Errorf("switch %s protocol IP %s is not in the protocol subnet %s", sw.Name, swProtoIP, protocolSubnet) //nolint:goerr113
		}

		if _, exist := protocolIPs[sw.Spec.ProtocolIP]; exist {
			return errors.Errorf("switch %s protocol IP %s is already in use", sw.Name, swProtoIP) //nolint:goerr113
		}
	}

	// check leaf ASN uniqueness, with the exception of MCLAG peers which should have the same ASN
	if sw.Spec.Role.IsLeaf() {
		if sw.Spec.Redundancy.Type == meta.RedundancyTypeMCLAG {
			if mclagPeer != nil {
				if mclagPeer.Spec.ASN != sw.Spec.ASN {
					return errors.Errorf("mclag peers should have same ASNs: %s and %s", sw.Name, mclagPeer.Name) //nolint:goerr113
				}
			} else {
				if _, exist := leafASNs[sw.Spec.ASN]; exist {
					return errors.Errorf("leaf %s ASN %d is already in use", sw.Name, sw.Spec.ASN) //nolint:goerr113
				}
			}
		} else if _, exist := leafASNs[sw.Spec.ASN]; exist {
			return errors.Errorf("leaf %s ASN %d is already in use", sw.Name, sw.Spec.ASN) //nolint:goerr113
		}
		// also check if it's within the fabric leaf ASN range
		if sw.Spec.ASN < fabricCfg.LeafASNStart || sw.Spec.ASN > fabricCfg.LeafASNEnd {
			return errors.Errorf("leaf %s ASN %d is not within the fabric leaf ASN range %d-%d", sw.Name, sw.Spec.ASN, fabricCfg.LeafASNStart, fabricCfg.LeafASNEnd) //nolint:goerr113
		}
	}

	// spine ASN consistency check
	if sw.Spec.Role.IsSpine() && sw.Spec.ASN != fabricCfg.SpineASN {
		return errors.Errorf("spine %s ASN %d is not the expected spine ASN %d", sw.Name, sw.Spec.ASN, fabricCfg.SpineASN) //nolint:goerr113
	}

	// leaf vtep IP uniqueness / consistency for mclag peers
	if sw.Spec.Role.IsLeaf() {
		swVTEPIP, err := netip.ParsePrefix(sw.Spec.VTEPIP)
		if err != nil {
			return errors.Wrapf(err, "parsing switch %s VTEP IP %s", sw.Name, sw.Spec.VTEPIP)
		}
		if swVTEPIP.Bits() != 32 {
			return errors.Errorf("switch %s VTEP IP %s must be a /32", sw.Name, swVTEPIP) //nolint:goerr113
		}

		if !vtepSubnet.Contains(swVTEPIP.Addr()) {
			return errors.Errorf("switch %s VTEP IP %s is not in the VTEP subnet %s", sw.Name, swVTEPIP, vtepSubnet) //nolint:goerr113
		}

		if sw.Spec.Redundancy.Type == meta.RedundancyTypeMCLAG {
			if mclagPeer != nil {
				if mclagPeer.Spec.VTEPIP != sw.Spec.VTEPIP {
					return errors.Errorf("mclag peers should have same VTEP IPs: %s and %s", sw.Name, mclagPeer.Name) //nolint:goerr113
				}
			} else {
				if _, exist := VTEPs[sw.Spec.VTEPIP]; exist {
					return errors.Errorf("switch %s VTEP IP %s is already in use", sw.Name, swVTEPIP) //nolint:goerr113
				}
			}
		} else if _, exist := VTEPs[sw.Spec.VTEPIP]; exist {
			return errors.Errorf("switch %s VTEP IP %s is already in use", sw.Name, swVTEPIP) //nolint:goerr113
		}
	}

	return nil
}

func (sw *Switch) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(sw); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	if len(sw.Spec.VLANNamespaces) == 0 {
		return nil, errors.Errorf("at least one VLAN namespace required")
	}
	if sw.Spec.ASN == 0 {
		return nil, errors.Errorf("ASN is required")
	}
	if sw.Spec.IP == "" {
		return nil, errors.Errorf("IP is required")
	}
	if sw.Spec.ProtocolIP == "" {
		return nil, errors.Errorf("protocol IP is required")
	}
	if sw.Spec.Role.IsLeaf() && fabricCfg != nil && fabricCfg.FabricMode == meta.FabricModeSpineLeaf && sw.Spec.VTEPIP == "" {
		return nil, errors.Errorf("VTEP IP is required for leaf switches in spine-leaf mode")
	}
	if sw.Spec.Role.IsSpine() && sw.Spec.VTEPIP != "" {
		return nil, errors.Errorf("VTEP IP is not allowed for spine switches")
	}

	if sw.Spec.Profile == "" {
		return nil, errors.Errorf("profile is required")
	}

	if !slices.Contains(meta.RedundancyTypes, sw.Spec.Redundancy.Type) {
		return nil, errors.Errorf("invalid redundancy type")
	}
	if sw.Spec.Redundancy.Group != "" && sw.Spec.Redundancy.Type == meta.RedundancyTypeNone {
		return nil, errors.Errorf("redundancy group specified without type")
	}
	if sw.Spec.Redundancy.Group == "" && sw.Spec.Redundancy.Type != meta.RedundancyTypeNone {
		return nil, errors.Errorf("redundancy type specified without group")
	}

	if kube != nil {
		namespaces := &VLANNamespaceList{}
		err := kube.List(ctx, namespaces)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get VLAN namespaces") // TODO replace with some internal error to not expose to the user
		}

		ranges := []meta.VLANRange{}

		for _, ns := range sw.Spec.VLANNamespaces {
			found := false
			for _, other := range namespaces.Items {
				if ns == other.Name {
					found = true
					ranges = append(ranges, other.Spec.Ranges...)

					break
				}
			}
			if !found {
				return nil, errors.Errorf("specified VLANNamespace %s does not exist", ns)
			}
		}

		if err := meta.CheckVLANRangesOverlap(ranges); err != nil {
			return nil, errors.Wrapf(err, "invalid VLANNamespaces")
		}

		for _, group := range sw.Spec.Groups {
			if group == "" {
				return nil, errors.Errorf("group name cannot be empty")
			}

			sg := &SwitchGroup{}
			err = kube.Get(ctx, ktypes.NamespacedName{Name: group, Namespace: sw.Namespace}, sg)
			if err != nil {
				if kapierrors.IsNotFound(err) {
					return nil, errors.Errorf("switch group %s does not exist", group)
				}

				return nil, errors.Wrapf(err, "failed to get switch group %s", group) // TODO replace with some internal error to not expose to the user
			}
		}

		sp := &SwitchProfile{}
		err = kube.Get(ctx, ktypes.NamespacedName{Name: sw.Spec.Profile, Namespace: sw.Namespace}, sp)
		if err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, errors.Errorf("switch profile %s does not exist", sw.Spec.Profile)
			}

			return nil, errors.Wrapf(err, "failed to get switch profile %s", sw.Spec.Profile) // TODO replace with some internal error to not expose to the user
		}

		for name, speed := range sw.Spec.PortGroupSpeeds {
			group, exists := sp.Spec.PortGroups[name]
			if !exists {
				return nil, errors.Errorf("port group %s not found in switch profile", name)
			}

			profile, exists := sp.Spec.PortProfiles[group.Profile]
			if !exists {
				return nil, errors.Errorf("port profile %s for group %s not found in switch profile", group.Profile, name)
			}

			if profile.Speed == nil {
				return nil, errors.Errorf("port profile %s for group %s has no supported speeds", group.Profile, name)
			}

			if !slices.Contains(profile.Speed.Supported, speed) {
				return nil, errors.Errorf("port group %s does not support specified speed %s", name, speed)
			}
		}

		for name, speed := range sw.Spec.PortSpeeds {
			port, exists := sp.Spec.Ports[name]
			if !exists {
				return nil, errors.Errorf("port %s not found in switch profile", name)
			}

			profile, exists := sp.Spec.PortProfiles[port.Profile]
			if !exists {
				return nil, errors.Errorf("port profile %s for port %s not found in switch profile", port.Profile, name)
			}

			if profile.Speed == nil {
				return nil, errors.Errorf("port profile %s for port %s has no supported speeds", port.Profile, name)
			}

			if !slices.Contains(profile.Speed.Supported, speed) {
				return nil, errors.Errorf("port %s does not support specified speed %s", name, speed)
			}
		}

		for name, breakout := range sw.Spec.PortBreakouts {
			port, exists := sp.Spec.Ports[name]
			if !exists {
				return nil, errors.Errorf("port %s not found in switch profile", name)
			}

			profile, exists := sp.Spec.PortProfiles[port.Profile]
			if !exists {
				return nil, errors.Errorf("port profile %s for port %s not found in switch profile", port.Profile, name)
			}

			if profile.Breakout == nil {
				return nil, errors.Errorf("port profile %s for port %s has no supported breakouts", port.Profile, name)
			}

			if _, exists := profile.Breakout.Supported[breakout]; !exists {
				return nil, errors.Errorf("port %s does not support specified breakout %s", name, breakout)
			}
		}

		autoNegAllowed, _, err := sp.Spec.GetAutoNegsDefaultsFor(&sw.Spec)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get auto negotiation defaults")
		}

		for name := range sw.Spec.PortAutoNegs {
			if !autoNegAllowed[name] {
				return nil, errors.Errorf("port %s does not support configuring auto negotiation", name)
			}
		}

		if sw.Spec.RoCE && !sp.Spec.Features.RoCE {
			return nil, errors.Errorf("RoCEv2 is not supported on switch profile %s", sw.Spec.Profile)
		}

		if sw.Spec.ECMP.RoCEQPN && !sp.Spec.Features.ECMPRoCEQPN {
			return nil, errors.Errorf("ECMP RoCE QPN hashing is not supported on switch profile %s", sw.Spec.Profile)
		}

		switch sw.Spec.Redundancy.Type {
		case meta.RedundancyTypeNone:
			// No redundancy, nothing to check
		case meta.RedundancyTypeMCLAG:
			if !sp.Spec.Features.MCLAG {
				return nil, errors.Errorf("MCLAG is not supported on switch profile %s", sw.Spec.Profile)
			}
		case meta.RedundancyTypeESLAG:
			if !sp.Spec.Features.ESLAG {
				return nil, errors.Errorf("ESLAG is not supported on switch profile %s", sw.Spec.Profile)
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
					return nil, errors.Errorf("port %s has no group or profile", name)
				}
				profile, ok := sp.Spec.PortProfiles[port.Profile]
				if !ok {
					return nil, errors.Errorf("port %s has invalid profile %s", name, port.Profile)
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
						return nil, errors.Errorf("port %s has invalid breakout mode %s", name, mode)
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
			return nil, errors.Errorf("switch %s has exceeded maximum ports: %d > %d", sp.Name, totalPorts, sp.Spec.MaxPorts)
		}

		if sp.Spec.Pipelines != nil {
			for pipeline, ports := range pipelinePorts {
				if portPipeline, ok := sp.Spec.Pipelines[pipeline]; ok {
					if ports > portPipeline.MaxPorts {
						return nil, errors.Errorf("pipeline %s (ports %s) has exceeded maximum ports: %d > %d",
							pipeline, strings.Join(pipelinePortNames[pipeline], ", "), ports, portPipeline.MaxPorts)
					}
				} else {
					return nil, errors.Errorf("unknown port pipeline %s reference", pipeline)
				}
			}
		}

		if err := sw.HydrationValidation(ctx, kube, fabricCfg); err != nil {
			return nil, errors.Wrapf(err, "failed hydration validation")
		}
	}

	return nil, nil
}
