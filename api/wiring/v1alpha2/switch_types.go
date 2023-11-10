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
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:validation:Enum=spine;server-leaf;border-leaf
type SwitchRole string

const (
	SwitchRoleSpine      SwitchRole = "spine"
	SwitchRoleServerLeaf SwitchRole = "server-leaf"
	SwitchRoleBorderLeaf SwitchRole = "border-leaf"
)

var SwitchRoles = []SwitchRole{
	SwitchRoleSpine,
	SwitchRoleServerLeaf,
	SwitchRoleBorderLeaf,
}

func (r SwitchRole) IsSpine() bool {
	return r == SwitchRoleSpine
}

func (r SwitchRole) IsLeaf() bool {
	return r == SwitchRoleServerLeaf || r == SwitchRoleBorderLeaf
}

// SwitchSpec defines the desired state of Switch
type SwitchSpec struct {
	// +kubebuilder:validation:Required
	Role            SwitchRole        `json:"role,omitempty"`
	Description     string            `json:"description,omitempty"`
	Profile         string            `json:"profile,omitempty"`
	Location        Location          `json:"location,omitempty"`
	LocationSig     LocationSig       `json:"locationSig,omitempty"`
	ASN             uint32            `json:"asn,omitempty"`
	IP              string            `json:"ip,omitempty"`
	PortGroupSpeeds map[string]string `json:"portGroupSpeeds,omitempty"`
	PortBreakouts   map[string]string `json:"portBreakouts,omitempty"`
}

// SwitchStatus defines the observed state of Switch
type SwitchStatus struct {
	Applied ApplyStatus `json:"applied,omitempty"`
	// TODO: add port status fields
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=sw
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`,priority=0
// +kubebuilder:printcolumn:name="Descr",type=string,JSONPath=`.spec.description`,priority=0
// +kubebuilder:printcolumn:name="LocationUUID",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/location`,priority=0
// +kubebuilder:printcolumn:name="PortGroupSpeeds",type=string,JSONPath=`.spec.portGroupSpeeds`,priority=1
// +kubebuilder:printcolumn:name="Rack",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/rack`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// Switch is the Schema for the switches API
//
// All switches should always have 1 labels defined: wiring.githedgehog.com/rack. It represents name of the rack it
// belongs to.
type Switch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SwitchSpec   `json:"spec,omitempty"`
	Status SwitchStatus `json:"status,omitempty"`
}

const KindSwitch = "Switch"

//+kubebuilder:object:root=true

// SwitchList contains a list of Switch
type SwitchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Switch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Switch{}, &SwitchList{})
}

func (sw *Switch) Default() {
	if sw.Labels == nil {
		sw.Labels = map[string]string{}
	}

	CleanupFabricLabels(sw.Labels)

	if sw.Spec.Location.IsEmpty() {
		sw.Spec.Location = Location{Location: fmt.Sprintf("gen--%s--%s", sw.Namespace, sw.Name)}
	}
	if sw.Spec.LocationSig.Sig == "" {
		sw.Spec.LocationSig.Sig = "<undefined>"
	}
	if sw.Spec.LocationSig.UUIDSig == "" {
		sw.Spec.LocationSig.UUIDSig = "<undefined>"
	}

	uuid, _ := sw.Spec.Location.GenerateUUID()
	sw.Labels[LabelLocation] = uuid

	for name, value := range sw.Spec.PortGroupSpeeds {
		sw.Spec.PortGroupSpeeds[name], _ = strings.CutPrefix(value, "SPEED_")
	}
}

func (sw *Switch) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	// TODO validate port group speeds against switch profile

	if client != nil {
		switches := &SwitchList{}
		err := client.List(ctx, switches, map[string]string{
			LabelLocation: sw.Labels[LabelLocation],
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get switches") // TODO replace with some internal error to not expose to the user
		}

		for _, other := range switches.Items {
			if sw.Name == other.Name {
				continue
			}

			return nil, errors.Errorf("switch with location %s already exists", sw.Labels[LabelLocation])
		}
	}

	return nil, nil
}
