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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExternalPeeringSpec defines the desired state of ExternalPeering
type ExternalPeeringSpec struct {
	// Permit defines the peering policy - which VPC and External to peer with and which subnets/prefixes to permit
	Permit ExternalPeeringSpecPermit `json:"permit,omitempty"`
}

// ExternalPeeringSpecPermit defines the peering policy - which VPC and External to peer with and which subnets/prefixes to permit
type ExternalPeeringSpecPermit struct {
	// VPC is the VPC-side of the configuration to peer with
	VPC ExternalPeeringSpecVPC `json:"vpc,omitempty"`
	// External is the External-side of the configuration to peer with
	External ExternalPeeringSpecExternal `json:"external,omitempty"`
}

// ExternalPeeringSpecVPC defines the VPC-side of the configuration to peer with
type ExternalPeeringSpecVPC struct {
	// Name is the name of the VPC to peer with
	Name string `json:"name,omitempty"`
	// Subnets is the list of subnets to advertise from VPC to the External
	Subnets []string `json:"subnets,omitempty"`
}

// ExternalPeeringSpecExternal defines the External-side of the configuration to peer with
type ExternalPeeringSpecExternal struct {
	// Name is the name of the External to peer with
	Name string `json:"name,omitempty"`
	// Prefixes is the list of prefixes to permit from the External to the VPC
	Prefixes []ExternalPeeringSpecPrefix `json:"prefixes,omitempty"`
}

// ExternalPeeringSpecPrefix defines the prefix to permit from the External to the VPC
type ExternalPeeringSpecPrefix struct {
	// Prefix is the subnet to permit from the External to the VPC, e.g. 0.0.0.0/0 for default route
	Prefix string `json:"prefix,omitempty"`
	// Ge is the minimum prefix length to permit from the External to the VPC, e.g. 24 for /24
	Ge uint8 `json:"ge,omitempty"`
	// Le is the maximum prefix length to permit from the External to the VPC, e.g. 32 for /32
	Le uint8 `json:"le,omitempty"`
}

// ExternalPeeringStatus defines the observed state of ExternalPeering
type ExternalPeeringStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric;external,shortName=extpeering;extpeer
// +kubebuilder:printcolumn:name="VPC",type=string,JSONPath=`.spec.permit.vpc.name`,priority=0
// +kubebuilder:printcolumn:name="VPCSubnets",type=string,JSONPath=`.spec.permit.vpc.subnets`,priority=1
// +kubebuilder:printcolumn:name="External",type=string,JSONPath=`.spec.permit.external.name`,priority=0
// +kubebuilder:printcolumn:name="ExtPrefixes",type=string,JSONPath=`.spec.permit.external.prefixes`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// ExternalPeering is the Schema for the externalpeerings API
type ExternalPeering struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the ExternalPeering
	Spec ExternalPeeringSpec `json:"spec,omitempty"`
	// Status is the observed state of the ExternalPeering
	Status ExternalPeeringStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ExternalPeeringList contains a list of ExternalPeering
type ExternalPeeringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalPeering `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalPeering{}, &ExternalPeeringList{})
}

func (peering *ExternalPeering) Default() {
	if peering.Labels == nil {
		peering.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(peering.Labels)

	peering.Labels[LabelVPC] = peering.Spec.Permit.VPC.Name
	peering.Labels[LabelExternal] = peering.Spec.Permit.External.Name

	sort.Strings(peering.Spec.Permit.VPC.Subnets)
	sort.Slice(peering.Spec.Permit.External.Prefixes, func(i, j int) bool {
		return peering.Spec.Permit.External.Prefixes[i].Prefix < peering.Spec.Permit.External.Prefixes[j].Prefix
	})
}

func (peering *ExternalPeering) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	if peering.Spec.Permit.VPC.Name == "" {
		return nil, errors.Errorf("vpc.name is required")
	}
	if peering.Spec.Permit.External.Name == "" {
		return nil, errors.Errorf("external.name is required")
	}

	for _, permit := range peering.Spec.Permit.External.Prefixes {
		if permit.Prefix == "" {
			return nil, errors.Errorf("external.prefixes.prefix is required")
		}
		if permit.Ge > permit.Le {
			return nil, errors.Errorf("external.prefixes.ge must be <= external.prefixes.le")
		}
		if permit.Ge > 32 {
			return nil, errors.Errorf("external.prefixes.ge must be <= 32")
		}
		if permit.Le > 32 {
			return nil, errors.Errorf("external.prefixes.le must be <= 32")
		}

		// TODO add more validation for prefix/ge/le
	}

	if client != nil {
		vpc := &VPC{}
		if err := client.Get(ctx, types.NamespacedName{Name: peering.Spec.Permit.VPC.Name, Namespace: peering.Namespace}, vpc); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("vpc %s not found", peering.Spec.Permit.VPC.Name)
			}

			return nil, errors.Wrapf(err, "failed to read vpc %s", peering.Spec.Permit.VPC.Name) // TODO replace with some internal error to not expose to the user
		}

		ext := &External{}
		if err := client.Get(ctx, types.NamespacedName{Name: peering.Spec.Permit.External.Name, Namespace: peering.Namespace}, ext); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("external %s not found", peering.Spec.Permit.External.Name)
			}

			return nil, errors.Wrapf(err, "failed to read external %s", peering.Spec.Permit.External.Name) // TODO replace with some internal error to not expose to the user
		}

		for _, subnet := range peering.Spec.Permit.VPC.Subnets {
			if _, exists := vpc.Spec.Subnets[subnet]; !exists {
				return nil, errors.Errorf("vpc %s does not have subnet %s", peering.Spec.Permit.VPC.Name, subnet)
			}
		}
	}

	return nil, nil
}
