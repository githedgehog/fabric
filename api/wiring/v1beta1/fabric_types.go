// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"context"
	"fmt"
	"strings"

	"go.githedgehog.com/fabric/api/meta"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FabricSpec defines the desired state of Fabric
type FabricSpec struct {
	// LeafASNStart is the first ASN of the range leaves of this Fabric are allocated from
	LeafASNStart uint32 `json:"leafASNStart,omitempty"`
	// LeafASNEnd is the last ASN of the range leaves of this Fabric are allocated from
	LeafASNEnd uint32 `json:"leafASNEnd,omitempty"`
	// DisableBFD disables BFD on the links between switches and on the sessions with the gateways
	DisableBFD bool `json:"disableBFD,omitempty"`
	// Domains is the set of spine layers in this Fabric, at least one is required
	Domains map[string]FabricDomainSpec `json:"domains,omitempty"`
}

// FabricDomainSpec defines a single spine layer of a Fabric
type FabricDomainSpec struct {
	// SpineASN is the ASN shared by all spines of this domain, outside the leaf ASN range
	SpineASN uint32 `json:"spineASN,omitempty"`
	// GatewayASN is the ASN shared by all gateways attached to this domain, outside the leaf ASN range
	GatewayASN uint32 `json:"gatewayASN,omitempty"`
}

// FabricStatus defines the observed state of Fabric
type FabricStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=fab
// +kubebuilder:printcolumn:name="LeafASNStart",type=integer,JSONPath=`.spec.leafASNStart`,priority=0
// +kubebuilder:printcolumn:name="LeafASNEnd",type=integer,JSONPath=`.spec.leafASNEnd`,priority=0
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
			return fmt.Errorf("fabric %s not found", name) //nolint:err113
		}

		return fmt.Errorf("failed to get fabric %s: %w", name, err) // TODO replace with some internal error to not expose to the user
	}

	return nil
}

// domainASNs maps each spine and gateway ASN of the fabric to what it is used for, failing if a
// domain has either unset or the same ASN is used twice
func (fabric *Fabric) domainASNs() (map[uint32]string, error) {
	asns := map[uint32]string{}
	for name, domain := range fabric.Spec.Domains {
		for role, asn := range map[string]uint32{"spineASN": domain.SpineASN, "gatewayASN": domain.GatewayASN} {
			what := fmt.Sprintf("domain %s %s", name, role)
			if asn == 0 {
				return nil, fmt.Errorf("%s is required", what) //nolint:err113
			}
			if other, exists := asns[asn]; exists {
				return nil, fmt.Errorf("%s %d is already used as %s", what, asn, other) //nolint:err113
			}
			asns[asn] = what
		}
	}

	return asns, nil
}

// DefaultFabricSpec is the spec Fabric/default is seeded with from the controller config
func DefaultFabricSpec(cfg *meta.FabricConfig) FabricSpec {
	return FabricSpec{
		LeafASNStart: cfg.LeafASNStart,
		LeafASNEnd:   cfg.LeafASNEnd,
		DisableBFD:   cfg.DisableBFD,
		Domains: map[string]FabricDomainSpec{
			DefaultFabricDomain: {SpineASN: cfg.SpineASN, GatewayASN: cfg.GatewayASN},
		},
	}
}

// GetFabricSpec returns the spec of the named fabric. Fabric/default falls back to the controller
// config while it does not exist: hhfab validates wiring with no controller running, and
// admission can run before the initializer has created it.
func GetFabricSpec(ctx context.Context, kube kclient.Reader, cfg *meta.FabricConfig, namespace, fabricName string) (*FabricSpec, error) {
	name := FabricNameOrDefault(fabricName)

	if kube != nil {
		fabric := &Fabric{}
		err := kube.Get(ctx, ktypes.NamespacedName{Name: name, Namespace: namespace}, fabric)
		if err == nil {
			return &fabric.Spec, nil
		}
		if !kapierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get fabric %s: %w", name, err) // TODO replace with some internal error to not expose to the user
		}
	}

	if name != DefaultFabric || cfg == nil {
		return nil, fmt.Errorf("fabric %s not found", name) //nolint:err113
	}

	spec := DefaultFabricSpec(cfg)

	return &spec, nil
}

func (fabric *Fabric) Default() {
	meta.DefaultObjectMetadata(fabric)
}

func (fabric *Fabric) Validate(ctx context.Context, kube kclient.Reader, _ *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(fabric); err != nil {
		return nil, fmt.Errorf("failed to validate metadata: %w", err)
	}

	// the name becomes a label key segment, which Kubernetes caps at 63 characters. Without this
	// the failure surfaces as an opaque label error on every object that references the fabric
	if len(fabric.Name) > 63 {
		return nil, fmt.Errorf("name %s is too long, must be <= 63 characters", fabric.Name) //nolint:err113
	}

	if fabric.Spec.LeafASNStart == 0 || fabric.Spec.LeafASNEnd == 0 {
		return nil, fmt.Errorf("leafASNStart and leafASNEnd are required") //nolint:err113
	}
	if fabric.Spec.LeafASNStart > fabric.Spec.LeafASNEnd {
		return nil, fmt.Errorf("leafASNStart %d is greater than leafASNEnd %d", fabric.Spec.LeafASNStart, fabric.Spec.LeafASNEnd) //nolint:err113
	}

	if len(fabric.Spec.Domains) == 0 {
		return nil, fmt.Errorf("at least one domain is required") //nolint:err113
	}
	for name := range fabric.Spec.Domains {
		// the name becomes a label key segment
		if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
			return nil, fmt.Errorf("invalid domain name %s: %s", name, strings.Join(errs, ", ")) //nolint:err113
		}
	}

	asns, err := fabric.domainASNs()
	if err != nil {
		return nil, err
	}
	for asn, what := range asns {
		if asn >= fabric.Spec.LeafASNStart && asn <= fabric.Spec.LeafASNEnd {
			return nil, fmt.Errorf("%s %d is within the leaf ASN range %d-%d", what, asn, fabric.Spec.LeafASNStart, fabric.Spec.LeafASNEnd) //nolint:err113
		}
	}

	if kube != nil {
		// fabrics can peer with each other as externals, and a route carrying an ASN of the receiving
		// fabric is silently dropped by the border leaf filter or by BGP loop detection
		fabrics := &FabricList{}
		if err := kube.List(ctx, fabrics, kclient.InNamespace(fabric.Namespace)); err != nil {
			return nil, fmt.Errorf("failed to list fabrics: %w", err) // TODO hide internal error
		}
		for _, other := range fabrics.Items {
			if other.Name == fabric.Name {
				continue
			}
			if fabric.Spec.LeafASNStart <= other.Spec.LeafASNEnd && other.Spec.LeafASNStart <= fabric.Spec.LeafASNEnd {
				return nil, fmt.Errorf("leaf ASN range %d-%d overlaps with fabric %s leaf ASN range %d-%d", fabric.Spec.LeafASNStart, fabric.Spec.LeafASNEnd, other.Name, other.Spec.LeafASNStart, other.Spec.LeafASNEnd) //nolint:err113
			}

			// the other fabric has passed this validation already
			otherASNs, _ := other.domainASNs()
			for asn, what := range asns {
				if asn >= other.Spec.LeafASNStart && asn <= other.Spec.LeafASNEnd {
					return nil, fmt.Errorf("%s %d is within fabric %s leaf ASN range %d-%d", what, asn, other.Name, other.Spec.LeafASNStart, other.Spec.LeafASNEnd) //nolint:err113
				}
				if otherWhat, exists := otherASNs[asn]; exists {
					return nil, fmt.Errorf("%s %d is already used by fabric %s as %s", what, asn, other.Name, otherWhat) //nolint:err113
				}
			}
			for asn, otherWhat := range otherASNs {
				if asn >= fabric.Spec.LeafASNStart && asn <= fabric.Spec.LeafASNEnd {
					return nil, fmt.Errorf("leaf ASN range %d-%d contains fabric %s %s %d", fabric.Spec.LeafASNStart, fabric.Spec.LeafASNEnd, other.Name, otherWhat, asn) //nolint:err113
				}
			}
		}
	}

	return nil, nil
}
