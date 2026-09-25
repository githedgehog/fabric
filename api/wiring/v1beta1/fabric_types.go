// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FabricSpec defines the desired state of Fabric
type FabricSpec struct {
	// ASNStart is the first ASN of the range reserved for this Fabric
	ASNStart uint32 `json:"asnStart,omitempty"`
	// ASNEnd is the last ASN of the range reserved for this Fabric
	ASNEnd uint32 `json:"asnEnd,omitempty"`
	// Domains is the set of spine layers in this Fabric, defaulted to a single "default" domain if empty
	Domains map[string]FabricDomainSpec `json:"domains,omitempty"`
}

// FabricDomainSpec defines a single spine layer of a Fabric
type FabricDomainSpec struct {
	// SpineASN is the ASN shared by all spines of this domain, within the Fabric ASN range
	SpineASN uint32 `json:"spineASN,omitempty"`
}

// FabricStatus defines the observed state of Fabric
type FabricStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=fab
// +kubebuilder:printcolumn:name="ASNStart",type=integer,JSONPath=`.spec.asnStart`,priority=0
// +kubebuilder:printcolumn:name="ASNEnd",type=integer,JSONPath=`.spec.asnEnd`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// Fabric is a single spine-leaf topology, owning its switches, its ASN range and its address and VLAN namespaces.
// Fabrics are not cabled to each other and reach each other by peering as external systems.
type Fabric struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the Fabric
	Spec FabricSpec `json:"spec,omitempty"`
	// Status is the observed state of the Fabric
	Status FabricStatus `json:"status,omitempty"`
}

const KindFabric = "Fabric"

//+kubebuilder:object:root=true

// FabricList contains a list of Fabric
type FabricList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []Fabric `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &Fabric{}, &FabricList{})

		return nil
	})
}

var (
	_ meta.Object     = (*Fabric)(nil)
	_ meta.ObjectList = (*FabricList)(nil)
)

func (fabricList *FabricList) GetItems() []meta.Object {
	items := make([]meta.Object, len(fabricList.Items))
	for i := range fabricList.Items {
		items[i] = &fabricList.Items[i]
	}

	return items
}

// CheckFabricExists checks that a fabric reference names a Fabric that exists. The default fabric
// is exempt: it is ensured at controller startup, and an object admitted before that has run would
// otherwise be refused.
func CheckFabricExists(ctx context.Context, kube kclient.Reader, namespace, fabricName string) error {
	name := FabricNameOrDefault(fabricName)
	if kube == nil || name == DefaultFabric {
		return nil
	}

	if err := kube.Get(ctx, ktypes.NamespacedName{Name: name, Namespace: namespace}, &Fabric{}); err != nil {
		if kapierrors.IsNotFound(err) {
			return errors.Errorf("fabric %s not found", name)
		}

		return errors.Wrapf(err, "failed to get fabric %s", name) // TODO replace with some internal error to not expose to the user
	}

	return nil
}

func (fabric *Fabric) Default() {
	meta.DefaultObjectMetadata(fabric)

	// storing the implicit domain keeps a single-domain fabric free of special cases, and gives
	// its spine ASN somewhere to live
	if len(fabric.Spec.Domains) == 0 {
		fabric.Spec.Domains = map[string]FabricDomainSpec{DefaultFabricDomain: {}}
	}
}

func (fabric *Fabric) Validate(ctx context.Context, kube kclient.Reader, _ *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(fabric); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	// the name becomes a label key segment, which Kubernetes caps at 63 characters. Without this
	// the failure surfaces as an opaque label error on every object that references the fabric
	if len(fabric.Name) > 63 {
		return nil, errors.Errorf("name %s is too long, must be <= 63 characters", fabric.Name)
	}

	if fabric.Spec.ASNStart == 0 || fabric.Spec.ASNEnd == 0 {
		return nil, errors.Errorf("asnStart and asnEnd are required")
	}
	if fabric.Spec.ASNStart > fabric.Spec.ASNEnd {
		return nil, errors.Errorf("asnStart %d is greater than asnEnd %d", fabric.Spec.ASNStart, fabric.Spec.ASNEnd)
	}

	if len(fabric.Spec.Domains) > 1 {
		return nil, errors.Errorf("a fabric with more than one domain is not supported yet")
	}
	for name, domain := range fabric.Spec.Domains {
		if domain.SpineASN < fabric.Spec.ASNStart || domain.SpineASN > fabric.Spec.ASNEnd {
			return nil, errors.Errorf("domain %s spineASN %d is not within the fabric ASN range %d-%d", name, domain.SpineASN, fabric.Spec.ASNStart, fabric.Spec.ASNEnd)
		}
	}

	if kube != nil {
		// fabrics peer with each other as externals, and a route carrying an ASN of the receiving
		// fabric is silently dropped by the border leaf filter or by BGP loop detection
		fabrics := &FabricList{}
		if err := kube.List(ctx, fabrics, kclient.InNamespace(fabric.Namespace)); err != nil {
			return nil, errors.Wrapf(err, "failed to list fabrics") // TODO hide internal error
		}
		for _, other := range fabrics.Items {
			if other.Name == fabric.Name {
				continue
			}
			if fabric.Spec.ASNStart <= other.Spec.ASNEnd && other.Spec.ASNStart <= fabric.Spec.ASNEnd {
				return nil, errors.Errorf("ASN range %d-%d overlaps with fabric %s range %d-%d", fabric.Spec.ASNStart, fabric.Spec.ASNEnd, other.Name, other.Spec.ASNStart, other.Spec.ASNEnd)
			}
		}
	}

	return nil, nil
}
