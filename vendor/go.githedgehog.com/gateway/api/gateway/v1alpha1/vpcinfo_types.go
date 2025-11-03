// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"net/netip"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	VPCInfoExtPrefix = "ext."
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VPCInfoSpec defines the desired state of VPCInfo.
type VPCInfoSpec struct {
	// Subnets is a map of all subnets in the VPC (incl. CIDRs, VNIs, etc) keyed by the subnet name
	Subnets map[string]*VPCInfoSubnet `json:"subnets,omitempty"`
	// VNI is the VNI for the VPC
	VNI uint32 `json:"vni,omitempty"`
}

type VPCInfoSubnet struct {
	// CIDR is the subnet CIDR block, such as "10.0.0.0/24"
	CIDR string `json:"cidr,omitempty"`
}

// VPCInfoStatus defines the observed state of VPCInfo.
type VPCInfoStatus struct {
	InternalID string `json:"internalID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=vpcinfos,categories=hedgehog;hedgehog-gateway,shortName=gwvpc
// +kubebuilder:printcolumn:name="InternalID",type=string,JSONPath=`.status.internalID`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// VPCInfo is the Schema for the vpcinfos API.
type VPCInfo struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCInfoSpec   `json:"spec,omitempty"`
	Status VPCInfoStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VPCInfoList contains a list of VPCInfo.
type VPCInfoList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []VPCInfo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPCInfo{}, &VPCInfoList{})
}

func (vpc *VPCInfo) IsReady() bool {
	return vpc.Status.InternalID != ""
}

func (vpc *VPCInfo) Default() {
	// TODO add defaulting logic
}

func (vpc *VPCInfo) Validate(_ context.Context, _ kclient.Reader) error {
	if vpc.Spec.VNI == 0 {
		return fmt.Errorf("VPCInfo VNI must be set and non-zero") //nolint:goerr113
	}

	for name, subnet := range vpc.Spec.Subnets {
		if _, err := netip.ParsePrefix(subnet.CIDR); err != nil {
			return fmt.Errorf("invalid CIDR %s for subnet %s: %w", subnet.CIDR, name, err)
		}
	}

	return nil
}
