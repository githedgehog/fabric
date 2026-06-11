// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

// Package v1alpha1 contains API Schema definitions for the gateway v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=gateway.githedgehog.com
package v1alpha1

import (
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "gateway.githedgehog.com", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		kmetav1.AddToGroupVersion(s, GroupVersion)

		return nil
	})

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
