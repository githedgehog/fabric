// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultGatewayGroup = "default"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewayGroupTopology is where a GatewayGroup sits in the fabric topology
type GatewayGroupTopology struct {
	// Fabric is the name of the Fabric this GatewayGroup belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
}

// GatewayGroupSpec defines the desired state of GatewayGroup
type GatewayGroupSpec struct {
	// Topology is where the GatewayGroup sits in the fabric topology
	Topology GatewayGroupTopology `json:"topology,omitempty"`
}

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
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &GatewayGroup{}, &GatewayGroupList{})

		return nil
	})
}

func (gg *GatewayGroup) Default() {
	if gg.Namespace == "" {
		gg.Namespace = kmetav1.NamespaceDefault
	}

	if gg.Spec.Topology.Fabric == "" {
		gg.Spec.Topology.Fabric = wiringapi.DefaultFabric
	}

	if gg.Labels == nil {
		gg.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(gg.Labels)

	gg.Labels[wiringapi.ListLabelFabric(gg.Spec.Topology.Fabric)] = ListLabelValue
}

func (gg *GatewayGroup) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) error {
	if fabricCfg != nil && !fabricCfg.EnableGateway {
		return fmt.Errorf("gateway support is not enabled") //nolint:err113
	}
	if gg.Namespace != kmetav1.NamespaceDefault {
		return fmt.Errorf("gatewaygroup namespace must be %s", kmetav1.NamespaceDefault) //nolint:err113
	}

	if err := wiringapi.CheckFabricExists(ctx, kube, gg.Namespace, gg.Spec.Topology.Fabric); err != nil {
		return fmt.Errorf("invalid gateway group: %w", err)
	}

	return nil
}
