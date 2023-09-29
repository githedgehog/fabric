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
	"sort"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VPCPeeringSpec defines the desired state of VPCPeering
type VPCPeeringSpec struct {
	//+kubebuilder:validation:MinItems=2
	//+kubebuilder:validation:MaxItems=2
	VPCs []string `json:"vpcs,omitempty"`
}

// VPCPeeringStatus defines the observed state of VPCPeering
type VPCPeeringStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric,shortName=vpcpeer;peering;vp
// +kubebuilder:printcolumn:name="VPC1",type=string,JSONPath=`.spec.vpcs[0]`,priority=0
// +kubebuilder:printcolumn:name="VPC2",type=string,JSONPath=`.spec.vpcs[1]`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// VPCPeering is the Schema for the vpcpeerings API
type VPCPeering struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCPeeringSpec   `json:"spec,omitempty"`
	Status VPCPeeringStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VPCPeeringList contains a list of VPCPeering
type VPCPeeringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPCPeering `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPCPeering{}, &VPCPeeringList{})
}

func (peering *VPCPeering) Default() {
	sort.Slice(peering.Spec.VPCs, func(i, j int) bool {
		return peering.Spec.VPCs[i] < peering.Spec.VPCs[j]
	})

	if peering.Labels == nil {
		peering.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(peering.Labels)

	for _, vpc := range peering.Spec.VPCs {
		peering.Labels[ListLabelVPC(vpc)] = ListLabelValue
	}
}

func (peering *VPCPeering) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	if len(peering.Spec.VPCs) != 2 {
		return nil, errors.Errorf("vpc peering must have exactly 2 VPCs")
	}
	if peering.Spec.VPCs[0] == peering.Spec.VPCs[1] {
		return nil, errors.Errorf("vpc peering must have different VPCs")
	}

	if client != nil {
		other := &VPCPeeringList{}
		err := client.List(ctx, other, map[string]string{
			ListLabelVPC(peering.Spec.VPCs[0]): ListLabelValue,
			ListLabelVPC(peering.Spec.VPCs[1]): ListLabelValue,
		})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "failed to list VPC peerings") // TODO replace with some internal error to not expose to the user
		}

		for _, vpc := range peering.Spec.VPCs {
			vpcs := &VPCList{}
			err := client.List(ctx, vpcs, map[string]string{
				ListLabelVPC(vpc): ListLabelValue,
			})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil, errors.Errorf("vpc %s not found", vpc)
				}
				return nil, errors.Wrapf(err, "failed to list VPCs") // TODO replace with some internal error to not expose to the user
			}
		}
	}

	return nil, nil
}
