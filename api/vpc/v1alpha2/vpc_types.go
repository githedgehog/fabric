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
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VPCSpec defines the desired state of VPC
type VPCSpec struct {
	Subnet string  `json:"subnet,omitempty"`
	DHCP   VPCDHCP `json:"dhcp,omitempty"`
}

type VPCDHCP struct {
	Enable bool          `json:"enable,omitempty"`
	Range  *VPCDHCPRange `json:"range,omitempty"`
}

type VPCDHCPRange struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// VPCStatus defines the observed state of VPC
type VPCStatus struct {
	VLAN uint16 `json:"vlan,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric
// +kubebuilder:printcolumn:name="Subnet",type=string,JSONPath=`.spec.subnet`,priority=0
// +kubebuilder:printcolumn:name="VLAN",type=string,JSONPath=`.status.vlan`,priority=0
// +kubebuilder:printcolumn:name="DHCP",type=boolean,JSONPath=`.spec.dhcp.enable`,priority=0
// +kubebuilder:printcolumn:name="Start",type=string,JSONPath=`.spec.dhcp.range.start`,priority=0
// +kubebuilder:printcolumn:name="End",type=string,JSONPath=`.spec.dhcp.range.end`,priority=0
// +kubebuilder:printcolumn:name="Age",type=string,JSONPath=`.metadata.creationTimestamp`,priority=0
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
	cidr, err := iputil.ParseCIDR(vpc.Spec.Subnet)
	if err != nil {
		return // it'll be handled in validation stage
	}

	vpc.Spec.Subnet = cidr.Subnet.String()

	if vpc.Labels == nil {
		vpc.Labels = map[string]string{}
	}

	vpc.Labels[LabelSubnet] = EncodeSubnet(vpc.Spec.Subnet)
}

func (vpc *VPC) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	if len(vpc.Name) > 11 { // TODO should be probably configurable
		return nil, errors.Errorf("name %s is too long, must be <= 11 characters", vpc.Name)
	}

	if vpc.Spec.Subnet == "" {
		return nil, errors.Errorf("subnet is required")
	}

	cidr, err := iputil.ParseCIDR(vpc.Spec.Subnet)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid subnet %s", vpc.Spec.Subnet)
	}

	// TODO to remove this limitation we need to check all VPC subnets for overlaps
	prefixLength, _ := cidr.Subnet.Mask.Size()
	if prefixLength != 24 {
		return nil, errors.Errorf("only /24 subnets currently supported")
	}

	if vpc.Spec.DHCP.Enable {
		if vpc.Spec.DHCP.Range != nil {
			if vpc.Spec.DHCP.Range.Start != "" {
				ip := net.ParseIP(vpc.Spec.DHCP.Range.Start)
				if ip == nil {
					return nil, errors.Errorf("invalid dhcp range start %s", vpc.Spec.DHCP.Range.Start)
				}
				if ip.Equal(cidr.Gateway) {
					return nil, errors.Errorf("dhcp range start %s is equal to gateway", vpc.Spec.DHCP.Range.Start)
				}
				if ip.Equal(cidr.Subnet.IP) {
					return nil, errors.Errorf("dhcp range start %s is equal to subnet", vpc.Spec.DHCP.Range.Start)
				}
				if !cidr.Subnet.Contains(ip) {
					return nil, errors.Errorf("dhcp range start %s is not in the subnet", vpc.Spec.DHCP.Range.Start)
				}
			}
			if vpc.Spec.DHCP.Range.End != "" {
				ip := net.ParseIP(vpc.Spec.DHCP.Range.End)
				if ip == nil {
					return nil, errors.Errorf("invalid dhcp range end %s", vpc.Spec.DHCP.Range.End)
				}
				if ip.Equal(cidr.Gateway) {
					return nil, errors.Errorf("dhcp range end %s is equal to gateway", vpc.Spec.DHCP.Range.End)
				}
				if ip.Equal(cidr.Subnet.IP) {
					return nil, errors.Errorf("dhcp range end %s is equal to subnet", vpc.Spec.DHCP.Range.End)
				}
				if !cidr.Subnet.Contains(ip) {
					return nil, errors.Errorf("dhcp range end %s is not in the subnet", vpc.Spec.DHCP.Range.End)
				}
			}

			// TODO check start < end
		}
	}

	if client != nil {
		vpcs := &VPCList{}
		err := client.List(ctx, vpcs, map[string]string{
			LabelSubnet: EncodeSubnet(vpc.Spec.Subnet),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list VPCs") // TODO replace with some internal error to not expose to the user
		}

		for _, other := range vpcs.Items {
			if vpc.Spec.Subnet == other.Spec.Subnet && vpc.Name != other.Name { // TODO ns?
				return nil, errors.Errorf("subnet %s is already used by other VPC", vpc.Spec.Subnet)
			}
		}
	}

	return nil, nil
}
