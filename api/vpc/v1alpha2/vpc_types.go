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
	"net"
	"strconv"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TODO specify gateway explicitly?
// TODO rename VPCSubnet.Subnet to CIDR? or CIDRBlock like in AWS?

// VPCSpec defines the desired state of VPC.
// At least one subnet is required.
type VPCSpec struct {
	// Subnets is the list of VPC subnets to configure
	Subnets map[string]*VPCSubnet `json:"subnets,omitempty"`
	// IPv4Namespace is the name of the IPv4Namespace this VPC belongs to (if not specified, "default" is used)
	IPv4Namespace string `json:"ipv4Namespace,omitempty"`
	// VLANNamespace is the name of the VLANNamespace this VPC belongs to (if not specified, "default" is used)
	VLANNamespace string `json:"vlanNamespace,omitempty"`
	// DefaultIsolated sets default bahivour for isolated mode for the subnets (disabled by default)
	DefaultIsolated bool `json:"defaultIsolated,omitempty"`
	// DefaultRestricted sets default bahivour for restricted mode for the subnets (disabled by default)
	DefaultRestricted bool `json:"defaultRestricted,omitempty"`
	// Permit defines a list of the access policies between the subnets within the VPC - each policy is a list of subnets that have access to each other.
	// It's applied on top of the subnet isolation flag and if subnet isn't isolated it's not required to have it in a permit list while if vpc is marked
	// as isolated it's required to have it in a permit list to have access to other subnets.
	Permit [][]string `json:"permit,omitempty"`
}

// VPCSubnet defines the VPC subnet configuration
type VPCSubnet struct {
	// Subnet is the subnet CIDR block, such as "10.0.0.0/24", should belong to the IPv4Namespace and be unique within the namespace
	Subnet string `json:"subnet,omitempty"`
	// DHCP is the on-demand DHCP configuration for the subnet
	DHCP VPCDHCP `json:"dhcp,omitempty"`
	// VLAN is the VLAN ID for the subnet, should belong to the VLANNamespace and be unique within the namespace
	VLAN string `json:"vlan,omitempty"`
	// Isolated is the flag to enable isolated mode for the subnet which means no access to and from the other subnets within the VPC
	Isolated *bool `json:"isolated,omitempty"`
	// Restricted is the flag to enable restricted mode for the subnet which means no access between hosts within the subnet itself
	Restricted *bool `json:"restricted,omitempty"`
}

// VPCDHCP defines the on-demand DHCP configuration for the subnet
type VPCDHCP struct {
	// Relay is the DHCP relay IP address, if specified, DHCP server will be disabled
	Relay string `json:"relay,omitempty"`
	// Enable enables DHCP server for the subnet
	Enable bool `json:"enable,omitempty"`
	// Range (optional) is the DHCP range for the subnet if DHCP server is enabled
	Range *VPCDHCPRange `json:"range,omitempty"`
	// PXEURL (optional) to identify the pxe server to use to boot hosts connected to this segment such as http://10.10.10.99/bootfilename or tftp://10.10.10.99/bootfilename, http query strings are not supported
	PXEURL string `json:"pxeURL,omitempty"`
}

// VPCDHCPRange defines the DHCP range for the subnet if DHCP server is enabled
type VPCDHCPRange struct {
	// Start is the start IP address of the DHCP range
	Start string `json:"start,omitempty"`
	// End is the end IP address of the DHCP range
	End string `json:"end,omitempty"`
}

// VPCStatus defines the observed state of VPC
type VPCStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric
// +kubebuilder:printcolumn:name="IPv4NS",type=string,JSONPath=`.spec.ipv4Namespace`,priority=0
// +kubebuilder:printcolumn:name="VLANNS",type=string,JSONPath=`.spec.vlanNamespace`,priority=0
// +kubebuilder:printcolumn:name="Subnets",type=string,JSONPath=`.spec.subnets`,priority=1
// +kubebuilder:printcolumn:name="VNI",type=string,JSONPath=`.status.vni`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// VPC is Virtual Private Cloud, similar to the public cloud VPC it provides an isolated private network for the
// resources with support for multiple subnets each with user-provided VLANs and on-demand DHCP.
type VPC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the VPC
	Spec VPCSpec `json:"spec,omitempty"`
	// Status is the observed state of the VPC
	Status VPCStatus `json:"status,omitempty"`
}

const KindVPC = "VPC"

//+kubebuilder:object:root=true

// VPCList contains a list of VPC
type VPCList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPC `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPC{}, &VPCList{})
}

var (
	_ meta.Object     = (*VPC)(nil)
	_ meta.ObjectList = (*VPCList)(nil)
)

func (vpcList *VPCList) GetItems() []meta.Object {
	items := make([]meta.Object, len(vpcList.Items))
	for i := range vpcList.Items {
		items[i] = &vpcList.Items[i]
	}

	return items
}

func (vpc *VPCSpec) IsSubnetIsolated(subnetName string) bool {
	if subnet, ok := vpc.Subnets[subnetName]; ok && subnet.Isolated != nil {
		return *subnet.Isolated
	}

	return vpc.DefaultIsolated
}

func (vpc *VPCSpec) IsSubnetRestricted(subnetName string) bool {
	if subnet, ok := vpc.Subnets[subnetName]; ok && subnet.Restricted != nil {
		return *subnet.Restricted
	}

	return vpc.DefaultRestricted
}

func (vpc *VPC) Default() {
	meta.DefaultObjectMetadata(vpc)

	if vpc.Spec.IPv4Namespace == "" {
		vpc.Spec.IPv4Namespace = DefaultIPv4Namespace
	}
	if vpc.Spec.VLANNamespace == "" {
		vpc.Spec.VLANNamespace = wiringapi.DefaultVLANNamespace
	}

	if vpc.Labels == nil {
		vpc.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(vpc.Labels)

	vpc.Labels[LabelIPv4NS] = vpc.Spec.IPv4Namespace
	vpc.Labels[LabelVLANNS] = vpc.Spec.VLANNamespace
}

func (vpc *VPC) Validate(ctx context.Context, kube client.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(vpc); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	if len(vpc.Name) > 11 {
		return nil, errors.Errorf("name %s is too long, must be <= 11 characters", vpc.Name)
	}
	if vpc.Spec.IPv4Namespace == "" {
		return nil, errors.Errorf("ipv4Namespace is required")
	}
	if vpc.Spec.VLANNamespace == "" {
		return nil, errors.Errorf("vlanNamespace is required")
	}
	if len(vpc.Spec.Subnets) == 0 {
		return nil, errors.Errorf("at least one subnet is required")
	}
	if len(vpc.Spec.Subnets) > 20 {
		return nil, errors.Errorf("too many subnets, max is 10")
	}

	subnets := []*net.IPNet{}
	vlans := map[string]bool{}
	for subnetName, subnetCfg := range vpc.Spec.Subnets {
		if subnetCfg.Subnet == "" {
			return nil, errors.Errorf("subnet %s: missing subnet", subnetName)
		}

		_, ipNet, err := net.ParseCIDR(subnetCfg.Subnet)
		if err != nil {
			return nil, errors.Wrapf(err, "subnet %s: failed to parse subnet %s", subnetName, subnetCfg.Subnet)
		}

		if fabricCfg != nil {
			for _, reserved := range fabricCfg.ParsedReservedSubnets() {
				if reserved.Contains(ipNet.IP) {
					return nil, errors.Errorf("subnet %s: subnet %s is reserved", subnetName, subnetCfg.Subnet)
				}
			}
		}

		if subnetCfg.VLAN == "" {
			return nil, errors.Errorf("subnet %s: vlan is required", subnetName)
		}
		vlans[subnetCfg.VLAN] = true

		subnets = append(subnets, ipNet)

		if subnetCfg.DHCP.Relay != "" && subnetCfg.DHCP.Enable {
			return nil, errors.Errorf("subnet %s: dhcp relay and dhcp server cannot be enabled at the same time", subnetName)
		}

		if subnetCfg.DHCP.Relay != "" {
			_, _, err := net.ParseCIDR(subnetCfg.DHCP.Relay)
			if err != nil {
				return nil, errors.Wrapf(err, "subnet %s: failed to parse dhcp relay %s", subnetName, subnetCfg.DHCP.Relay)
			}
		}

		if subnetCfg.DHCP.Enable {
			if fabricCfg != nil && !fabricCfg.DHCPMode.IsMultiNSDHCP() {
				if vpc.Spec.IPv4Namespace != DefaultIPv4Namespace {
					return nil, errors.Errorf("subnet %s: DHCP is not supported for non-default IPv4Namespace in the current Fabric config", subnetName)
				}
				if vpc.Spec.VLANNamespace != wiringapi.DefaultVLANNamespace {
					return nil, errors.Errorf("subnet %s: DHCP is not supported for non-default VLANNamespace in the current Fabric config", subnetName)
				}
			}

			if subnetCfg.DHCP.Range != nil {
				if subnetCfg.DHCP.Range.Start != "" {
					ip := net.ParseIP(subnetCfg.DHCP.Range.Start)
					if ip == nil {
						return nil, errors.Errorf("subnet %s: invalid dhcp range start %s", subnetName, subnetCfg.DHCP.Range.Start)
					}
					if ip.Equal(ipNet.IP) {
						return nil, errors.Errorf("subnet %s: dhcp range start %s is equal to subnet", subnetName, subnetCfg.DHCP.Range.Start)
					}
					if !ipNet.Contains(ip) {
						return nil, errors.Errorf("subnet %s: dhcp range start %s is not in the subnet", subnetName, subnetCfg.DHCP.Range.Start)
					}
				}
				if subnetCfg.DHCP.Range.End != "" {
					ip := net.ParseIP(subnetCfg.DHCP.Range.End)
					if ip == nil {
						return nil, errors.Errorf("subnet %s: invalid dhcp range end %s", subnetName, subnetCfg.DHCP.Range.End)
					}
					if ip.Equal(ipNet.IP) {
						return nil, errors.Errorf("subnet %s: dhcp range end %s is equal to subnet", subnetName, subnetCfg.DHCP.Range.End)
					}
					if !ipNet.Contains(ip) {
						return nil, errors.Errorf("subnet %s: dhcp range end %s is not in the subnet", subnetName, subnetCfg.DHCP.Range.End)
					}
				}

				// TODO check start < end
			}
		} else {
			if subnetCfg.DHCP.Range != nil && (subnetCfg.DHCP.Range.Start != "" || subnetCfg.DHCP.Range.End != "") {
				return nil, errors.Errorf("dhcp range start or end is set but dhcp is disabled")
			}
		}
	}

	if len(vlans) != len(vpc.Spec.Subnets) {
		return nil, errors.Errorf("duplicate subnet VLANs")
	}

	if err := iputil.VerifyNoOverlap(subnets); err != nil {
		return nil, errors.Wrapf(err, "failed to verify no overlap subnets")
	}

	for permitIdx, permit := range vpc.Spec.Permit {
		if len(permit) < 2 {
			return nil, errors.Errorf("each permit policy must have at least 2 subnets in it")
		}

		subnets := map[string]bool{}
		for _, subnetName := range permit {
			if _, ok := vpc.Spec.Subnets[subnetName]; !ok {
				return nil, errors.Errorf("permit policy #%d: subnet %s not found", permitIdx, subnetName)
			}

			subnets[subnetName] = true
		}

		if len(subnets) != len(permit) {
			return nil, errors.Errorf("permit policy #%d: duplicate subnets", permitIdx)
		}
	}

	if kube != nil {
		// TODO Can we rely on Validation webhook for cross VPC subnet? if not - main VPC subnet validation should happen in the VPC controller

		ipNs := &IPv4Namespace{}
		err := kube.Get(ctx, types.NamespacedName{Name: vpc.Spec.IPv4Namespace, Namespace: vpc.Namespace}, ipNs)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("IPv4Namespace %s not found", vpc.Spec.IPv4Namespace)
			}

			return nil, errors.Wrapf(err, "failed to get IPv4Namespace %s", vpc.Spec.IPv4Namespace) // TODO replace with some internal error to not expose to the user
		}

		vlanNs := &wiringapi.VLANNamespace{}
		err = kube.Get(ctx, types.NamespacedName{Name: vpc.Spec.VLANNamespace, Namespace: vpc.Namespace}, vlanNs)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("VLANNamespace %s not found", vpc.Spec.VLANNamespace)
			}

			return nil, errors.Wrapf(err, "failed to get VLANNamespace %s", vpc.Spec.VLANNamespace) // TODO replace with some internal error to not expose to the user
		}

		for subnetName, subnetCfg := range vpc.Spec.Subnets {
			_, vpcSubnet, err := net.ParseCIDR(subnetCfg.Subnet)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse vpc subnet %s", subnetCfg.Subnet)
			}

			ok := false
			for _, ipNsSubnetCfg := range ipNs.Spec.Subnets {
				_, ipNsSubnet, err := net.ParseCIDR(ipNsSubnetCfg)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse IPv4Namespace %s subnet %s", vpc.Spec.IPv4Namespace, ipNsSubnetCfg)
				}

				if ipNsSubnet.Contains(vpcSubnet.IP) {
					ok = true

					break
				}
			}

			if !ok {
				return nil, errors.Errorf("vpc subnet %s (%s) doesn't belong to the IPv4Namespace %s", subnetName, subnetCfg.Subnet, vpc.Spec.IPv4Namespace)
			}

			vlanRaw, err := strconv.ParseUint(subnetCfg.VLAN, 10, 16)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse subnet %s (%s) VLAN %s", subnetName, subnetCfg.Subnet, subnetCfg.VLAN)
			}
			if !vlanNs.Spec.Contains(uint16(vlanRaw)) {
				return nil, errors.Errorf("vpc subnet %s (%s) vlan %s doesn't belong to the VLANNamespace %s", subnetName, subnetCfg.Subnet, subnetCfg.VLAN, vpc.Spec.VLANNamespace)
			}
		}

		vpcs := &VPCList{}
		err = kube.List(ctx, vpcs, client.MatchingLabels{
			LabelIPv4NS: vpc.Spec.IPv4Namespace,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list VPCs") // TODO replace with some internal error to not expose to the user
		}

		for _, other := range vpcs.Items {
			if other.Name == vpc.Name {
				continue
			}
			if other.Spec.IPv4Namespace != vpc.Spec.IPv4Namespace {
				continue
			}

			for _, otherSubnet := range other.Spec.Subnets {
				_, otherNet, err := net.ParseCIDR(otherSubnet.Subnet)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse subnet %s", otherSubnet.Subnet)
				}

				for _, subnet := range subnets {
					if subnet.Contains(otherNet.IP) {
						return nil, errors.Errorf("subnet %s overlaps with subnet %s of VPC %s", subnet.String(), otherSubnet.Subnet, other.Name)
					}
				}
			}
		}

		vpcs = &VPCList{}
		err = kube.List(ctx, vpcs, client.MatchingLabels{
			LabelVLANNS: vpc.Spec.VLANNamespace,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list VPCs") // TODO replace with some internal error to not expose to the user
		}

		for _, other := range vpcs.Items {
			if other.Name == vpc.Name {
				continue
			}
			if other.Spec.VLANNamespace != vpc.Spec.VLANNamespace {
				continue
			}

			for _, otherSubnet := range other.Spec.Subnets {
				for _, subnet := range vpc.Spec.Subnets {
					if subnet.VLAN == otherSubnet.VLAN {
						return nil, errors.Errorf("vlan %s is already used by other VPC", subnet.VLAN)
					}
				}
			}
		}
	}

	return nil, nil
}
