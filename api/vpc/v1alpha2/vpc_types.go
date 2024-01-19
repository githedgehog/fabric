/*
Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha2

import (
	"context"
	"net"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TODO specify gateway explicitly?
// TODO rename VPCSubnet.Subnet to CIDR? or CIDRBlock like in AWS?

// VPCSpec defines the desired state of VPC
type VPCSpec struct {
	Subnets       map[string]*VPCSubnet `json:"subnets,omitempty"`
	IPv4Namespace string                `json:"ipv4Namespace,omitempty"`
	VLANNamespace string                `json:"vlanNamespace,omitempty"`
}

type VPCSubnet struct {
	Subnet string  `json:"subnet,omitempty"`
	DHCP   VPCDHCP `json:"dhcp,omitempty"`
	VLAN   string  `json:"vlan,omitempty"`
}

type VPCDHCP struct {
	Relay  string        `json:"relay,omitempty"`
	Enable bool          `json:"enable,omitempty"`
	Range  *VPCDHCPRange `json:"range,omitempty"`
}

type VPCDHCPRange struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// VPCStatus defines the observed state of VPC
type VPCStatus struct {
	VNI        uint32            `json:"vni,omitempty"` // 1..16_777_215
	SubnetVNIs map[string]uint32 `json:"subnetVNIs,omitempty"`
	// Applied wiringapi.ApplyStatus `json:"applied,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric
// +kubebuilder:printcolumn:name="IPv4NS",type=string,JSONPath=`.spec.ipv4Namespace`,priority=0
// +kubebuilder:printcolumn:name="VLANNS",type=string,JSONPath=`.spec.vlanNamespace`,priority=0
// +kubebuilder:printcolumn:name="Subnets",type=string,JSONPath=`.spec.subnets`,priority=1
// +kubebuilder:printcolumn:name="VNI",type=string,JSONPath=`.status.vni`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// VPC is the Schema for the vpcs API
type VPC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCSpec   `json:"spec,omitempty"`
	Status VPCStatus `json:"status,omitempty"`
}

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

func (vpc *VPC) Default() {
	if vpc.Spec.IPv4Namespace == "" {
		vpc.Spec.IPv4Namespace = "default"
	}
	if vpc.Spec.VLANNamespace == "" {
		vpc.Spec.VLANNamespace = "default"
	}

	if vpc.Labels == nil {
		vpc.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(vpc.Labels)

	vpc.Labels[LabelIPv4NS] = vpc.Spec.IPv4Namespace
	vpc.Labels[LabelVLANNS] = vpc.Spec.VLANNamespace
}

func (vpc *VPC) Validate(ctx context.Context, client validation.Client, reservedSubnets []*net.IPNet) (admission.Warnings, error) {
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
	for subnetName, subnetCfg := range vpc.Spec.Subnets {
		if subnetCfg.Subnet == "" {
			return nil, errors.Errorf("subnet %s: missing subnet", subnetName)
		}

		_, ipNet, err := net.ParseCIDR(subnetCfg.Subnet)
		if err != nil {
			return nil, errors.Wrapf(err, "subnet %s: failed to parse subnet %s", subnetName, subnetCfg.Subnet)
		}

		for _, reserved := range reservedSubnets {
			if reserved.Contains(ipNet.IP) {
				return nil, errors.Errorf("subnet %s: subnet %s is reserved", subnetName, subnetCfg.Subnet)
			}
		}

		if subnetCfg.VLAN == "" {
			return nil, errors.Errorf("subnet %s: vlan is required", subnetName)
		}

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
			// TODO remove after migration to custom DHCP server
			if vpc.Spec.IPv4Namespace != "default" {
				return nil, errors.Errorf("subnet %s: DHCP is not supported for non-default IPv4Namespace yet", subnetName)
			}
			if vpc.Spec.VLANNamespace != "default" {
				return nil, errors.Errorf("subnet %s: DHCP is not supported for non-default VLANNamespace yet", subnetName)
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

	if err := iputil.VerifyNoOverlap(subnets); err != nil {
		return nil, errors.Wrapf(err, "failed to verify no overlap subnets")
	}

	if client != nil {
		// TODO check VLANs
		// TODO Can we rely on Validation webhook for croll VPC subnet? if not - main VPC subnet validation should happen in the VPC controller

		ipNs := &IPv4Namespace{}
		err := client.Get(ctx, types.NamespacedName{Name: vpc.Spec.IPv4Namespace, Namespace: vpc.Namespace}, ipNs)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("IPv4Namespace %s not found", vpc.Spec.IPv4Namespace)
			}
			return nil, errors.Wrapf(err, "failed to get IPv4Namespace %s", vpc.Spec.IPv4Namespace) // TODO replace with some internal error to not expose to the user
		}

		vpcs := &VPCList{}
		err = client.List(ctx, vpcs, map[string]string{
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
	}

	return nil, nil
}
