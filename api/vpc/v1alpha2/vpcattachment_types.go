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
	"slices"
	"strings"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"golang.org/x/exp/maps"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VPCAttachmentSpec defines the desired state of VPCAttachment
type VPCAttachmentSpec struct {
	Subnet     string `json:"subnet,omitempty"`
	Connection string `json:"connection,omitempty"`
}

// VPCAttachmentStatus defines the observed state of VPCAttachment
type VPCAttachmentStatus struct {
	// Applied wiringapi.ApplyStatus `json:"applied,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric,shortName=vpcattach;attach;va
// +kubebuilder:printcolumn:name="VPCSUBNET",type=string,JSONPath=`.spec.subnet`,priority=0
// +kubebuilder:printcolumn:name="Connection",type=string,JSONPath=`.spec.connection`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// VPCAttachment is the Schema for the vpcattachments API
type VPCAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCAttachmentSpec   `json:"spec,omitempty"`
	Status VPCAttachmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VPCAttachmentList contains a list of VPCAttachment
type VPCAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPCAttachment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPCAttachment{}, &VPCAttachmentList{})
}

func (s *VPCAttachmentSpec) VPCName() string {
	return strings.SplitN(s.Subnet, "/", 2)[0]
}

func (s *VPCAttachmentSpec) SubnetName() string {
	parts := strings.SplitN(s.Subnet, "/", 2)
	if len(parts) == 1 {
		return "default"
	}
	if parts[1] == "" {
		return "default"
	}

	return parts[1]
}

func (s *VPCAttachmentSpec) Labels() map[string]string {
	return map[string]string{
		LabelVPC:                  s.VPCName(),
		LabelSubnet:               s.SubnetName(),
		wiringapi.LabelConnection: s.Connection,
	}
}

func (attach *VPCAttachment) Default() {
	parts := strings.SplitN(attach.Spec.Subnet, "/", 2)
	if len(parts[0]) == 0 {
		return // it'll be handled in validation stage
	}
	if len(parts) == 1 {
		attach.Spec.Subnet = parts[0] + "/default"
	}

	if attach.Labels == nil {
		attach.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(attach.Labels)

	maps.Copy(attach.Labels, attach.Spec.Labels())
}

func (attach *VPCAttachment) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	if attach.Spec.Subnet == "" {
		return nil, errors.Errorf("subnet is required")
	}
	parts := strings.SplitN(attach.Spec.Subnet, "/", 2)
	if len(parts[0]) == 0 {
		return nil, errors.Errorf("subnet should be in <vpc>/<subnet> format, vpc is missing")
	}
	if len(parts) == 1 {
		return nil, errors.Errorf("subnet should be in <vpc>/<subnet> format, subnet is missing")
	}
	vpcName, subnet := parts[0], parts[1]

	if attach.Spec.Connection == "" {
		return nil, errors.Errorf("connection is required")
	}

	if client != nil {
		vpc := &VPC{}
		err := client.Get(ctx, types.NamespacedName{Name: vpcName, Namespace: attach.Namespace}, vpc)
		if apierrors.IsNotFound(err) {
			return nil, errors.Errorf("vpc %s not found", vpcName)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get vpc %s", vpcName) // TODO replace with some internal error to not expose to the user
		}
		if vpc.Spec.Subnets == nil || vpc.Spec.Subnets[subnet] == nil {
			return nil, errors.Errorf("subnet %s not found in vpc %s", subnet, vpcName)
		}

		conn := &wiringapi.Connection{}
		err = client.Get(ctx, types.NamespacedName{Name: attach.Spec.Connection, Namespace: attach.Namespace}, conn)
		if apierrors.IsNotFound(err) {
			return nil, errors.Errorf("connection %s not found", attach.Spec.Connection)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get connection %s", attach.Spec.Connection) // TODO replace with some internal error to not expose to the user
		}

		var switchNames []string
		if conn.Spec.Unbundled != nil || conn.Spec.Bundled == nil || conn.Spec.MCLAG == nil {
			switchNames, _, _, _, err = conn.Spec.Endpoints()
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get endpoints for connection %s", attach.Spec.Connection) // TODO replace with some internal error to not expose to the user
			}
		} else {
			return nil, errors.Errorf("vpc could be attached only to Unbundled, Bundled and MCLAG connections")
		}

		if len(switchNames) == 0 {
			return nil, errors.Errorf("connection %s has no switch endpoints", attach.Spec.Connection)
		}

		for _, switchName := range switchNames {
			sw := &wiringapi.Switch{}
			err = client.Get(ctx, types.NamespacedName{Name: switchName, Namespace: attach.Namespace}, sw)
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("switch %s used in connection not found", switchName)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get switch %s used in connection", switchName) // TODO replace with some internal error to not expose to the user
			}

			if !slices.Contains(sw.Spec.VLANNamespaces, vpc.Spec.VLANNamespace) {
				return nil, errors.Errorf("switch %s used in connection doesn't have vlan namespace %s", switchName, vpc.Spec.VLANNamespace)
			}
		}

		attaches := &VPCAttachmentList{}
		err = client.List(ctx, attaches, map[string]string{
			wiringapi.LabelConnection: attach.Spec.Connection,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list vpc attachments for connection %s", attach.Spec.Connection) // TODO replace with some internal error to not expose to the user
		}

		for _, other := range attaches.Items {
			if other.Name == attach.Name {
				return nil, errors.Errorf("connection %s already attached to vpc %s", attach.Spec.Connection, vpcName)
			}
		}
	}

	return nil, nil
}
