// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"net/netip"
	"slices"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	VPCInfoExtPrefix = "ext."
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VPCInfoTopology is where a VPCInfo sits in the fabric topology, the same as its VPC or External
type VPCInfoTopology struct {
	// Fabric is the name of the Fabric this VPCInfo belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
	// Domains are the Fabric domains of the VPC or External (if not specified, "default" is used), only the
	// gateways in them get it
	Domains []string `json:"domains,omitempty"`
}

// VPCInfoSpec defines the desired state of VPCInfo.
type VPCInfoSpec struct {
	// Topology is where the VPCInfo sits in the fabric topology
	Topology VPCInfoTopology `json:"topology,omitempty"`
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
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
// +kubebuilder:printcolumn:name="Domains",type=string,JSONPath=`.spec.topology.domains`,priority=0
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
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &VPCInfo{}, &VPCInfoList{})

		return nil
	})
}

func (vpc *VPCInfo) IsReady() bool {
	return vpc.Status.InternalID != ""
}

func (vpc *VPCInfo) Default() {
	if vpc.Namespace == "" {
		vpc.Namespace = kmetav1.NamespaceDefault
	}

	if vpc.Spec.Topology.Fabric == "" {
		vpc.Spec.Topology.Fabric = wiringapi.DefaultFabric
	}
	if len(vpc.Spec.Topology.Domains) == 0 {
		vpc.Spec.Topology.Domains = []string{wiringapi.DefaultFabricDomain}
	}
	slices.Sort(vpc.Spec.Topology.Domains)

	if vpc.Labels == nil {
		vpc.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(vpc.Labels)

	vpc.Labels[wiringapi.ListLabelFabric(vpc.Spec.Topology.Fabric)] = ListLabelValue
	for _, domain := range vpc.Spec.Topology.Domains {
		// validation rejects an empty name, but as a label key it would be refused first with a vaguer error
		if domain != "" {
			vpc.Labels[wiringapi.ListLabelDomain(domain)] = ListLabelValue
		}
	}
}

func (vpc *VPCInfo) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) error {
	if fabricCfg != nil && !fabricCfg.EnableGateway {
		return fmt.Errorf("gateway support is not enabled") //nolint:err113
	}
	if vpc.Namespace != kmetav1.NamespaceDefault {
		return fmt.Errorf("vpcinfo namespace must be %s", kmetav1.NamespaceDefault) //nolint:err113
	}

	if err := wiringapi.CheckFabricExists(ctx, kube, vpc.Namespace, vpc.Spec.Topology.Fabric); err != nil {
		return fmt.Errorf("invalid vpcinfo: %w", err)
	}
	if len(vpc.Spec.Topology.Domains) == 0 {
		return fmt.Errorf("vpcinfo must be in at least one domain") //nolint:err113
	}
	if slices.Contains(vpc.Spec.Topology.Domains, "") {
		return fmt.Errorf("vpcinfo domain names must not be empty") //nolint:err113
	}

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
