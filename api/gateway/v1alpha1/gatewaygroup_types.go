// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultGatewayGroup = "default"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewayGroupSpec defines the desired state of GatewayGroup
type GatewayGroupSpec struct{}

// GatewayGroupStatus defines the observed state of GatewayGroup.
type GatewayGroupStatus struct {
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	// Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;hedgehog-gateway,shortName=gwgr
// GatewayGroup is the Schema for the gatewaygroups API
type GatewayGroup struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitzero"`

	// +optional
	Spec GatewayGroupSpec `json:"spec"`
	// +optional
	Status GatewayGroupStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// GatewayGroupList contains a list of GatewayGroup
type GatewayGroupList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitzero"`
	Items            []GatewayGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayGroup{}, &GatewayGroupList{})
}

func (gg *GatewayGroup) Default() {
}

func (gg *GatewayGroup) Validate(_ context.Context, _ kclient.Reader) error {
	return nil
}
