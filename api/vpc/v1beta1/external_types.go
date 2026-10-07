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
	"fmt"
	"net/netip"
	"regexp"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	VPCInfoExtPrefix = "ext."
)

var communityCheck = regexp.MustCompile("^(6553[0-5]|655[0-2][0-9]|654[0-9]{2}|65[0-4][0-9]{2}|6[0-4][0-9]{3}|[1-5][0-9]{4}|[1-9][0-9]{1,3}|[0-9]):(6553[0-5]|655[0-2][0-9]|654[0-9]{2}|65[0-4][0-9]{2}|6[0-4][0-9]{3}|[1-5][0-9]{4}|[1-9][0-9]{1,3}|[0-9])$")

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ExternalStaticSpec struct {
	// Prefixes is the list of IPv4 prefixes reachable via the external
	Prefixes []string `json:"prefixes,omitempty"`
}

// ExternalTopology is where a External sits in the fabric topology
type ExternalTopology struct {
	// Fabric is the name of the Fabric this External belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
	// Domain is the Fabric domain the External is in (if not specified, "default" is used). It can
	// only be attached to switches in that domain, and it is immutable
	Domain string `json:"domain,omitempty"`
}

// ExternalSpec describes IPv4 namespace External belongs to and inbound/outbound communities which are used to
// filter routes from/to the external system.
type ExternalSpec struct {
	// Topology is where the External sits in the fabric topology
	Topology ExternalTopology `json:"topology,omitempty"`
	// IPv4Namespace is the name of the IPv4Namespace this External belongs to
	IPv4Namespace string `json:"ipv4Namespace,omitempty"`
	// InboundCommunity is the optional inbound community to filter routes from the external system (e.g. 65102:5000)
	InboundCommunity string `json:"inboundCommunity,omitempty"`
	// OutboundCommunity is the optional outbound community that all outbound routes will be stamped with (e.g. 50000:50001)
	OutboundCommunity string `json:"outboundCommunity,omitempty"`
	// Static contains parameters specific to static externals
	// +optional
	Static *ExternalStaticSpec `json:"static,omitempty"`
	// LocalASN makes every attachment to this External present the same ASN to the external system
	// instead of each border leaf's own. Changing it resets all sessions to this External, and the
	// external system has to change its remote-as to match. Static attachments ignore it.
	// +optional
	LocalASN uint32 `json:"localASN,omitempty"`
}

// ExternalStatus defines the observed state of External
type ExternalStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric;external,shortName=ext
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.topology.domain`,priority=0
// +kubebuilder:printcolumn:name="IPv4NS",type=string,JSONPath=`.spec.ipv4Namespace`,priority=0
// +kubebuilder:printcolumn:name="InComm",type=string,JSONPath=`.spec.inboundCommunity`,priority=0
// +kubebuilder:printcolumn:name="OutComm",type=string,JSONPath=`.spec.outboundCommunity`,priority=0
// +kubebuilder:printcolumn:name="LocalASN",type=string,JSONPath=`.spec.localASN`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// External object represents an external system connected to the Fabric and available to the specific IPv4Namespace.
// Users can do external peering with the external system by specifying the name of the External Object without need to
// worry about the details of how external system is attached to the Fabric.
type External struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the External
	Spec ExternalSpec `json:"spec,omitempty"`
	// Status is the observed state of the External
	Status ExternalStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ExternalList contains a list of External
type ExternalList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []External `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &External{}, &ExternalList{})

		return nil
	})
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

	if external.Spec.Topology.Fabric == "" {
		external.Spec.Topology.Fabric = wiringapi.DefaultFabric
	}
	if external.Spec.Topology.Domain == "" {
		external.Spec.Topology.Domain = wiringapi.DefaultFabricDomain
	}
	if external.Spec.IPv4Namespace == "" {
		external.Spec.IPv4Namespace = DefaultIPv4Namespace
	}

	if external.Labels == nil {
		external.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(external.Labels)

	external.Labels[LabelIPv4NS] = external.Spec.IPv4Namespace
	external.Labels[wiringapi.ListLabelFabric(external.Spec.Topology.Fabric)] = ListLabelValue
	external.Labels[wiringapi.ListLabelDomain(external.Spec.Topology.Domain)] = ListLabelValue
}

func (external *External) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	var warns admission.Warnings
	if err := meta.ValidateObjectMetadata(external); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	if err := wiringapi.CheckFabricExists(ctx, kube, external.Namespace, external.Spec.Topology.Fabric); err != nil {
		return nil, fmt.Errorf("failed to validate fabric: %w", err)
	}
	if external.Spec.Topology.Domain == "" {
		return nil, fmt.Errorf("topology.domain is required") //nolint:err113
	}

	if len(external.Name) > 11 {
		return nil, errors.Errorf("name %s is too long, must be <= 11 characters", external.Name)
	}
	if external.Spec.IPv4Namespace == "" {
		return nil, errors.Errorf("IPv4Namespace is required")
	}

	if external.Spec.Static == nil {
		if external.Spec.InboundCommunity != "" && !communityCheck.MatchString(external.Spec.InboundCommunity) {
			return nil, errors.Errorf("inboundCommunity %s is not a valid community, example 50000:50001", external.Spec.InboundCommunity)
		}

		if external.Spec.OutboundCommunity != "" && !communityCheck.MatchString(external.Spec.OutboundCommunity) {
			return nil, errors.Errorf("outboundCommunity %s is not a valid community, example 50000:50001", external.Spec.OutboundCommunity)
		}
	} else {
		// While static prefixes are present the external may also have BGP attachments, and a
		// static route carries no community. Any route-map that matches on the inbound community
		// would then drop the static route outright rather than merely rank it lower: on a switch
		// holding both kinds ext-inbound--<ext> is also the EVPN advertise policy, so the static
		// route would never be re-originated as a type-5, and the same applies to the leak into a
		// VPC VRF. Both stay untagged until the last static attachment is gone; tagging them too
		// needs a fabric-owned identity community.
		if external.Spec.InboundCommunity != "" || external.Spec.OutboundCommunity != "" {
			return nil, errors.Errorf("inboundCommunity and outboundCommunity must be empty when static configuration is present")
		}

		if len(external.Spec.Static.Prefixes) == 0 {
			return nil, errors.Errorf("at least one prefix must be specified for static externals")
		}
		prefixes := []netip.Prefix{}
		for _, p := range external.Spec.Static.Prefixes {
			parsed, err := netip.ParsePrefix(p)
			if err != nil {
				return nil, fmt.Errorf("invalid prefix %s in static external configuration: %w", p, err)
			}
			prefixes = append(prefixes, parsed)
		}
		for i := range prefixes {
			for j := i + 1; j < len(prefixes); j++ {
				if prefixes[i].Overlaps(prefixes[j]) {
					return nil, fmt.Errorf("static prefixes %s and %s overlap with each other", prefixes[i].String(), prefixes[j].String()) //nolint:goerr113
				}
			}
		}
	}

	if kube != nil {
		ipNs := &IPv4Namespace{}
		err := kube.Get(ctx, ktypes.NamespacedName{Name: external.Spec.IPv4Namespace, Namespace: external.Namespace}, ipNs)
		if err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, errors.Errorf("IPv4Namespace %s not found", external.Spec.IPv4Namespace)
			}

			return nil, errors.Wrapf(err, "failed to get IPv4Namespace %s", external.Spec.IPv4Namespace) // TODO replace with some internal error to not expose to the user
		}

		extFabric := external.Spec.Topology.Fabric
		if nsFabric := ipNs.Spec.Topology.Fabric; nsFabric != extFabric {
			return nil, fmt.Errorf("external is in fabric %s but its IPv4Namespace %s is in fabric %s", extFabric, ipNs.Name, nsFabric) //nolint:err113
		}

		fabric, err := wiringapi.GetFabricSpec(ctx, kube, external.Namespace, external.Spec.Topology.Fabric)
		if err != nil {
			return nil, fmt.Errorf("failed to get fabric: %w", err)
		}
		domain := external.Spec.Topology.Domain
		if _, exists := fabric.Domains[domain]; !exists {
			return nil, fmt.Errorf("domain %s not found in fabric %s, topology.domain must name one of its domains", domain, extFabric) //nolint:err113
		}

		if localASN := external.Spec.LocalASN; localASN != 0 {
			if what := asnCollision(fabric, localASN); what != "" {
				return nil, fmt.Errorf("localASN %d is %s of its own fabric", localASN, what) //nolint:err113
			}

			attaches := &ExternalAttachmentList{}
			if err := kube.List(ctx, attaches, kclient.InNamespace(external.Namespace), kclient.MatchingLabels{LabelExternal: external.Name}); err != nil {
				return nil, fmt.Errorf("failed to list external attachments for %s: %w", external.Name, err) // TODO hide internal error
			}
			for _, attach := range attaches.Items {
				if attach.Spec.Static != nil {
					continue
				}
				// see ExternalAttachment.Validate, a neighbor ASN is required with a localASN
				if attach.Spec.Neighbor.ASN == 0 {
					return nil, fmt.Errorf("localASN requires a neighbor ASN, external attachment %s has none", attach.Name) //nolint:err113
				}
				if attach.Spec.Neighbor.ASN == localASN {
					return nil, fmt.Errorf("localASN %d is the neighbor ASN of external attachment %s", localASN, attach.Name) //nolint:err113
				}
			}

			// a session drops routes carrying its local-as, so two fabrics presenting the same one
			// can't reach each other through the external systems
			fabrics := &wiringapi.FabricList{}
			if err := kube.List(ctx, fabrics, kclient.InNamespace(external.Namespace)); err != nil {
				return nil, fmt.Errorf("failed to list fabrics: %w", err) // TODO hide internal error
			}
			for _, other := range fabrics.Items {
				if other.Name == extFabric {
					continue
				}
				if what := asnCollision(&other.Spec, localASN); what != "" {
					warns = append(warns, fmt.Sprintf("localASN %d is %s of fabric %s, which will drop routes from this fabric if they reach it", localASN, what, other.Name))
				}
			}
			externals := &ExternalList{}
			if err := kube.List(ctx, externals, kclient.InNamespace(external.Namespace)); err != nil {
				return nil, fmt.Errorf("failed to list externals: %w", err) // TODO hide internal error
			}
			for _, other := range externals.Items {
				if other.Spec.LocalASN == localASN && other.Spec.Topology.Fabric != extFabric {
					warns = append(warns, fmt.Sprintf("localASN %d is also used by external %s of fabric %s, so the two fabrics can't reach each other through external systems", localASN, other.Name, other.Spec.Topology.Fabric))
				}
			}
		}
	}

	return warns, nil
}
