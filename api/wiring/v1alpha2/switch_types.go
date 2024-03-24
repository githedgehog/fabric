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

package v1alpha2

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:validation:Enum=spine;server-leaf;border-leaf;mixed-leaf;virtual-edge
// SwitchRole is the role of the switch, could be spine, server-leaf or border-leaf or mixed-leaf
type SwitchRole string

const (
	SwitchRoleSpine       SwitchRole = "spine"
	SwitchRoleServerLeaf  SwitchRole = "server-leaf"
	SwitchRoleBorderLeaf  SwitchRole = "border-leaf"
	SwitchRoleMixedLeaf   SwitchRole = "mixed-leaf"
	SwitchRoleVirtualEdge SwitchRole = "virtual-edge"
)

var SwitchRoles = []SwitchRole{
	SwitchRoleSpine,
	SwitchRoleServerLeaf,
	SwitchRoleBorderLeaf,
}

func (r SwitchRole) IsSpine() bool {
	return r == SwitchRoleSpine
}

func (r SwitchRole) IsLeaf() bool {
	return r == SwitchRoleServerLeaf || r == SwitchRoleBorderLeaf
}

func (r SwitchRole) IsVirtualEdge() bool {
	return r == SwitchRoleVirtualEdge
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

// SwitchSpec defines the desired state of Switch
type SwitchSpec struct {
	// +kubebuilder:validation:Required
	// Role is the role of the switch, could be spine, server-leaf or border-leaf or mixed-leaf
	Role SwitchRole `json:"role,omitempty"`
	// Description is a description of the switch
	Description string `json:"description,omitempty"`
	// Profile is the profile of the switch, name of the SwitchProfile object to be used for this switch, currently not used by the Fabric
	Profile string `json:"profile,omitempty"`
	// Location is the location of the switch, it is used to generate the location UUID and location signature
	Location Location `json:"location,omitempty"`
	// LocationSig is the location signature for the switch
	LocationSig LocationSig `json:"locationSig,omitempty"`
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
}

// SwitchStatus defines the observed state of Switch
type SwitchStatus struct {
	// Applied ApplyStatus `json:"applied,omitempty"`
	// TODO: add port status fields
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=sw
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`,priority=0
// +kubebuilder:printcolumn:name="Descr",type=string,JSONPath=`.spec.description`,priority=0
// +kubebuilder:printcolumn:name="Groups",type=string,JSONPath=`.spec.groups`,priority=0
// +kubebuilder:printcolumn:name="LocationUUID",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/location`,priority=0
// +kubebuilder:printcolumn:name="PortGroups",type=string,JSONPath=`.spec.portGroupSpeeds`,priority=1
// +kubebuilder:printcolumn:name="Breakouts",type=string,JSONPath=`.spec.portBreakouts`,priority=1
// +kubebuilder:printcolumn:name="Rack",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/rack`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// Switch is the Schema for the switches API
//
// All switches should always have 1 labels defined: wiring.githedgehog.com/rack. It represents name of the rack it
// belongs to.
type Switch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is desired state of the switch
	Spec SwitchSpec `json:"spec,omitempty"`
	// Status is the observed state of the switch
	Status SwitchStatus `json:"status,omitempty"`
}

const KindSwitch = "Switch"

//+kubebuilder:object:root=true

// SwitchList contains a list of Switch
type SwitchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Switch `json:"items"`
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

	if sw.Spec.Location.IsEmpty() {
		sw.Spec.Location = Location{Location: fmt.Sprintf("gen--%s--%s", sw.Namespace, sw.Name)}
	}
	if sw.Spec.LocationSig.Sig == "" {
		sw.Spec.LocationSig.Sig = "<undefined>"
	}
	if sw.Spec.LocationSig.UUIDSig == "" {
		sw.Spec.LocationSig.UUIDSig = "<undefined>"
	}

	uuid, _ := sw.Spec.Location.GenerateUUID()
	sw.Labels[LabelLocation] = uuid

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

	for _, vlanNs := range sw.Spec.VLANNamespaces {
		sw.Labels[ListLabelVLANNamespace(vlanNs)] = ListLabelValue
	}

	sort.Strings(sw.Spec.Groups)
	sort.Strings(sw.Spec.VLANNamespaces)
}

func (sw *Switch) Validate(ctx context.Context, kube client.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(sw); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	// TODO validate port group speeds against switch profile

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

		switches := &SwitchList{}
		err = kube.List(ctx, switches, client.MatchingLabels{
			LabelLocation: sw.Labels[LabelLocation],
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get switches") // TODO replace with some internal error to not expose to the user
		}

		for _, other := range switches.Items {
			if sw.Name == other.Name {
				continue
			}

			return nil, errors.Errorf("switch with location %s already exists", sw.Labels[LabelLocation])
		}

		for _, group := range sw.Spec.Groups {
			if group == "" {
				return nil, errors.Errorf("group name cannot be empty")
			}

			sg := &SwitchGroup{}
			err = kube.Get(ctx, types.NamespacedName{Name: group, Namespace: sw.Namespace}, sg)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil, errors.Errorf("switch group %s does not exist", group)
				}

				return nil, errors.Wrapf(err, "failed to get switch group %s", group) // TODO replace with some internal error to not expose to the user
			}
		}
	}

	return nil, nil
}
