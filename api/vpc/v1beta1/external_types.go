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
	"slices"
	"strings"

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

// ExternalAdvertiseSpec controls what the fabric announces to this External, on every attachment
// to it. Only valid for BGP externals: a static external has no session to carry any of it.
type ExternalAdvertiseSpec struct {
	// Prepend is how many times to prepend our own ASN to the routes advertised to this External,
	// making it less attractive to the world by that many AS hops. Defaults to spec.priority, so a
	// backup External is de-preferred in both directions. Unlike MED, this is visible to the whole
	// internet, not just to this External. This and the attachment's own prepend add up, to at
	// most 20 in total.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +optional
	Prepend *uint8 `json:"prepend,omitempty"`
	// Prefixes is our own address space to announce to this External, for instance the pool a
	// Gateway NATs VPC traffic into. The fabric never accepts these prefixes back.
	// +optional
	Prefixes []string `json:"prefixes,omitempty"`
	// Communities are optionally attached to everything we advertise to this External, carrying
	// provider policy rather than our preference ranking
	// +optional
	Communities []string `json:"communities,omitempty"`
}

// ExternalSpec describes IPv4 namespace External belongs to and inbound/outbound communities which are used to
// filter routes from/to the external system.
type ExternalSpec struct {
	// IPv4Namespace is the name of the IPv4Namespace this External belongs to
	IPv4Namespace string `json:"ipv4Namespace,omitempty"`
	// InboundCommunity is the optional inbound community to filter routes from the external system (e.g. 65102:5000)
	InboundCommunity string `json:"inboundCommunity,omitempty"`
	// OutboundCommunity is the optional outbound community that all outbound routes will be stamped with (e.g. 50000:50001)
	OutboundCommunity string `json:"outboundCommunity,omitempty"`
	// Static contains parameters specific to static externals
	// +optional
	Static *ExternalStaticSpec `json:"static,omitempty"`
	// Priority is the default preference class for routes learned from this External, used by any
	// ExternalPeering that does not override it. Lower is preferred; equal priorities load-balance.
	// 0 (the default) preserves the existing behaviour. Not valid for static externals.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3
	// +optional
	Priority uint8 `json:"priority,omitempty"`
	// LocalASN makes every attachment to this External present the same ASN to the external system
	// instead of each border leaf's own. Without it the external system sees two neighbouring
	// ASNs and never compares the MEDs we send, so this is what makes advertise.med work. Opt-in
	// and never defaulted: changing it resets the sessions and the external system has to change
	// its remote-as to match. Not valid for static externals.
	// +optional
	LocalASN uint32 `json:"localASN,omitempty"`
	// Advertise controls what we announce to this External and how attractive we make it
	// +optional
	Advertise *ExternalAdvertiseSpec `json:"advertise,omitempty"`
}

// ExternalStatus defines the observed state of External
type ExternalStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric;external,shortName=ext
// +kubebuilder:printcolumn:name="IPv4NS",type=string,JSONPath=`.spec.ipv4Namespace`,priority=0
// +kubebuilder:printcolumn:name="InComm",type=string,JSONPath=`.spec.inboundCommunity`,priority=0
// +kubebuilder:printcolumn:name="OutComm",type=string,JSONPath=`.spec.outboundCommunity`,priority=0
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.spec.priority`,priority=1
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

	if external.Spec.IPv4Namespace == "" {
		external.Spec.IPv4Namespace = DefaultIPv4Namespace
	}

	if external.Labels == nil {
		external.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(external.Labels)

	external.Labels[LabelIPv4NS] = external.Spec.IPv4Namespace
}

// validateAdvertiseCommunities rejects communities the fabric gives its own meaning to. The
// ingress rewrite makes such a value harmless rather than dangerous, but a community meaning one
// thing to the ISP and another to us is not something to leave configurable.
func validateAdvertiseCommunities(comms []string, fabricCfg *meta.FabricConfig) error {
	reserved := []string{}
	for _, base := range meta.ReservedCommBases {
		reserved = append(reserved, fmt.Sprintf("%d", base))
	}
	if fabricCfg != nil {
		for _, gwComm := range fabricCfg.GatewayCommunities {
			if base, _, found := strings.Cut(gwComm, ":"); found {
				reserved = append(reserved, base)
			}
		}
		if base, _, found := strings.Cut(fabricCfg.BaseVPCCommunity, ":"); found {
			reserved = append(reserved, base)
		}
	}

	for _, comm := range comms {
		if !communityCheck.MatchString(comm) {
			return errors.Errorf("advertise.communities entry %s is not a valid community, example 65102:100", comm)
		}
		if base, _, _ := strings.Cut(comm, ":"); slices.Contains(reserved, base) {
			return errors.Errorf("advertise.communities entry %s is in a fabric-owned community namespace", comm)
		}
	}

	return nil
}

func (external *External) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(external); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	if len(external.Name) > 11 {
		return nil, errors.Errorf("name %s is too long, must be <= 11 characters", external.Name)
	}
	if external.Spec.IPv4Namespace == "" {
		return nil, errors.Errorf("IPv4Namespace is required")
	}

	if external.Spec.InboundCommunity != "" && !communityCheck.MatchString(external.Spec.InboundCommunity) {
		return nil, errors.Errorf("inboundCommunity %s is not a valid community, example 50000:50001", external.Spec.InboundCommunity)
	}

	if external.Spec.OutboundCommunity != "" && !communityCheck.MatchString(external.Spec.OutboundCommunity) {
		return nil, errors.Errorf("outboundCommunity %s is not a valid community, example 50000:50001", external.Spec.OutboundCommunity)
	}

	if external.Spec.Priority >= meta.MaxExtPrioLevels {
		return nil, errors.Errorf("priority must be less than %d", meta.MaxExtPrioLevels)
	}

	var advertisePrefixes []netip.Prefix
	if external.Spec.Advertise != nil {
		if err := validateAdvertiseCommunities(external.Spec.Advertise.Communities, fabricCfg); err != nil {
			return nil, err
		}
		for _, p := range external.Spec.Advertise.Prefixes {
			parsed, err := netip.ParsePrefix(p)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid prefix %s in advertise.prefixes", p)
			}
			// the session only carries the IPv4 AFI, and the prefix list it lands in is IPv4-only
			if !parsed.Addr().Is4() {
				return nil, errors.Errorf("advertise.prefixes entry %s is not IPv4", p)
			}
			advertisePrefixes = append(advertisePrefixes, parsed)
		}
	}

	if external.Spec.LocalASN != 0 && fabricCfg != nil {
		if external.Spec.LocalASN == fabricCfg.SpineASN {
			return nil, errors.Errorf("localASN %d is the fabric spine ASN", external.Spec.LocalASN)
		}
		if external.Spec.LocalASN >= fabricCfg.LeafASNStart && external.Spec.LocalASN <= fabricCfg.LeafASNEnd {
			return nil, errors.Errorf("localASN %d is inside the fabric leaf ASN range %d-%d", external.Spec.LocalASN, fabricCfg.LeafASNStart, fabricCfg.LeafASNEnd)
		}
	}

	if external.Spec.Static != nil {
		// a static external has no BGP session and so no liveness signal: a "primary" that dies
		// would keep attracting traffic
		if external.Spec.Priority != 0 {
			return nil, errors.Errorf("priority must not be set for static externals")
		}

		// and nothing to carry an announcement on either
		if external.Spec.Advertise != nil {
			return nil, errors.Errorf("advertise must not be set for static externals")
		}
		if external.Spec.LocalASN != 0 {
			return nil, errors.Errorf("localASN must not be set for static externals")
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

		for _, prefix := range advertisePrefixes {
			for _, subnet := range ipNs.Spec.Subnets {
				ipnsPrefix, err := netip.ParsePrefix(subnet)
				if err != nil {
					return nil, errors.Wrapf(err, "invalid subnet %s in IPv4Namespace %s", subnet, external.Spec.IPv4Namespace)
				}
				if ipnsPrefix.Overlaps(prefix) {
					return nil, errors.Errorf("advertise.prefixes entry %s is inside IPv4Namespace subnet %s, which is already advertised", prefix, subnet)
				}
			}
		}

		if external.Spec.LocalASN != 0 {
			attaches := &ExternalAttachmentList{}
			if err := kube.List(ctx, attaches, kclient.MatchingLabels{LabelExternal: external.Name}); err != nil {
				return nil, errors.Wrapf(err, "failed to list external attachments for %s", external.Name) // TODO replace with some internal error to not expose to the user
			}
			for _, attach := range attaches.Items {
				if attach.Spec.Neighbor.ASN == external.Spec.LocalASN {
					return nil, errors.Errorf("localASN %d is the neighbor ASN of external attachment %s", external.Spec.LocalASN, attach.Name)
				}
			}
		}
	}

	return nil, nil
}
