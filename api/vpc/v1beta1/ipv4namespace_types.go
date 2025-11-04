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
	"maps"
	"net"
	"sort"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IPv4NamespaceSpec defines the desired state of IPv4Namespace
type IPv4NamespaceSpec struct {
	//+kubebuilder:validation:MinItems=1
	//+kubebuilder:validation:MaxItems=20
	// Subnets is the list of subnets to allocate VPC subnets from, couldn't overlap between each other and with Fabric reserved subnets
	Subnets []string `json:"subnets,omitempty"`
}

// IPv4NamespaceStatus defines the observed state of IPv4Namespace
type IPv4NamespaceStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric,shortName=ipns
// +kubebuilder:printcolumn:name="Subnets",type=string,JSONPath=`.spec.subnets`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// IPv4Namespace represents a namespace for VPC subnets allocation. All VPC subnets within a single IPv4Namespace are
// non-overlapping. Users can create multiple IPv4Namespaces to allocate same VPC subnets.
type IPv4Namespace struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the IPv4Namespace
	Spec IPv4NamespaceSpec `json:"spec,omitempty"`
	// Status is the observed state of the IPv4Namespace
	Status IPv4NamespaceStatus `json:"status,omitempty"`
}

const KindIPv4Namespace = "IPv4Namespace"

//+kubebuilder:object:root=true

// IPv4NamespaceList contains a list of IPv4Namespace
type IPv4NamespaceList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []IPv4Namespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPv4Namespace{}, &IPv4NamespaceList{})
}

var (
	_ meta.Object     = (*IPv4Namespace)(nil)
	_ meta.ObjectList = (*IPv4NamespaceList)(nil)
)

func (ipNsList *IPv4NamespaceList) GetItems() []meta.Object {
	items := make([]meta.Object, len(ipNsList.Items))
	for i := range ipNsList.Items {
		items[i] = &ipNsList.Items[i]
	}

	return items
}

func (ns *IPv4NamespaceSpec) Labels() map[string]string {
	// TODO
	return map[string]string{}
}

func (ns *IPv4Namespace) Default() {
	meta.DefaultObjectMetadata(ns)

	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(ns.Labels)

	maps.Copy(ns.Labels, ns.Spec.Labels())

	sort.Strings(ns.Spec.Subnets)
}

func (ns *IPv4Namespace) Validate(_ context.Context, _ kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(ns); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

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

	// this limit is imposed by how we index ACL rules to prevent traffic local to
	// the VPC from being peered via the external peering
	if len(subnets) > 64 {
		return nil, errors.Errorf("too many subnets defined (%d), maximum is 64", len(subnets))
	}

	if err := iputil.VerifyNoOverlap(subnets); err != nil {
		return nil, errors.Wrapf(err, "subnets overlap")
	}

	if fabricCfg != nil {
		subnets = append(subnets, fabricCfg.ParsedReservedSubnets()...)
	}

	return nil, errors.Wrapf(iputil.VerifyNoOverlap(subnets), "subnets overlap with reserved subnets")
}
