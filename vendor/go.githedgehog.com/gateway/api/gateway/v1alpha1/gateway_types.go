// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewaySpec defines the desired state of Gateway.
type GatewaySpec struct{}

// GatewayStatus defines the observed state of Gateway.
type GatewayStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;hedgehog-gateway,shortName=gw
// Gateway is the Schema for the gateways API.
type Gateway struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewaySpec   `json:"spec,omitempty"`
	Status GatewayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayList contains a list of Gateway.
type GatewayList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []Gateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Gateway{}, &GatewayList{})
}

func (gw *Gateway) Default() {
	// TODO add defaulting logic
}

func (gw *Gateway) Validate(_ context.Context, _ kclient.Reader) error {
	// TODO add validation logic
	return nil
}
