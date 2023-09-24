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
	"net"

	"github.com/pkg/errors"
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
	Start *string `json:"start,omitempty"`
	End   *string `json:"end,omitempty"`
}

// VPCStatus defines the observed state of VPC
type VPCStatus struct {
	VLAN uint16 `json:"vlan,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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

	vpc.Labels[LabelVPCSubnet] = vpc.Spec.Subnet
}

func (vpc *VPC) Validate() (admission.Warnings, error) {
	cidr, err := iputil.ParseCIDR(vpc.Spec.Subnet)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid subnet %q", vpc.Spec.Subnet)
	}

	if vpc.Spec.DHCP.Enable {
		if vpc.Spec.DHCP.Range != nil {
			if vpc.Spec.DHCP.Range.Start != nil {
				ip := net.ParseIP(*vpc.Spec.DHCP.Range.Start)
				if ip == nil {
					return nil, errors.Wrapf(err, "invalid dhcp range start %q", *vpc.Spec.DHCP.Range.Start)
				}
				if ip.Equal(cidr.Gateway) {
					return nil, errors.Wrapf(err, "dhcp range start %q is equal to gateway", *vpc.Spec.DHCP.Range.Start)
				}
				if !cidr.Subnet.Contains(ip) {
					return nil, errors.Wrapf(err, "dhcp range start %q is not in the subnet", *vpc.Spec.DHCP.Range.Start)
				}
			}
			if vpc.Spec.DHCP.Range.End != nil {
				ip := net.ParseIP(*vpc.Spec.DHCP.Range.End)
				if ip == nil {
					return nil, errors.Wrapf(err, "invalid dhcp range end %q", *vpc.Spec.DHCP.Range.End)
				}
				if ip.Equal(cidr.Gateway) {
					return nil, errors.Wrapf(err, "dhcp range end %q is equal to gateway", *vpc.Spec.DHCP.Range.Start)
				}
				if !cidr.Subnet.Contains(ip) {
					return nil, errors.Wrapf(err, "dhcp range end %q is not in the subnet", *vpc.Spec.DHCP.Range.Start)
				}
			}

			// TODO check start < end
		}
	}

	// TODO check subnet is unique using subnet label on VPC

	return nil, nil
}
