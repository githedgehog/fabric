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

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VLANNamespaceSpec defines the desired state of VLANNamespace
type VLANNamespaceSpec struct {
	//+kubebuilder:validation:MinItems=1
	//+kubebuilder:validation:MaxItems=20
	// Ranges is a list of VLAN ranges to be used in this namespace, couldn't overlap between each other and with Fabric reserved VLAN ranges
	Ranges []meta.VLANRange `json:"ranges,omitempty"`
}

// VLANNamespaceStatus defines the observed state of VLANNamespace
type VLANNamespaceStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=vlanns
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// VLANNamespace is the Schema for the vlannamespaces API
type VLANNamespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the VLANNamespace
	Spec VLANNamespaceSpec `json:"spec,omitempty"`
	// Status is the observed state of the VLANNamespace
	Status VLANNamespaceStatus `json:"status,omitempty"`
}

const KindVLANNamespace = "VLANNamespace"

//+kubebuilder:object:root=true

// VLANNamespaceList contains a list of VLANNamespace
type VLANNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VLANNamespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VLANNamespace{}, &VLANNamespaceList{})
}

func (ns *VLANNamespaceSpec) Labels() map[string]string {
	// TODO
	return map[string]string{}
}

func (ns *VLANNamespace) Default() {
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	CleanupFabricLabels(ns.Labels)

	maps.Copy(ns.Labels, ns.Spec.Labels())

	if ranges, err := meta.NormalizedVLANRanges(ns.Spec.Ranges); err != nil {
		ns.Spec.Ranges = ranges
	}
}

func (ns *VLANNamespace) Validate(ctx context.Context, client validation.Client, reservedVLANs []meta.VLANRange) (admission.Warnings, error) {
	if _, err := meta.NormalizedVLANRanges(ns.Spec.Ranges); err != nil {
		return nil, errors.Wrapf(err, "invalid ranges")
	}

	if err := meta.CheckVLANRangesOverlap(append(reservedVLANs, ns.Spec.Ranges...)); err != nil {
		return nil, errors.Wrapf(err, "ranges overlap with Fabric reserved VLANs")
	}

	return nil, nil
}
