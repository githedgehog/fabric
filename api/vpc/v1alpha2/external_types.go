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

package v1alpha2

import (
	"context"
	"regexp"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var communityCheck = regexp.MustCompile("^(6553[0-5]|655[0-2][0-9]|654[0-9]{2}|65[0-4][0-9]{2}|6[0-4][0-9]{3}|[1-5][0-9]{4}|[1-9][0-9]{1,3}|[0-9]):(6553[0-5]|655[0-2][0-9]|654[0-9]{2}|65[0-4][0-9]{2}|6[0-4][0-9]{3}|[1-5][0-9]{4}|[1-9][0-9]{1,3}|[0-9])$")

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExternalSpec describes IPv4 namespace External belongs to and inbound/outbound communities which are used to
// filter routes from/to the external system.
type ExternalSpec struct {
	// IPv4Namespace is the name of the IPv4Namespace this External belongs to
	IPv4Namespace string `json:"ipv4Namespace,omitempty"`
	// InboundCommunity is the inbound community to filter routes from the external system (e.g. 65102:5000)
	InboundCommunity string `json:"inboundCommunity,omitempty"`
	// OutboundCommunity is theoutbound community that all outbound routes will be stamped with (e.g. 50000:50001)
	OutboundCommunity string `json:"outboundCommunity,omitempty"`
}

// ExternalStatus defines the observed state of External
type ExternalStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric;external,shortName=ext
// +kubebuilder:printcolumn:name="IPv4NS",type=string,JSONPath=`.spec.ipv4Namespace`,priority=0
// +kubebuilder:printcolumn:name="InComm",type=string,JSONPath=`.spec.inboundCommunity`,priority=0
// +kubebuilder:printcolumn:name="OutComm",type=string,JSONPath=`.spec.outboundCommunity`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// External object represents an external system connected to the Fabric and available to the specific IPv4Namespace.
// Users can do external peering with the external system by specifying the name of the External Object without need to
// worry about the details of how external system is attached to the Fabric.
type External struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the External
	Spec ExternalSpec `json:"spec,omitempty"`
	// Status is the observed state of the External
	Status ExternalStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ExternalList contains a list of External
type ExternalList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []External `json:"items"`
}

func init() {
	SchemeBuilder.Register(&External{}, &ExternalList{})
}

var (
	_ meta.Object     = (*External)(nil)
	_ meta.ObjectList = (*ExternalList)(nil)
)

func (extList *ExternalList) GetItems() []meta.Object {
	items := make([]meta.Object, len(extList.Items))
	for i := range extList.Items {
		items[i] = &extList.Items[i]
	}

	return items
}

func (external *External) Default() {
	meta.DefaultObjectMetadata(external)

	if external.Spec.IPv4Namespace == "" {
		external.Spec.IPv4Namespace = DefaultIPv4Namespace
	}

	if external.Labels == nil {
		external.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(external.Labels)

	external.Labels[LabelIPv4NS] = external.Spec.IPv4Namespace
}

func (external *External) Validate(ctx context.Context, kube client.Reader, _ *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(external); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	if external.Spec.IPv4Namespace == "" {
		return nil, errors.Errorf("IPv4Namespace is required")
	}

	if external.Spec.InboundCommunity == "" {
		return nil, errors.Errorf("inboundCommunity is required")
	}

	if external.Spec.OutboundCommunity == "" {
		return nil, errors.Errorf("outboundCommunity is required")
	}

	if !communityCheck.MatchString(external.Spec.InboundCommunity) {
		return nil, errors.Errorf("inboundCommunity %s is not a valid community, example 50000:50001", external.Spec.InboundCommunity)
	}

	if !communityCheck.MatchString(external.Spec.OutboundCommunity) {
		return nil, errors.Errorf("outboundCommunity %s is not a valid community, example 50000:50001", external.Spec.OutboundCommunity)
	}

	if kube != nil {
		ipNs := &IPv4Namespace{}
		err := kube.Get(ctx, types.NamespacedName{Name: external.Spec.IPv4Namespace, Namespace: external.Namespace}, ipNs)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("IPv4Namespace %s not found", external.Spec.IPv4Namespace)
			}

			return nil, errors.Wrapf(err, "failed to get IPv4Namespace %s", external.Spec.IPv4Namespace) // TODO replace with some internal error to not expose to the user
		}
	}

	return nil, nil
}
