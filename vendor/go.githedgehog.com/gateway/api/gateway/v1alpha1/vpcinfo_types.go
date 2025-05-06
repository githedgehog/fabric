// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VPCInfoSpec defines the desired state of VPCInfo.
type VPCInfoSpec struct {
	// Subnets is a map of all subnets in the VPC (incl. CIDRs, VNIs, etc) keyed by the subnet name
	Subnets map[string]*VPCInfoSubnet `json:"subnets,omitempty"`
	// VNI is the VNI for the VPC
	VNI uint32 `json:"vni,omitempty"`
	// VRF (optional) is the VRF name for the VPC, if not specified, predictable VRF name is generated
	VRF string `json:"vrf,omitempty"`
}

type VPCInfoSubnet struct {
	// CIDR is the subnet CIDR block, such as "10.0.0.0/24"
	CIDR string `json:"cidr,omitempty"`
	// Gateway (optional) for the subnet, if not specified, the first IP (e.g. 10.0.0.1) in the subnet is used as the gateway
	Gateway string `json:"gateway,omitempty"`
	// VNI is the VNI for the subnet
	VNI uint32 `json:"vni,omitempty"`
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
	// TODO add validation logic
	return nil
}
