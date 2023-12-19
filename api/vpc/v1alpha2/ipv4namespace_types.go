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
	"net"
	"sort"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IPv4NamespaceSpec defines the desired state of IPv4Namespace
type IPv4NamespaceSpec struct {
	//+kubebuilder:validation:MinItems=1
	//+kubebuilder:validation:MaxItems=20
	Subnets []string `json:"subnets,omitempty"`
}

// IPv4NamespaceStatus defines the observed state of IPv4Namespace
type IPv4NamespaceStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=ipns
// +kubebuilder:printcolumn:name="Subnets",type=string,JSONPath=`.spec.subnets`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// IPv4Namespace is the Schema for the ipv4namespaces API
type IPv4Namespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPv4NamespaceSpec   `json:"spec,omitempty"`
	Status IPv4NamespaceStatus `json:"status,omitempty"`
}

const KindIPv4Namespace = "IPv4Namespace"

//+kubebuilder:object:root=true

// IPv4NamespaceList contains a list of IPv4Namespace
type IPv4NamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPv4Namespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPv4Namespace{}, &IPv4NamespaceList{})
}

func (ns *IPv4NamespaceSpec) Labels() map[string]string {
	// TODO
	return map[string]string{}
}

func (ns *IPv4Namespace) Default() {
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(ns.Labels)

	maps.Copy(ns.Labels, ns.Spec.Labels())

	sort.Strings(ns.Spec.Subnets)
}

func (ns *IPv4Namespace) Validate(ctx context.Context, client validation.Client, resrvedSubnets []*net.IPNet) (admission.Warnings, error) {
	if len(ns.Name) > 11 {
		return nil, errors.Errorf("name %s is too long, must be <= 11 characters", ns.Name)
	}

	subnets := []*net.IPNet{}
	for _, subnet := range ns.Spec.Subnets {
		_, ipNet, err := net.ParseCIDR(subnet)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse cidr %s", subnet)
		}

		subnets = append(subnets, ipNet)
	}

	if err := iputil.VerifyNoOverlap(subnets); err != nil {
		return nil, errors.Wrapf(err, "subnets overlap")
	}

	subnets = append(subnets, resrvedSubnets...)

	return nil, errors.Wrapf(iputil.VerifyNoOverlap(subnets), "subnets overlap with reserved subnets")
}
