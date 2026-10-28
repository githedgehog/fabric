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
	"net"
	"net/netip"
	"slices"
	"strings"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TODO specify gateway explicitly?
// TODO rename VPCSubnet.Subnet to CIDR? or CIDRBlock like in AWS?

// VPCTopology is where a VPC sits in the fabric topology
type VPCTopology struct {
	// Fabric is the name of the Fabric this VPC belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
	// Domains are the Fabric domains the VPC is in (if not specified, "default" is used). It can only
	// be attached to switches that are in all of them, and they are immutable
	Domains []string `json:"domains,omitempty"`
}

// VPCSpec defines the desired state of VPC.
// At least one subnet is required.
type VPCSpec struct {
	// Mode is the VPC mode that defines how the VPCs are configured on the switches
	Mode VPCMode `json:"mode,omitempty"`
	// Subnets is the list of VPC subnets to configure
	Subnets map[string]*VPCSubnet `json:"subnets,omitempty"`
	// Topology is where the VPC sits in the fabric topology
	Topology VPCTopology `json:"topology,omitempty"`
	// IPv4Namespace is the name of the IPv4Namespace this VPC belongs to (if not specified, "default" is used)
	IPv4Namespace string `json:"ipv4Namespace,omitempty"`
	// VLANNamespace is the name of the VLANNamespace this VPC belongs to (if not specified, "default" is used)
	VLANNamespace string `json:"vlanNamespace,omitempty"`
	// DefaultIsolated sets default behavior for isolated mode for the subnets (disabled by default)
	DefaultIsolated bool `json:"defaultIsolated,omitempty"`
	// DefaultRestricted sets default behavior for restricted mode for the subnets (disabled by default)
	DefaultRestricted bool `json:"defaultRestricted,omitempty"`
	// Permit defines a list of the access policies between the subnets within the VPC - each policy is a list of subnets that have access to each other.
	// It's applied on top of the subnet isolation flag and if subnet isn't isolated it's not required to have it in a permit list while if vpc is marked
	// as isolated it's required to have it in a permit list to have access to other subnets.
	Permit [][]string `json:"permit,omitempty"`
	// StaticRoutes is the list of additional static routes for the VPC
	StaticRoutes []VPCStaticRoute `json:"staticRoutes,omitempty"`
}

// VPCMode defines how VPCs are implemented on the switches
type VPCMode string

const (
	// L2VNI: L2VPN EVPN with VRF per VPC and L2VNI per VPC subnet
	VPCModeL2VNI VPCMode = ""
	// L3VNI: L2VPN EVPN with VRF per VPC and only L3VNI per VPC, hosts should have /32 IP addresses in the VPC subnets
	VPCModeL3VNI VPCMode = "l3vni"
	// (EXPERIMENTAL) L3Flat: All VPCs are configured within default VRF and ACLs are used for isolation and VPC peerings, hosts should have /32 IP addresses in the VPC subnets
	VPCModeL3Flat VPCMode = "l3flat"
)

var VPCModes = []VPCMode{
	VPCModeL2VNI,
	VPCModeL3VNI,
	VPCModeL3Flat,
}

const (
	// DefaultHostBGPPrefixLen is the only prefix length accepted from hostBGP speakers unless configured otherwise
	DefaultHostBGPPrefixLen uint8 = 32
	// MaxHostBGPExtraPrefixes is the maximum number of extra prefixes allowed on a single hostBGP subnet
	MaxHostBGPExtraPrefixes = 100
)

// VPCSubnet defines the VPC subnet configuration
type VPCSubnet struct {
	// Subnet is the subnet CIDR block, such as "10.0.0.0/24", should belong to the IPv4Namespace and be unique within the namespace
	Subnet string `json:"subnet,omitempty"`
	// Gateway (optional) for the subnet, if not specified, the first IP (e.g. 10.0.0.1) in the subnet is used as the gateway
	Gateway string `json:"gateway,omitempty"`
	// DHCP is the on-demand DHCP configuration for the subnet
	DHCP VPCDHCP `json:"dhcp,omitempty"`
	// VLAN is the VLAN ID for the subnet, should belong to the VLANNamespace and be unique within the namespace
	VLAN uint16 `json:"vlan,omitempty"`
	// Isolated is the flag to enable isolated mode for the subnet which means no access to and from the other subnets within the VPC
	Isolated *bool `json:"isolated,omitempty"`
	// Restricted is the flag to enable restricted mode for the subnet which means no access between hosts within the subnet itself
	Restricted *bool `json:"restricted,omitempty"`
	// HostBGP is the flag to set this Subnet as dedicated to BGP speaking hosts advertising their VIPs within the subnet's IP range
	HostBGP bool `json:"hostBGP,omitempty"`
	// HostBGPMinPrefixLen is the shortest prefix length accepted from the host within the subnet's IP range, 32 by default
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=32
	HostBGPMinPrefixLen uint8 `json:"hostBGPMinPrefixLen,omitempty"`
	// HostBGPMaxPrefixLen is the longest prefix length accepted from the host within the subnet's IP range, 32 by default
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=32
	HostBGPMaxPrefixLen uint8 `json:"hostBGPMaxPrefixLen,omitempty"`
	// HostBGPExtraPrefixes is the set of extra IP prefixes, on top of the subnet's own IP range, to accept from the
	// host over its BGP sessions, keyed by prefix
	HostBGPExtraPrefixes map[string]VPCSubnetHostBGPPrefix `json:"hostBGPExtraPrefixes,omitempty"`
}

// VPCSubnetHostBGPPrefix defines the prefix lengths accepted from the host within an extra hostBGP prefix
type VPCSubnetHostBGPPrefix struct {
	// MinPrefixLen is the shortest prefix length accepted within the prefix, defaults to the subnet's hostBGPMinPrefixLen
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=32
	MinPrefixLen uint8 `json:"minPrefixLen,omitempty"`
	// MaxPrefixLen is the longest prefix length accepted within the prefix, defaults to the subnet's hostBGPMaxPrefixLen
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=32
	MaxPrefixLen uint8 `json:"maxPrefixLen,omitempty"`
}

// HostBGPPrefixLens returns the shortest and longest prefix length accepted from the host within
// the subnet's own IP range
func (subnet *VPCSubnet) HostBGPPrefixLens() (uint8, uint8) {
	minLen, maxLen := subnet.HostBGPMinPrefixLen, subnet.HostBGPMaxPrefixLen
	if minLen == 0 {
		minLen = DefaultHostBGPPrefixLen
	}
	if maxLen == 0 {
		maxLen = DefaultHostBGPPrefixLen
	}

	return minLen, maxLen
}

// HostBGPExtraPrefixLens returns the shortest and longest prefix length accepted from the host within
// the given extra prefix, falling back to the subnet-level configuration when unset
func (subnet *VPCSubnet) HostBGPExtraPrefixLens(prefixCfg VPCSubnetHostBGPPrefix) (uint8, uint8) {
	minLen, maxLen := subnet.HostBGPPrefixLens()
	if prefixCfg.MinPrefixLen != 0 {
		minLen = prefixCfg.MinPrefixLen
	}
	if prefixCfg.MaxPrefixLen != 0 {
		maxLen = prefixCfg.MaxPrefixLen
	}

	return minLen, maxLen
}

// VPCDHCP defines the on-demand DHCP configuration for the subnet
type VPCDHCP struct {
	// Relay is the DHCP relay IP address, if specified, DHCP server will be disabled
	Relay string `json:"relay,omitempty"`
	// RelayVPC (optional) is the name of the VPC where the selected DHCP relay lives, requires Relay. It has to be a VPC
	// this one could peer with: same fabric, at least one shared domain, same IPv4 and VLAN namespaces.
	RelayVPC string `json:"relayVPC,omitempty"`
	// Enable enables DHCP server for the subnet
	Enable bool `json:"enable,omitempty"`
	// Range (optional) is the DHCP range for the subnet if DHCP server is enabled. The gateway, subnet and broadcast
	// addresses are never handed out, even if the range includes them (all addresses of a /31 or /32 are usable).
	Range *VPCDHCPRange `json:"range,omitempty"`
	// Options (optional) is the DHCP options for the subnet if DHCP server is enabled
	Options *VPCDHCPOptions `json:"options,omitempty"`
	// Static is a map of static IP assignments for MAC addresses
	Static map[string]VPCDHCPStatic `json:"static,omitempty"`
}

// VPCDHCPStatic represents static IP assignment
type VPCDHCPStatic struct {
	// IP is the assigned static IP address
	IP string `json:"ip"`
}

// VPCDHCPRange defines the DHCP range for the subnet if DHCP server is enabled
type VPCDHCPRange struct {
	// Start is the start IP address of the DHCP range
	Start string `json:"start,omitempty"`
	// End is the end IP address of the DHCP range
	End string `json:"end,omitempty"`
}

type VPCDHCPRoute struct {
	// Destination is the destination prefix for the route
	Destination string `json:"destination,omitempty"`
	// Gateway is the gateway IP address for the route
	Gateway string `json:"gateway,omitempty"`
}

// sorting function for VPCDHCPRoute
func VPCDHCPRouteCompare(a, b VPCDHCPRoute) int {
	if a.Destination < b.Destination {
		return -1
	} else if a.Destination > b.Destination {
		return 1
	}
	if a.Gateway < b.Gateway {
		return -1
	} else if a.Gateway > b.Gateway {
		return 1
	}

	return 0
}

// VPCDHCPOptions defines the DHCP options for the subnet if DHCP server is enabled
type VPCDHCPOptions struct {
	// PXEURL (optional) to identify the pxe server to use to boot hosts connected to this segment such as http://10.10.10.99/bootfilename or tftp://10.10.10.99/bootfilename, http query strings are not supported
	PXEURL string `json:"pxeURL,omitempty"`
	// +kubebuilder:validation:Optional
	// DNSservers (optional) to configure Domain Name Servers for this particular segment such as: 10.10.10.1, 10.10.10.2
	DNSServers []string `json:"dnsServers"`
	// +kubebuilder:validation:Optional
	// TimeServers (optional) NTP server addresses to configure for time servers for this particular segment such as: 10.10.10.1, 10.10.10.2
	TimeServers []string `json:"timeServers"`
	// +kubebuilder:validation:Minimum: 96
	// +kubebuilder:validation:Maximum: 9036
	// InterfaceMTU (optional) is the MTU setting that the dhcp server will send to the clients. It is dependent on the client to honor this option.
	InterfaceMTU uint16 `json:"interfaceMTU"`
	// Lease time in seconds, such as 3600
	// +kubebuilder:validation:Minimum: 1
	LeaseTimeSeconds uint32 `json:"leaseTimeSeconds"`
	// Disable default route advertisement. For L3VNI VPCs, a classless static route to the VPC subnet
	// will be advertised if this option is enabled.
	DisableDefaultRoute bool `json:"disableDefaultRoute,omitempty"`
	// Advertise custom routes to the clients via the classless static route option. If non-empty,
	// and unless the disable default route flag is enabled, a default route via the VPC gateway
	// will be added automatically.
	// +kubebuilder:validation:Optional
	AdvertisedRoutes []VPCDHCPRoute `json:"advertisedRoutes,omitempty"`
}

// VPCStaticRoute defines the static route for the VPC
type VPCStaticRoute struct {
	// Prefix for the static route (mandatory), e.g. 10.42.0.0/24
	Prefix string `json:"prefix,omitempty"`
	// NextHops for the static route (at least one is required), e.g. 10.99.0.0
	NextHops []string `json:"nextHops,omitempty"`
}

// VPCStatus defines the observed state of VPC
type VPCStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
// +kubebuilder:printcolumn:name="Domains",type=string,JSONPath=`.spec.topology.domains`,priority=0
// +kubebuilder:printcolumn:name="IPv4NS",type=string,JSONPath=`.spec.ipv4Namespace`,priority=0
// +kubebuilder:printcolumn:name="VLANNS",type=string,JSONPath=`.spec.vlanNamespace`,priority=0
// +kubebuilder:printcolumn:name="Subnets",type=string,JSONPath=`.spec.subnets`,priority=1
// +kubebuilder:printcolumn:name="VNI",type=string,JSONPath=`.status.vni`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// VPC is Virtual Private Cloud, similar to the public cloud VPC it provides an isolated private network for the
// resources with support for multiple subnets each with user-provided VLANs and on-demand DHCP.
type VPC struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the VPC
	Spec VPCSpec `json:"spec,omitempty"`
	// Status is the observed state of the VPC
	Status VPCStatus `json:"status,omitempty"`
}

const KindVPC = "VPC"

// +kubebuilder:object:root=true

// VPCList contains a list of VPC
type VPCList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []VPC `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &VPC{}, &VPCList{})

		return nil
	})
}

var (
	_ meta.Object     = (*VPC)(nil)
	_ meta.ObjectList = (*VPCList)(nil)
)

func (vpcList *VPCList) GetItems() []meta.Object {
	items := make([]meta.Object, len(vpcList.Items))
	for i := range vpcList.Items {
		items[i] = &vpcList.Items[i]
	}

	return items
}

func (vpc *VPCSpec) IsSubnetIsolated(subnetName string) bool {
	if subnet, ok := vpc.Subnets[subnetName]; ok && subnet.Isolated != nil {
		return *subnet.Isolated
	}

	return vpc.DefaultIsolated
}

func (vpc *VPCSpec) IsSubnetRestricted(subnetName string) bool {
	if subnet, ok := vpc.Subnets[subnetName]; ok && subnet.Restricted != nil {
		return *subnet.Restricted
	}

	return vpc.DefaultRestricted
}

func (vpc *VPC) Default() {
	meta.DefaultObjectMetadata(vpc)

	if vpc.Spec.Topology.Fabric == "" {
		vpc.Spec.Topology.Fabric = wiringapi.DefaultFabric
	}
	if len(vpc.Spec.Topology.Domains) == 0 {
		vpc.Spec.Topology.Domains = []string{wiringapi.DefaultFabricDomain}
	}
	slices.Sort(vpc.Spec.Topology.Domains)
	if vpc.Spec.IPv4Namespace == "" {
		vpc.Spec.IPv4Namespace = DefaultIPv4Namespace
	}
	if vpc.Spec.VLANNamespace == "" {
		vpc.Spec.VLANNamespace = wiringapi.DefaultVLANNamespace
	}

	if vpc.Labels == nil {
		vpc.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(vpc.Labels)

	vpc.Labels[LabelIPv4NS] = vpc.Spec.IPv4Namespace
	vpc.Labels[LabelVLANNS] = vpc.Spec.VLANNamespace
	vpc.Labels[wiringapi.ListLabelFabric(vpc.Spec.Topology.Fabric)] = ListLabelValue
	for _, domain := range vpc.Spec.Topology.Domains {
		// validation rejects an empty name, but as a label key it would be refused first with a vaguer error
		if domain != "" {
			vpc.Labels[wiringapi.ListLabelDomain(domain)] = ListLabelValue
		}
	}

	for _, subnet := range vpc.Spec.Subnets {
		cidr, err := iputil.ParseCIDR(subnet.Subnet)
		if err != nil {
			continue
		}

		if subnet.Gateway == "" && !subnet.HostBGP {
			subnet.Gateway = cidr.Gateway.String()
		}

		if prefixLen := cidr.Subnet.Bits(); prefixLen > 30 {
			continue
		}

		if !subnet.DHCP.Enable {
			subnet.DHCP.Range = nil
			subnet.DHCP.Options = nil

			continue
		}

		if subnet.DHCP.Range == nil {
			subnet.DHCP.Range = &VPCDHCPRange{}
		}

		start := cidr.DHCPRangeStart.String()
		if subnet.DHCP.Range.Start == "" {
			subnet.DHCP.Range.Start = start
		}

		end := cidr.DHCPRangeEnd.String()
		if subnet.DHCP.Range.End == "" {
			subnet.DHCP.Range.End = end
		}

		if subnet.DHCP.Options != nil {
			if subnet.DHCP.Options.PXEURL == "" && subnet.DHCP.Options.DNSServers == nil &&
				subnet.DHCP.Options.TimeServers == nil && subnet.DHCP.Options.InterfaceMTU == 0 &&
				subnet.DHCP.Options.LeaseTimeSeconds == 0 && !subnet.DHCP.Options.DisableDefaultRoute &&
				subnet.DHCP.Options.AdvertisedRoutes == nil {
				subnet.DHCP.Options = nil

				continue
			}

			if subnet.DHCP.Options.InterfaceMTU == 0 {
				subnet.DHCP.Options.InterfaceMTU = DefaultDHCPMTU
			}

			if subnet.DHCP.Options.DNSServers == nil {
				subnet.DHCP.Options.DNSServers = []string{}
			}
			slices.Sort(subnet.DHCP.Options.DNSServers)

			if subnet.DHCP.Options.TimeServers == nil {
				subnet.DHCP.Options.TimeServers = []string{}
			}
			slices.Sort(subnet.DHCP.Options.TimeServers)

			if subnet.DHCP.Options.LeaseTimeSeconds == 0 {
				subnet.DHCP.Options.LeaseTimeSeconds = DefaultDHCPLeaseTimeSeconds
			}

			if subnet.DHCP.Options.AdvertisedRoutes == nil {
				subnet.DHCP.Options.AdvertisedRoutes = []VPCDHCPRoute{}
			}
			slices.SortStableFunc(subnet.DHCP.Options.AdvertisedRoutes, VPCDHCPRouteCompare)
		}

		// normalize MAC addresses
		for mac, static := range subnet.DHCP.Static {
			prMAC, err := net.ParseMAC(mac)
			if err != nil {
				continue
			}

			if prMAC.String() != mac {
				delete(subnet.DHCP.Static, mac)
				subnet.DHCP.Static[prMAC.String()] = static
			}
		}
	}
}

func (vpc *VPC) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(vpc); err != nil {
		return nil, err //nolint:wrapcheck
	}

	spec := field.NewPath("spec")
	topology := spec.Child("topology")

	if err := wiringapi.CheckFabricExists(ctx, kube, vpc.Namespace, vpc.Spec.Topology.Fabric); err != nil {
		return nil, field.Invalid(topology.Child("fabric"), vpc.Spec.Topology.Fabric, err.Error())
	}

	domains := slices.Sorted(slices.Values(vpc.Spec.Topology.Domains))
	if len(domains) == 0 {
		return nil, field.Required(topology.Child("domains"), "at least one domain is required")
	}
	if slices.Contains(domains, "") {
		return nil, field.Invalid(topology.Child("domains"), vpc.Spec.Topology.Domains, "domain name cannot be empty")
	}
	if len(slices.Compact(slices.Clone(domains))) != len(domains) {
		return nil, field.Invalid(topology.Child("domains"), vpc.Spec.Topology.Domains, "domains must be unique")
	}

	if len(vpc.Name) > 11 {
		return nil, field.TooLong(field.NewPath("metadata", "name"), vpc.Name, 11)
	}
	if strings.HasPrefix(vpc.Name, VPCInfoExtPrefix) {
		return nil, field.Invalid(field.NewPath("metadata", "name"), vpc.Name, fmt.Sprintf("cannot start with %q", VPCInfoExtPrefix))
	}
	if vpc.Spec.IPv4Namespace == "" {
		return nil, field.Required(spec.Child("ipv4Namespace"), "")
	}
	if vpc.Spec.VLANNamespace == "" {
		return nil, field.Required(spec.Child("vlanNamespace"), "")
	}
	if len(vpc.Spec.Subnets) == 0 {
		return nil, field.Required(spec.Child("subnets"), "at least one subnet is required")
	}
	if len(vpc.Spec.Subnets) > 20 {
		return nil, field.TooMany(spec.Child("subnets"), len(vpc.Spec.Subnets), 20)
	}
	// TODO remove L3Flat mode or make experimental flag for it
	if supported := []VPCMode{VPCModeL2VNI, VPCModeL3VNI}; !slices.Contains(supported, vpc.Spec.Mode) {
		return nil, field.NotSupported(spec.Child("mode"), vpc.Spec.Mode, supported)
	}

	subnets := []netip.Prefix{}
	vlans := map[uint16]bool{}
	hostBGPSubnets := 0
	for subnetName, subnetCfg := range vpc.Spec.Subnets {
		subnetPath := spec.Child("subnets").Key(subnetName)

		// the name is used in labels and in the name of the subnet's DHCPSubnet, as <vpc>--<subnet>
		if err := meta.ValidateName(spec.Child("subnets").Key(subnetName), subnetName); err != nil {
			return nil, err //nolint:wrapcheck
		}
		if subnetCfg.Subnet == "" {
			return nil, field.Required(subnetPath.Child("subnet"), "")
		}

		ipNet, err := netip.ParsePrefix(subnetCfg.Subnet)
		if err != nil {
			return nil, field.Invalid(subnetPath.Child("subnet"), subnetCfg.Subnet, err.Error())
		}

		if prefixLen := ipNet.Bits(); prefixLen > 30 {
			return nil, field.Invalid(subnetPath.Child("subnet"), subnetCfg.Subnet, "prefix length must be <= 30")
		}

		if fabricCfg != nil {
			for _, reserved := range fabricCfg.ParsedReservedSubnets() {
				if reserved.Overlaps(ipNet) {
					return nil, field.Invalid(subnetPath.Child("subnet"), subnetCfg.Subnet, "overlaps with reserved subnet "+reserved.String())
				}
			}
		}

		var gateway netip.Addr
		if subnetCfg.HostBGP {
			hostBGPSubnets++
			if subnetCfg.DHCP.Enable {
				return nil, field.Forbidden(subnetPath.Child("dhcp", "enable"), "dhcp should not be enabled for hostBGP subnets")
			}
			minLen, maxLen := subnetCfg.HostBGPPrefixLens()
			if err := validateHostBGPPrefixLens(ipNet, minLen, maxLen); err != nil {
				return nil, field.Invalid(subnetPath, field.OmitValueType{}, err.Error())
			}

			extraPrefixesPath := subnetPath.Child("hostBGPExtraPrefixes")
			if len(subnetCfg.HostBGPExtraPrefixes) > MaxHostBGPExtraPrefixes {
				return nil, field.TooMany(extraPrefixesPath, len(subnetCfg.HostBGPExtraPrefixes), MaxHostBGPExtraPrefixes)
			}
			for prefixStr, prefixCfg := range subnetCfg.HostBGPExtraPrefixes {
				p, err := netip.ParsePrefix(prefixStr)
				if err != nil {
					return nil, field.Invalid(extraPrefixesPath, prefixStr, err.Error())
				}
				if !p.Addr().Is4() {
					return nil, field.Invalid(extraPrefixesPath, prefixStr, "prefix is not a valid IPv4 prefix")
				}
				if p.Addr() != p.Masked().Addr() {
					return nil, field.Invalid(extraPrefixesPath, prefixStr, "prefix is not in canonical form")
				}

				extraMinLen, extraMaxLen := subnetCfg.HostBGPExtraPrefixLens(prefixCfg)
				if err := validateHostBGPPrefixLens(p, extraMinLen, extraMaxLen); err != nil {
					return nil, field.Invalid(extraPrefixesPath.Key(prefixStr), field.OmitValueType{}, err.Error())
				}

				if fabricCfg != nil {
					for _, reserved := range fabricCfg.ParsedReservedSubnets() {
						if reserved.Overlaps(p) {
							return nil, field.Invalid(extraPrefixesPath, prefixStr, "overlaps with reserved subnet "+reserved.String())
						}
					}
				}

				subnets = append(subnets, p)
			}
		} else {
			if len(subnetCfg.HostBGPExtraPrefixes) > 0 {
				return nil, field.Forbidden(subnetPath.Child("hostBGPExtraPrefixes"), "cannot specify hostBGP extra prefixes on non-hostBGP subnet")
			}
			if subnetCfg.HostBGPMinPrefixLen != 0 || subnetCfg.HostBGPMaxPrefixLen != 0 {
				return nil, field.Forbidden(subnetPath, "cannot specify hostBGP prefix lengths on non-hostBGP subnet")
			}
			if subnetCfg.Gateway == "" {
				return nil, field.Required(subnetPath.Child("gateway"), "")
			}

			gateway, err = netip.ParseAddr(subnetCfg.Gateway)
			if err != nil {
				return nil, field.Invalid(subnetPath.Child("gateway"), subnetCfg.Gateway, "not a valid IP address")
			}
			if !ipNet.Contains(gateway) {
				return nil, field.Invalid(subnetPath.Child("gateway"), subnetCfg.Gateway, "not in the subnet")
			}

			if subnetCfg.VLAN == 0 {
				return nil, field.Required(subnetPath.Child("vlan"), "")
			}
			vlans[subnetCfg.VLAN] = true
		}

		subnets = append(subnets, ipNet)

		dhcpPath := subnetPath.Child("dhcp")
		if subnetCfg.DHCP.Relay != "" && subnetCfg.DHCP.Enable {
			return nil, field.Forbidden(dhcpPath.Child("relay"), "dhcp relay and dhcp server cannot be enabled at the same time")
		}

		if subnetCfg.DHCP.Relay != "" {
			_, err := netip.ParsePrefix(subnetCfg.DHCP.Relay)
			if err != nil {
				return nil, field.Invalid(dhcpPath.Child("relay"), subnetCfg.DHCP.Relay, err.Error())
			}
		} else if subnetCfg.DHCP.RelayVPC != "" {
			return nil, field.Forbidden(dhcpPath.Child("relayVPC"), "dhcp relay VPC specified but dhcp relay is not enabled")
		}

		optionsPath := dhcpPath.Child("options")
		if subnetCfg.DHCP.Options != nil && !subnetCfg.DHCP.Enable {
			if subnetCfg.DHCP.Options.PXEURL != "" {
				return nil, field.Forbidden(optionsPath.Child("pxeURL"), "dhcp is disabled")
			}

			if len(subnetCfg.DHCP.Options.DNSServers) > 0 {
				return nil, field.Forbidden(optionsPath.Child("dnsServers"), "dhcp is disabled")
			}

			if len(subnetCfg.DHCP.Options.TimeServers) > 0 {
				return nil, field.Forbidden(optionsPath.Child("timeServers"), "dhcp is disabled")
			}

			if subnetCfg.DHCP.Options.InterfaceMTU > 0 {
				return nil, field.Forbidden(optionsPath.Child("interfaceMTU"), "dhcp is disabled")
			}

			if subnetCfg.DHCP.Options.LeaseTimeSeconds > 0 {
				return nil, field.Forbidden(optionsPath.Child("leaseTimeSeconds"), "dhcp is disabled")
			}

			if subnetCfg.DHCP.Options.DisableDefaultRoute {
				return nil, field.Forbidden(optionsPath.Child("disableDefaultRoute"), "dhcp is disabled")
			}

			if len(subnetCfg.DHCP.Options.AdvertisedRoutes) > 0 {
				return nil, field.Forbidden(optionsPath.Child("advertisedRoutes"), "dhcp is disabled")
			}
		}

		rangePath := dhcpPath.Child("range")
		if subnetCfg.DHCP.Enable {
			if subnetCfg.DHCP.Range == nil {
				return nil, field.Required(rangePath, "")
			}
			if subnetCfg.DHCP.Range.Start == "" {
				return nil, field.Required(rangePath.Child("start"), "")
			}

			start, err := netip.ParseAddr(subnetCfg.DHCP.Range.Start)
			if err != nil {
				return nil, field.Invalid(rangePath.Child("start"), subnetCfg.DHCP.Range.Start, "not a valid IP address")
			}
			if start == ipNet.Masked().Addr() {
				return nil, field.Invalid(rangePath.Child("start"), subnetCfg.DHCP.Range.Start, "equal to subnet")
			}
			if start == gateway {
				return nil, field.Invalid(rangePath.Child("start"), subnetCfg.DHCP.Range.Start, "equal to gateway")
			}
			if !ipNet.Contains(start) {
				return nil, field.Invalid(rangePath.Child("start"), subnetCfg.DHCP.Range.Start, "not in the subnet")
			}

			if subnetCfg.DHCP.Range.End == "" {
				return nil, field.Required(rangePath.Child("end"), "")
			}

			end, err := netip.ParseAddr(subnetCfg.DHCP.Range.End)
			if err != nil {
				return nil, field.Invalid(rangePath.Child("end"), subnetCfg.DHCP.Range.End, "not a valid IP address")
			}
			if end == ipNet.Masked().Addr() {
				return nil, field.Invalid(rangePath.Child("end"), subnetCfg.DHCP.Range.End, "equal to subnet")
			}
			if end == gateway {
				return nil, field.Invalid(rangePath.Child("end"), subnetCfg.DHCP.Range.End, "equal to gateway")
			}
			if !ipNet.Contains(end) {
				return nil, field.Invalid(rangePath.Child("end"), subnetCfg.DHCP.Range.End, "not in the subnet")
			}

			if !start.Less(end) {
				return nil, field.Invalid(rangePath.Child("end"), subnetCfg.DHCP.Range.End, "must come after range start "+subnetCfg.DHCP.Range.Start)
			}

			if subnetCfg.DHCP.Options != nil {
				if subnetCfg.DHCP.Options.InterfaceMTU < 96 {
					return nil, field.Invalid(optionsPath.Child("interfaceMTU"), subnetCfg.DHCP.Options.InterfaceMTU, "cannot be smaller than 96")
				}
				if subnetCfg.DHCP.Options.InterfaceMTU > 9036 {
					return nil, field.Invalid(optionsPath.Child("interfaceMTU"), subnetCfg.DHCP.Options.InterfaceMTU, "cannot be greater than 9036")
				}
				if subnetCfg.DHCP.Options.LeaseTimeSeconds < 1 {
					return nil, field.Invalid(optionsPath.Child("leaseTimeSeconds"), subnetCfg.DHCP.Options.LeaseTimeSeconds, "cannot be smaller than 1")
				}
				// TODO: max lease time?
			}

			// same checks as the DHCP server does on the DHCPSubnet, so an accepted VPC is always served
			dhcpSpec := vpc.DHCPSubnetSpec(subnetName, subnetCfg)
			if err := dhcpSpec.Validate(); err != nil {
				return nil, field.Invalid(dhcpPath, field.OmitValueType{}, err.Error())
			}
		}

		if !subnetCfg.DHCP.Enable {
			if subnetCfg.DHCP.Range != nil && (subnetCfg.DHCP.Range.Start != "" || subnetCfg.DHCP.Range.End != "") {
				return nil, field.Forbidden(rangePath, "dhcp is disabled")
			}

			if len(subnetCfg.DHCP.Static) > 0 {
				return nil, field.Forbidden(dhcpPath.Child("static"), "dhcp is disabled")
			}
		}
	}

	if len(vlans) != len(vpc.Spec.Subnets)-hostBGPSubnets {
		return nil, field.Invalid(spec.Child("subnets"), field.OmitValueType{}, "duplicate subnet VLANs")
	}

	if err := iputil.VerifyNoOverlapNetip(subnets); err != nil {
		return nil, field.Invalid(spec.Child("subnets"), field.OmitValueType{}, err.Error())
	}

	for permitIdx, permit := range vpc.Spec.Permit {
		permitPath := spec.Child("permit").Index(permitIdx)
		if len(permit) < 2 {
			return nil, field.Invalid(permitPath, permit, "must have at least 2 subnets")
		}

		subnets := map[string]bool{}
		for _, subnetName := range permit {
			if _, ok := vpc.Spec.Subnets[subnetName]; !ok {
				return nil, field.NotFound(permitPath, subnetName)
			}

			subnets[subnetName] = true
		}

		if len(subnets) != len(permit) {
			return nil, field.Invalid(permitPath, permit, "duplicate subnets")
		}
	}

	for idx, staticRoute := range vpc.Spec.StaticRoutes {
		routePath := spec.Child("staticRoutes").Index(idx)
		if staticRoute.Prefix == "" {
			return nil, field.Required(routePath.Child("prefix"), "")
		}

		staticNet, err := netip.ParsePrefix(staticRoute.Prefix)
		if err != nil {
			return nil, field.Invalid(routePath.Child("prefix"), staticRoute.Prefix, err.Error())
		}

		if staticNet.Addr() != staticNet.Masked().Addr() {
			return nil, field.Invalid(routePath.Child("prefix"), staticRoute.Prefix, "inconsistent IP address and mask")
		}

		if len(staticRoute.NextHops) == 0 {
			return nil, field.Required(routePath.Child("nextHops"), "at least one next hop is required")
		}
	}

	if kube != nil {
		// TODO Can we rely on Validation webhook for cross VPC subnet? if not - main VPC subnet validation should happen in the VPC controller

		ipNs := &IPv4Namespace{}
		err := kube.Get(ctx, ktypes.NamespacedName{Name: vpc.Spec.IPv4Namespace, Namespace: vpc.Namespace}, ipNs)
		if err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, field.NotFound(spec.Child("ipv4Namespace"), vpc.Spec.IPv4Namespace)
			}

			return nil, fmt.Errorf("failed to get IPv4Namespace %s: %w", vpc.Spec.IPv4Namespace, err) // TODO replace with some internal error to not expose to the user
		}

		vpcFabric := vpc.Spec.Topology.Fabric
		if nsFabric := ipNs.Spec.Topology.Fabric; nsFabric != vpcFabric {
			return nil, field.Invalid(spec.Child("ipv4Namespace"), vpc.Spec.IPv4Namespace, fmt.Sprintf("vpc is in fabric %s but the IPv4Namespace is in fabric %s", vpcFabric, nsFabric))
		}

		fabric, err := wiringapi.GetFabricSpec(ctx, kube, vpc.Namespace, vpc.Spec.Topology.Fabric)
		if err != nil {
			return nil, fmt.Errorf("failed to get fabric: %w", err)
		}
		for _, domain := range domains {
			if _, exists := fabric.Domains[domain]; !exists {
				return nil, field.Invalid(topology.Child("domains"), domain, fmt.Sprintf("domain %s not found in fabric %s", domain, vpcFabric))
			}
		}

		vlanNs := &wiringapi.VLANNamespace{}
		err = kube.Get(ctx, ktypes.NamespacedName{Name: vpc.Spec.VLANNamespace, Namespace: vpc.Namespace}, vlanNs)
		if err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, field.NotFound(spec.Child("vlanNamespace"), vpc.Spec.VLANNamespace)
			}

			return nil, fmt.Errorf("failed to get VLANNamespace %s: %w", vpc.Spec.VLANNamespace, err) // TODO replace with some internal error to not expose to the user
		}

		for subnetName, subnetCfg := range vpc.Spec.Subnets {
			subnetPath := spec.Child("subnets").Key(subnetName)

			vpcSubnet, err := netip.ParsePrefix(subnetCfg.Subnet)
			if err != nil {
				return nil, fmt.Errorf("failed to parse vpc subnet %s: %w", subnetCfg.Subnet, err)
			}

			ok, err := isInIPv4Namespace(vpcSubnet, ipNs)
			if err != nil {
				return nil, fmt.Errorf("failed to check if vpc subnet %s (%s) belongs to IPv4Namespace %s: %w", subnetName, subnetCfg.Subnet, vpc.Spec.IPv4Namespace, err)
			}
			if !ok {
				return nil, field.Invalid(subnetPath.Child("subnet"), subnetCfg.Subnet, "doesn't belong to IPv4Namespace "+vpc.Spec.IPv4Namespace)
			}

			for hbgpPrefixStr := range subnetCfg.HostBGPExtraPrefixes {
				hbgpPrefix, err := netip.ParsePrefix(hbgpPrefixStr)
				if err != nil {
					return nil, fmt.Errorf("failed to parse hostBGP extra prefix %s in vpc subnet %s (%s): %w", hbgpPrefixStr, subnetName, subnetCfg.Subnet, err)
				}
				ok, err := isInIPv4Namespace(hbgpPrefix, ipNs)
				if err != nil {
					return nil, fmt.Errorf("failed to check if hostBGP extra prefix %s in vpc subnet %s (%s) belongs to IPv4Namespace %s: %w", hbgpPrefixStr, subnetName, subnetCfg.Subnet, ipNs.Name, err)
				}
				if !ok {
					return nil, field.Invalid(subnetPath.Child("hostBGPExtraPrefixes"), hbgpPrefixStr, "doesn't belong to IPv4Namespace "+ipNs.Name)
				}
			}

			if !subnetCfg.HostBGP && !vlanNs.Spec.Contains(subnetCfg.VLAN) {
				return nil, field.Invalid(subnetPath.Child("vlan"), subnetCfg.VLAN, "doesn't belong to VLANNamespace "+vpc.Spec.VLANNamespace)
			}

			if subnetCfg.DHCP.RelayVPC != "" {
				relayVPCPath := subnetPath.Child("dhcp", "relayVPC")
				relayVPC := &VPC{}
				err := kube.Get(ctx, ktypes.NamespacedName{Name: subnetCfg.DHCP.RelayVPC, Namespace: vpc.Namespace}, relayVPC)
				if err != nil {
					if kapierrors.IsNotFound(err) {
						return nil, field.NotFound(relayVPCPath, subnetCfg.DHCP.RelayVPC)
					}

					return nil, fmt.Errorf("subnet %s: failed to get dhcp relay VPC %s: %w", subnetName, subnetCfg.DHCP.RelayVPC, err) // TODO replace with some internal error to not expose to the user
				}

				// the relay VPC is built on the switches the subnet is attached to, so it has to be one the VPC could
				// peer with
				if relayFabric := relayVPC.Spec.Topology.Fabric; relayFabric != vpcFabric {
					return nil, field.Invalid(relayVPCPath, relayVPC.Name, fmt.Sprintf("vpc is in fabric %s but the relay vpc is in fabric %s", vpcFabric, relayFabric))
				}
				vpcDomains, relayDomains := vpc.Spec.Topology.Domains, relayVPC.Spec.Topology.Domains
				if !slices.ContainsFunc(vpcDomains, func(domain string) bool { return slices.Contains(relayDomains, domain) }) {
					return nil, field.Invalid(relayVPCPath, relayVPC.Name, fmt.Sprintf("vpc is in domains %v and the relay vpc in domains %v, they must share one", vpcDomains, relayDomains))
				}
				if relayNs := relayVPC.Spec.IPv4Namespace; relayNs != vpc.Spec.IPv4Namespace {
					return nil, field.Invalid(relayVPCPath, relayVPC.Name, fmt.Sprintf("vpc is in IPv4 namespace %s but the relay vpc is in %s", vpc.Spec.IPv4Namespace, relayNs))
				}
				if relayNs := relayVPC.Spec.VLANNamespace; relayNs != vpc.Spec.VLANNamespace {
					return nil, field.Invalid(relayVPCPath, relayVPC.Name, fmt.Sprintf("vpc is in VLAN namespace %s but the relay vpc is in %s", vpc.Spec.VLANNamespace, relayNs))
				}
			}
		}

		vpcs := &VPCList{}
		err = kube.List(ctx, vpcs, kclient.MatchingLabels{
			LabelIPv4NS: vpc.Spec.IPv4Namespace,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list VPCs: %w", err) // TODO replace with some internal error to not expose to the user
		}

		for _, other := range vpcs.Items {
			if other.Name == vpc.Name {
				continue
			}
			if other.Spec.IPv4Namespace != vpc.Spec.IPv4Namespace {
				continue
			}

			for _, otherSubnet := range other.Spec.Subnets {
				otherNet, err := netip.ParsePrefix(otherSubnet.Subnet)
				if err != nil {
					return nil, fmt.Errorf("failed to parse subnet %s: %w", otherSubnet.Subnet, err)
				}

				for _, subnet := range subnets {
					if subnet.Overlaps(otherNet) {
						return nil, field.Invalid(spec.Child("subnets"), subnet.String(), fmt.Sprintf("overlaps with subnet %s of VPC %s", otherSubnet.Subnet, other.Name))
					}
					for otherHBGPPrefixStr := range otherSubnet.HostBGPExtraPrefixes {
						op, err := netip.ParsePrefix(otherHBGPPrefixStr)
						if err != nil {
							return nil, fmt.Errorf("failed to parse hostBGP extra prefix %s: %w", otherHBGPPrefixStr, err)
						}
						if subnet.Overlaps(op) {
							return nil, field.Invalid(spec.Child("subnets"), subnet.String(), fmt.Sprintf("overlaps with hostBGP extra prefix %s of VPC %s", otherHBGPPrefixStr, other.Name))
						}
					}
				}
			}
		}

		vpcs = &VPCList{}
		err = kube.List(ctx, vpcs, kclient.MatchingLabels{
			LabelVLANNS: vpc.Spec.VLANNamespace,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list VPCs: %w", err) // TODO replace with some internal error to not expose to the user
		}

		for _, other := range vpcs.Items {
			if other.Name == vpc.Name {
				continue
			}
			if other.Spec.VLANNamespace != vpc.Spec.VLANNamespace {
				continue
			}

			for _, otherSubnet := range other.Spec.Subnets {
				for subnetName, subnet := range vpc.Spec.Subnets {
					if subnet.VLAN == otherSubnet.VLAN {
						// HostBGP subnets have VLAN 0 and we can have multiple of those
						if subnet.VLAN == 0 && (subnet.HostBGP || otherSubnet.HostBGP) {
							continue
						}

						return nil, field.Invalid(spec.Child("subnets").Key(subnetName).Child("vlan"), subnet.VLAN, "already used by VPC "+other.Name)
					}
				}
			}
		}
	}

	return nil, nil
}

func isInIPv4Namespace(vpcSubnet netip.Prefix, ipNs *IPv4Namespace) (bool, error) {
	for _, ipNsSubnetCfg := range ipNs.Spec.Subnets {
		ipNsSubnet, err := netip.ParsePrefix(ipNsSubnetCfg)
		if err != nil {
			return false, fmt.Errorf("failed to parse IPv4Namespace %s subnet %s: %w", ipNs.Name, ipNsSubnetCfg, err)
		}

		if iputil.IsSubset(vpcSubnet, ipNsSubnet) {
			return true, nil
		}
	}

	return false, nil
}

// validateHostBGPPrefixLens checks that the range of prefix lengths accepted from the host is
// consistent and that it falls within the prefix the host is expected to advertise from
func validateHostBGPPrefixLens(prefix netip.Prefix, minLen, maxLen uint8) error {
	if minLen > maxLen {
		return fmt.Errorf("hostBGP min prefix length %d is greater than max prefix length %d", minLen, maxLen) //nolint:err113
	}
	if maxLen > DefaultHostBGPPrefixLen {
		return fmt.Errorf("hostBGP max prefix length %d is greater than %d", maxLen, DefaultHostBGPPrefixLen) //nolint:err113
	}
	if int(minLen) < prefix.Bits() {
		return fmt.Errorf("hostBGP min prefix length %d is shorter than the prefix length of %s", minLen, prefix) //nolint:err113
	}

	return nil
}
