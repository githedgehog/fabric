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
	"slices"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VPCPeeringTopology is where a VPCPeering sits in the fabric topology
type VPCPeeringTopology struct {
	// Fabric is the name of the Fabric this VPCPeering belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
}

// VPCPeeringSpec defines the desired state of VPCPeering
type VPCPeeringSpec struct {
	// Topology is where the VPCPeering sits in the fabric topology
	Topology VPCPeeringTopology `json:"topology,omitempty"`
	// Deprecated and no longer supported
	Remote string `json:"remote,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// Permit defines a list of the peering policies - which VPC subnets will have access to the peer VPC subnets.
	Permit []map[string]VPCPeer `json:"permit,omitempty"`
}

type VPCPeer struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// Subnets is the list of subnets to advertise from current VPC to the peer VPC
	Subnets []string `json:"subnets,omitempty"`
}

// VPCPeeringStatus defines the observed state of VPCPeering
type VPCPeeringStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric,shortName=vpcpeer
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
// +kubebuilder:printcolumn:name="VPC1",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/vpc1`,priority=0
// +kubebuilder:printcolumn:name="VPC2",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/vpc2`,priority=0
// +kubebuilder:printcolumn:name="Remote",type=string,JSONPath=`.spec.remote`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// VPCPeering represents a peering between two VPCs with corresponding filtering rules.
// Minimal example of the VPC peering showing vpc-1 to vpc-2 peering with all subnets allowed:
//
//	spec:
//	  permit:
//	  - vpc-1: {}
//	    vpc-2: {}
type VPCPeering struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the VPCPeering
	Spec VPCPeeringSpec `json:"spec,omitempty"`
	// Status is the observed state of the VPCPeering
	Status VPCPeeringStatus `json:"status,omitempty"`
}

const KindVPCPeering = "VPCPeering"

// +kubebuilder:object:root=true

// VPCPeeringList contains a list of VPCPeering
type VPCPeeringList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []VPCPeering `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &VPCPeering{}, &VPCPeeringList{})

		return nil
	})
}

var (
	_ meta.Object     = (*VPCPeering)(nil)
	_ meta.ObjectList = (*VPCPeeringList)(nil)
)

func (peeringList *VPCPeeringList) GetItems() []meta.Object {
	items := make([]meta.Object, len(peeringList.Items))
	for i := range peeringList.Items {
		items[i] = &peeringList.Items[i]
	}

	return items
}

func (s *VPCPeeringSpec) VPCs() (string, string, error) {
	vpcs := []string{}
	for idx, permit := range s.Permit {
		if len(permit) != 2 {
			return "", "", field.Invalid(field.NewPath("spec", "permit").Index(idx), field.OmitValueType{}, "must have exactly 2 VPCs")
		}

		for vpc := range permit {
			if !slices.Contains(vpcs, vpc) {
				vpcs = append(vpcs, vpc)
			}
		}
	}

	if len(vpcs) != 2 {
		return "", "", field.Invalid(field.NewPath("spec", "permit"), field.OmitValueType{}, "VPCPeering must have exactly 2 VPCs")
	}

	slices.Sort(vpcs)

	return vpcs[0], vpcs[1], nil
}

func (peering *VPCPeering) Default() {
	meta.DefaultObjectMetadata(peering)

	if peering.Spec.Topology.Fabric == "" {
		peering.Spec.Topology.Fabric = wiringapi.DefaultFabric
	}

	if peering.Labels == nil {
		peering.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(peering.Labels)

	peering.Labels[wiringapi.ListLabelFabric(peering.Spec.Topology.Fabric)] = ListLabelValue

	vpc1, vpc2, err := peering.Spec.VPCs()
	if err != nil {
		return // it'll be handled in validation stage
	}

	peering.Labels[ListLabelVPC(vpc1)] = ListLabelValue
	peering.Labels[ListLabelVPC(vpc2)] = ListLabelValue
	peering.Labels[LabelVPC1] = vpc1
	peering.Labels[LabelVPC2] = vpc2
}

func (peering *VPCPeering) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(peering); err != nil {
		return nil, err //nolint:wrapcheck
	}

	spec := field.NewPath("spec")

	if err := wiringapi.CheckFabricExists(ctx, kube, peering.Namespace, peering.Spec.Topology.Fabric); err != nil {
		return nil, field.Invalid(spec.Child("topology", "fabric"), peering.Spec.Topology.Fabric, err.Error())
	}

	if fabricCfg != nil && fabricCfg.VPCPeeringDisabled {
		return nil, field.Forbidden(spec, "vpc peering is not allowed")
	}

	if peering.Spec.Remote != "" {
		return nil, field.Forbidden(spec.Child("remote"), "deprecated and no longer supported")
	}

	vpc1Name, vpc2Name, err := peering.Spec.VPCs()
	if err != nil {
		return nil, err
	}

	if kube != nil {
		if fabricCfg != nil && fabricCfg.ExtraValidators.Peering != nil {
			if err := fabricCfg.ExtraValidators.Peering(ctx, kube, peering); err != nil {
				return nil, err //nolint:wrapcheck
			}
		}

		peeringFabric := peering.Spec.Topology.Fabric
		ipv4Namespaces := []string{}
		vlanNamespaces := []string{}
		for _, vpcName := range []string{vpc1Name, vpc2Name} {
			vpc := &VPC{}
			err := kube.Get(ctx, ktypes.NamespacedName{Name: vpcName, Namespace: peering.Namespace}, vpc)
			if err != nil {
				if kapierrors.IsNotFound(err) {
					return nil, field.NotFound(spec.Child("permit"), vpcName)
				}

				return nil, fmt.Errorf("failed to list VPCs: %w", err) // TODO replace with some internal error to not expose to the user
			}

			if vpcFabric := vpc.Spec.Topology.Fabric; vpcFabric != peeringFabric {
				return nil, field.Invalid(spec.Child("permit"), vpcName, fmt.Sprintf("peering is in fabric %s but the vpc is in fabric %s", peeringFabric, vpcFabric))
			}

			ipv4Namespaces = append(ipv4Namespaces, vpc.Spec.IPv4Namespace)
			vlanNamespaces = append(vlanNamespaces, vpc.Spec.VLANNamespace)
		}

		if len(ipv4Namespaces) != 2 {
			return nil, fmt.Errorf("failed to find IPv4 namespaces for VPCs") //nolint:err113
		}
		if ipv4Namespaces[0] != ipv4Namespaces[1] {
			return nil, field.Invalid(spec.Child("permit"), field.OmitValueType{}, "VPCs must be in the same IPv4 namespace")
		}

		if len(vlanNamespaces) != 2 {
			return nil, fmt.Errorf("failed to find VLAN namespaces for VPCs") //nolint:err113
		}
		if vlanNamespaces[0] != vlanNamespaces[1] {
			return nil, field.Invalid(spec.Child("permit"), field.OmitValueType{}, "VPCs must be in the same VLAN namespace")
		}

		vpc1 := &VPC{}
		err = kube.Get(ctx, ktypes.NamespacedName{Name: vpc1Name, Namespace: peering.Namespace}, vpc1)
		if err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, field.NotFound(spec.Child("permit"), vpc1Name)
			}

			return nil, fmt.Errorf("failed to get VPC %s: %w", vpc1Name, err) // TODO replace with some internal error to not expose to the user
		}

		vpc2 := &VPC{}
		err = kube.Get(ctx, ktypes.NamespacedName{Name: vpc2Name, Namespace: peering.Namespace}, vpc2)
		if err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, field.NotFound(spec.Child("permit"), vpc2Name)
			}

			return nil, fmt.Errorf("failed to get VPC %s: %w", vpc2Name, err) // TODO replace with some internal error to not expose to the user
		}

		// VPCs with no domain in common could only reach each other through a leaf shared by their domains
		vpc1Domains, vpc2Domains := vpc1.Spec.Topology.Domains, vpc2.Spec.Topology.Domains
		if !slices.ContainsFunc(vpc1Domains, func(domain string) bool { return slices.Contains(vpc2Domains, domain) }) {
			return nil, field.Invalid(spec.Child("permit"), field.OmitValueType{}, fmt.Sprintf("vpc %s is in domains %v and vpc %s in domains %v, they must share one", vpc1Name, vpc1Domains, vpc2Name, vpc2Domains))
		}

		for idx, permit := range peering.Spec.Permit {
			for vpcName, vpcPeer := range permit {
				vpc := vpc1
				if vpcName == vpc2Name {
					vpc = vpc2
				} else if vpcName != vpc1Name {
					return nil, field.Invalid(spec.Child("permit").Index(idx), vpcName, "unexpected VPC")
				}

				for _, subnet := range vpcPeer.Subnets {
					if vpc.Spec.Subnets == nil || vpc.Spec.Subnets[subnet] == nil {
						return nil, field.NotFound(spec.Child("permit").Index(idx).Key(vpcName).Child("subnets"), subnet)
					}
				}
			}
		}
	}

	return nil, nil
}
