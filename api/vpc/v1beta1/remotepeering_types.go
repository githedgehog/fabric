// Copyright 2026 Hedgehog
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
	"maps"
	"net/netip"
	"slices"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// VPCInfoRPPrefix is the prefix of a RemotePeering's VPCInfo, which GatewayPeerings refer to it by
const VPCInfoRPPrefix = "rp."

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RemotePeeringTopology is where a RemotePeering sits in the fabric topology
type RemotePeeringTopology struct {
	// Fabric is the name of the Fabric this RemotePeering belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
	// Domain is the Fabric domain the RemotePeering is in (if not specified, "default" is used). Its links
	// must be on switches of that domain and its VPCs must be in it, and it is immutable
	Domain string `json:"domain,omitempty"`
}

// RemotePeeringLink is a BGP unnumbered session to the remote router on an External connection
type RemotePeeringLink struct {
	// Connection is the name of the External Connection the session runs over
	Connection string `json:"connection,omitempty"`
	// VLAN (optional) is the VLAN ID of the subinterface on the connection's switch port, 0 for no VLAN.
	// The remote end must use the same one
	VLAN uint16 `json:"vlan,omitempty"`
	// RemoteASN (optional) is the ASN of the remote router, if not set any ASN other than the switch's own is accepted
	RemoteASN uint32 `json:"remoteASN,omitempty"`
	// BFD (optional) enables BFD for the session, an empty object uses the fabric defaults
	// +optional
	BFD *ExternalAttachmentBFD `json:"bfd,omitempty"`
	// InboundACL (optional) defines the ACL statements to apply to inbound traffic on this link
	// +optional
	InboundACL *ACLSpec `json:"inboundACL,omitempty"`
}

// RemotePeeringVPC is a local VPC taking part in a RemotePeering
type RemotePeeringVPC struct {
	// Subnets are the names of the VPC subnets advertised to the remote side
	Subnets []string `json:"subnets,omitempty"`
}

// RemotePeeringRemote is what is accepted from the remote side
type RemotePeeringRemote struct {
	// Prefixes are the IPv4 prefixes accepted from the remote side. They match exactly, so
	// 0.0.0.0/0 only accepts the default route and 10.1.0.0/16 doesn't accept 10.1.1.0/24
	Prefixes []string `json:"prefixes,omitempty"`
}

// RemotePeeringSpec defines the desired state of RemotePeering
type RemotePeeringSpec struct {
	// Topology is where the RemotePeering sits in the fabric topology
	Topology RemotePeeringTopology `json:"topology,omitempty"`
	// IPv4Namespace is the IPv4Namespace of the local VPCs (if not specified, "default" is used)
	IPv4Namespace string `json:"ipv4Namespace,omitempty"`
	// Links are the BGP sessions to the remote router
	Links []RemotePeeringLink `json:"links,omitempty"`
	// Local are the local VPCs by name, with the subnets advertised to the remote side
	Local map[string]RemotePeeringVPC `json:"local,omitempty"`
	// Remote is what is accepted from the remote side
	Remote RemotePeeringRemote `json:"remote,omitempty"`
}

// RemotePeeringStatus defines the observed state of RemotePeering
type RemotePeeringStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric,shortName=rpeer
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.topology.domain`,priority=0
// +kubebuilder:printcolumn:name="IPv4NS",type=string,JSONPath=`.spec.ipv4Namespace`,priority=0
// +kubebuilder:printcolumn:name="Remote",type=string,JSONPath=`.spec.remote.prefixes`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// RemotePeering connects local VPCs to a remote router that hands off per VRF, such as a border leaf of
// another fabric. Every local VPC reaches every remote prefix, and nothing else: local VPCs don't reach
// each other through it, and only the listed subnets and prefixes are exchanged.
type RemotePeering struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the RemotePeering
	Spec RemotePeeringSpec `json:"spec,omitempty"`
	// Status is the observed state of the RemotePeering
	Status RemotePeeringStatus `json:"status,omitempty"`
}

const KindRemotePeering = "RemotePeering"

//+kubebuilder:object:root=true

// RemotePeeringList contains a list of RemotePeering
type RemotePeeringList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []RemotePeering `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &RemotePeering{}, &RemotePeeringList{})

		return nil
	})
}

var (
	_ meta.Object     = (*RemotePeering)(nil)
	_ meta.ObjectList = (*RemotePeeringList)(nil)
)

func (rpList *RemotePeeringList) GetItems() []meta.Object {
	items := make([]meta.Object, len(rpList.Items))
	for i := range rpList.Items {
		items[i] = &rpList.Items[i]
	}

	return items
}

func (rp *RemotePeering) Default() {
	meta.DefaultObjectMetadata(rp)

	if rp.Spec.Topology.Fabric == "" {
		rp.Spec.Topology.Fabric = wiringapi.DefaultFabric
	}
	if rp.Spec.Topology.Domain == "" {
		rp.Spec.Topology.Domain = wiringapi.DefaultFabricDomain
	}
	if rp.Spec.IPv4Namespace == "" {
		rp.Spec.IPv4Namespace = DefaultIPv4Namespace
	}

	if rp.Labels == nil {
		rp.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(rp.Labels)

	rp.Labels[LabelIPv4NS] = rp.Spec.IPv4Namespace
	rp.Labels[wiringapi.ListLabelFabric(rp.Spec.Topology.Fabric)] = ListLabelValue
	rp.Labels[wiringapi.ListLabelDomain(rp.Spec.Topology.Domain)] = ListLabelValue
	// an empty name would make an invalid label key, which the API server would report instead of the validation
	for vpcName, vpc := range rp.Spec.Local {
		if vpcName != "" {
			rp.Labels[ListLabelVPC(vpcName)] = ListLabelValue
		}
		slices.Sort(vpc.Subnets)
	}
	for _, link := range rp.Spec.Links {
		if link.Connection != "" {
			rp.Labels[wiringapi.ListLabelConnection(link.Connection)] = ListLabelValue
		}
	}
	slices.Sort(rp.Spec.Remote.Prefixes)
}

func (rp *RemotePeering) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	var warns admission.Warnings
	if err := meta.ValidateObjectMetadata(rp); err != nil {
		return nil, fmt.Errorf("failed to validate metadata: %w", err)
	}

	if err := wiringapi.CheckFabricExists(ctx, kube, rp.Namespace, rp.Spec.Topology.Fabric); err != nil {
		return nil, fmt.Errorf("failed to validate fabric: %w", err)
	}
	if rp.Spec.Topology.Domain == "" {
		return nil, fmt.Errorf("topology.domain is required") //nolint:err113
	}
	// the VRF is VrfR<name>, and SONiC VRF names are at most 15 characters
	if len(rp.Name) > 11 {
		return nil, fmt.Errorf("name %s is too long, must be <= 11 characters", rp.Name) //nolint:err113
	}
	if rp.Spec.IPv4Namespace == "" {
		return nil, fmt.Errorf("ipv4Namespace is required") //nolint:err113
	}
	if fabricCfg != nil && fabricCfg.LoopbackWorkaround {
		return nil, fmt.Errorf("remote peerings are not supported with the loopback workaround") //nolint:err113
	}

	if len(rp.Spec.Links) == 0 {
		return nil, fmt.Errorf("at least one link is required") //nolint:err113
	}
	type connVLAN struct {
		conn string
		vlan uint16
	}
	links := map[connVLAN]bool{}
	for idx, link := range rp.Spec.Links {
		if link.Connection == "" {
			return nil, fmt.Errorf("links[%d].connection is required", idx) //nolint:err113
		}
		key := connVLAN{link.Connection, link.VLAN}
		if links[key] {
			return nil, fmt.Errorf("connection %s is used twice with VLAN %d", link.Connection, link.VLAN) //nolint:err113
		}
		links[key] = true
		if err := link.BFD.Validate(); err != nil {
			return nil, fmt.Errorf("links[%d]: %w", idx, err)
		}
		if _, err := link.InboundACL.Validate(); err != nil {
			return nil, fmt.Errorf("links[%d].inboundACL: %w", idx, err)
		}
	}

	if len(rp.Spec.Local) == 0 {
		return nil, fmt.Errorf("at least one local VPC is required") //nolint:err113
	}
	for vpcName, vpc := range rp.Spec.Local {
		if vpcName == "" {
			return nil, fmt.Errorf("local VPC name is required") //nolint:err113
		}
		if len(vpc.Subnets) == 0 {
			return nil, fmt.Errorf("local VPC %s must list at least one subnet", vpcName) //nolint:err113
		}
		if len(slices.Compact(slices.Sorted(slices.Values(vpc.Subnets)))) != len(vpc.Subnets) {
			return nil, fmt.Errorf("local VPC %s lists a subnet twice", vpcName) //nolint:err113
		}
	}

	if len(rp.Spec.Remote.Prefixes) == 0 {
		return nil, fmt.Errorf("at least one remote prefix is required") //nolint:err113
	}
	remotePrefixes := []netip.Prefix{}
	for _, p := range rp.Spec.Remote.Prefixes {
		prefix, err := netip.ParsePrefix(p)
		if err != nil {
			return nil, fmt.Errorf("invalid remote prefix %s: %w", p, err)
		}
		if !prefix.Addr().Is4() {
			return nil, fmt.Errorf("remote prefix %s is not IPv4", p) //nolint:err113
		}
		// prefixes match exactly, so host bits would make one that never matches
		if prefix != prefix.Masked() {
			return nil, fmt.Errorf("remote prefix %s has host bits set, use %s", p, prefix.Masked()) //nolint:err113
		}
		if slices.Contains(remotePrefixes, prefix) {
			return nil, fmt.Errorf("remote prefix %s is listed twice", p) //nolint:err113
		}
		remotePrefixes = append(remotePrefixes, prefix)
	}

	if kube == nil {
		return warns, nil
	}

	rpFabric := rp.Spec.Topology.Fabric
	fabric, err := wiringapi.GetFabricSpec(ctx, kube, rp.Namespace, rpFabric)
	if err != nil {
		return nil, fmt.Errorf("failed to get fabric: %w", err)
	}
	domain := rp.Spec.Topology.Domain
	if _, exists := fabric.Domains[domain]; !exists {
		return nil, fmt.Errorf("domain %s not found in fabric %s, topology.domain must name one of its domains", domain, rpFabric) //nolint:err113
	}

	ipNs := &IPv4Namespace{}
	if err := kube.Get(ctx, ktypes.NamespacedName{Name: rp.Spec.IPv4Namespace, Namespace: rp.Namespace}, ipNs); err != nil {
		if kapierrors.IsNotFound(err) {
			return nil, fmt.Errorf("IPv4Namespace %s not found", rp.Spec.IPv4Namespace) //nolint:err113
		}

		return nil, fmt.Errorf("failed to get IPv4Namespace %s: %w", rp.Spec.IPv4Namespace, err) // TODO replace with some internal error to not expose to the user
	}
	if nsFabric := ipNs.Spec.Topology.Fabric; nsFabric != rpFabric {
		return nil, fmt.Errorf("remote peering is in fabric %s but its IPv4Namespace %s is in fabric %s", rpFabric, ipNs.Name, nsFabric) //nolint:err113
	}
	if err := rp.Spec.CheckNamespaceSubnets(ipNs.Name, ipNs.Spec.Subnets); err != nil {
		return nil, err
	}

	// the agent controller fails a switch getting a VPC whose VLAN namespace it doesn't have
	vlanNamespaces := map[string]string{}
	for vpcName := range rp.Spec.Local {
		vpc := &VPC{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: vpcName, Namespace: rp.Namespace}, vpc); err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, fmt.Errorf("vpc %s not found", vpcName) //nolint:err113
			}

			return nil, fmt.Errorf("failed to get vpc %s: %w", vpcName, err) // TODO replace with some internal error to not expose to the user
		}
		if vpcFabric := vpc.Spec.Topology.Fabric; vpcFabric != rpFabric {
			return nil, fmt.Errorf("remote peering is in fabric %s but vpc %s is in fabric %s", rpFabric, vpcName, vpcFabric) //nolint:err113
		}
		if vpcDomains := vpc.Spec.Topology.Domains; !slices.Contains(vpcDomains, domain) {
			return nil, fmt.Errorf("remote peering is in domain %s but vpc %s is in domains %v", domain, vpcName, vpcDomains) //nolint:err113
		}
		if err := rp.Spec.CheckLocalVPC(vpcName, &vpc.Spec); err != nil {
			return nil, err
		}
		vlanNamespaces[vpc.Spec.VLANNamespace] = vpcName
	}

	for idx, link := range rp.Spec.Links {
		conn := &wiringapi.Connection{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: link.Connection, Namespace: rp.Namespace}, conn); err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, fmt.Errorf("connection %s not found", link.Connection) //nolint:err113
			}

			return nil, fmt.Errorf("failed to get connection %s: %w", link.Connection, err) // TODO replace with some internal error to not expose to the user
		}
		if connFabric := conn.Spec.Topology.Fabric; connFabric != rpFabric {
			return nil, fmt.Errorf("remote peering is in fabric %s but connection %s is in fabric %s", rpFabric, link.Connection, connFabric) //nolint:err113
		}
		if conn.Spec.External == nil {
			return nil, fmt.Errorf("connection %s is not external", link.Connection) //nolint:err113
		}

		if link.RemoteASN != 0 {
			if what := asnCollision(fabric, link.RemoteASN); what != "" {
				return nil, fmt.Errorf("links[%d].remoteASN %d is %s of its own fabric", idx, link.RemoteASN, what) //nolint:err113
			}
		}
		if link.BFD != nil && fabric.DisableBFD {
			warns = append(warns, fmt.Sprintf("links[%d].bfd is ignored because disableBFD is set for the whole fabric", idx))
		}

		attaches := &ExternalAttachmentList{}
		if err := kube.List(ctx, attaches, kclient.InNamespace(rp.Namespace), kclient.MatchingLabels{wiringapi.LabelConnection: link.Connection}); err != nil {
			return nil, fmt.Errorf("failed to list external attachments for %s: %w", link.Connection, err) // TODO replace with some internal error to not expose to the user
		}
		for _, attach := range attaches.Items {
			if attach.VLAN() == link.VLAN {
				return nil, fmt.Errorf("connection %s already has external attachment %s with VLAN %d", link.Connection, attach.Name, link.VLAN) //nolint:err113
			}
		}

		rps := &RemotePeeringList{}
		if err := kube.List(ctx, rps, kclient.InNamespace(rp.Namespace), kclient.MatchingLabels{wiringapi.ListLabelConnection(link.Connection): ListLabelValue}); err != nil {
			return nil, fmt.Errorf("failed to list remote peerings for %s: %w", link.Connection, err) // TODO replace with some internal error to not expose to the user
		}
		for _, other := range rps.Items {
			if other.Name == rp.Name {
				continue
			}
			for _, otherLink := range other.Spec.Links {
				if otherLink.Connection == link.Connection && otherLink.VLAN == link.VLAN {
					return nil, fmt.Errorf("connection %s already has remote peering %s with VLAN %d", link.Connection, other.Name, link.VLAN) //nolint:err113
				}
			}
		}
	}

	// a leaf in two domains would merge both domains' routes into the one VRF
	connNames := []string{}
	for _, link := range rp.Spec.Links {
		connNames = append(connNames, link.Connection)
	}
	switches, err := ConnectionSwitches(ctx, kube, rp.Namespace, connNames)
	if err != nil {
		return nil, err
	}
	for _, name := range slices.Sorted(maps.Keys(switches)) {
		sw := switches[name]
		if swDomains := sw.Spec.Topology.Domains; !slices.Contains(swDomains, domain) {
			return nil, fmt.Errorf("remote peering is in domain %s but switch %s is in domains %v", domain, name, swDomains) //nolint:err113
		}
		for _, vlanNs := range slices.Sorted(maps.Keys(vlanNamespaces)) {
			if !slices.Contains(sw.Spec.VLANNamespaces, vlanNs) {
				return nil, fmt.Errorf("vpc %s is in VLAN namespace %s, which switch %s doesn't have", vlanNamespaces[vlanNs], vlanNs, name) //nolint:err113
			}
		}
	}

	return warns, nil
}

// CheckLocalVPC checks what the spec needs from a local VPC that can change after the RemotePeering is created
func (spec *RemotePeeringSpec) CheckLocalVPC(vpcName string, vpc *VPCSpec) error {
	if vpc.IPv4Namespace != spec.IPv4Namespace {
		return fmt.Errorf("vpc %s is in IPv4Namespace %s, not %s", vpcName, vpc.IPv4Namespace, spec.IPv4Namespace) //nolint:err113
	}
	for _, subnet := range spec.Local[vpcName].Subnets {
		if _, exists := vpc.Subnets[subnet]; !exists {
			return fmt.Errorf("vpc %s does not have subnet %s", vpcName, subnet) //nolint:err113
		}
	}

	return nil
}

// CheckNamespaceSubnets checks that no remote prefix is inside a subnet of the IPv4Namespace: the egress ACL on the
// links would drop the traffic to it, where an External drops the route silently. A prefix containing the namespace,
// such as a default route, is fine as the local routes are more specific
func (spec *RemotePeeringSpec) CheckNamespaceSubnets(nsName string, subnets []string) error {
	for _, s := range subnets {
		nsSubnet, err := netip.ParsePrefix(s)
		if err != nil {
			return fmt.Errorf("invalid subnet %s in IPv4Namespace %s: %w", s, nsName, err)
		}
		for _, p := range spec.Remote.Prefixes {
			prefix, err := netip.ParsePrefix(p)
			if err != nil {
				return fmt.Errorf("invalid remote prefix %s: %w", p, err)
			}
			if prefix.Bits() >= nsSubnet.Bits() && nsSubnet.Contains(prefix.Addr()) {
				return fmt.Errorf("remote prefix %s is inside subnet %s of IPv4Namespace %s", prefix, nsSubnet, nsName) //nolint:err113
			}
		}
	}

	return nil
}
