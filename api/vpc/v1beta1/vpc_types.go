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
	"net"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TODO specify gateway explicitly?
// TODO rename VPCSubnet.Subnet to CIDR? or CIDRBlock like in AWS?

// VPCSpec defines the desired state of VPC.
// At least one subnet is required.
type VPCSpec struct {
	// Mode is the VPC mode that defines how the VPCs are configured on the switches
	Mode VPCMode `json:"mode,omitempty"`
	// Subnets is the list of VPC subnets to configure
	Subnets map[string]*VPCSubnet `json:"subnets,omitempty"`
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
}

// VPCDHCP defines the on-demand DHCP configuration for the subnet
type VPCDHCP struct {
	// Relay is the DHCP relay IP address, if specified, DHCP server will be disabled
	Relay string `json:"relay,omitempty"`
	// Enable enables DHCP server for the subnet
	Enable bool `json:"enable,omitempty"`
	// Range (optional) is the DHCP range for the subnet if DHCP server is enabled
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

//+kubebuilder:object:root=true

// VPCList contains a list of VPC
type VPCList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []VPC `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPC{}, &VPCList{})
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

	for _, subnet := range vpc.Spec.Subnets {
		cidr, err := iputil.ParseCIDR(subnet.Subnet)
		if err != nil {
			continue
		}

		if subnet.Gateway == "" {
			subnet.Gateway = cidr.Gateway.String()
		}

		if prefixLen, _ := cidr.Subnet.Mask.Size(); prefixLen > 30 {
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
				subnet.DHCP.Options.InterfaceMTU = 9036 // TODO Magic number should be named constant somewhere.
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
				subnet.DHCP.Options.LeaseTimeSeconds = 3600 // TODO Magic number should be named constant somewhere.
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
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	if len(vpc.Name) > 11 {
		return nil, errors.Errorf("name %s is too long, must be <= 11 characters", vpc.Name)
	}
	if strings.HasPrefix(vpc.Name, VPCInfoExtPrefix) {
		return nil, errors.Errorf("vpc name cannot start with '%s': %s", VPCInfoExtPrefix, vpc.Name)
	}
	if vpc.Spec.IPv4Namespace == "" {
		return nil, errors.Errorf("ipv4Namespace is required")
	}
	if vpc.Spec.VLANNamespace == "" {
		return nil, errors.Errorf("vlanNamespace is required")
	}
	if len(vpc.Spec.Subnets) == 0 {
		return nil, errors.Errorf("at least one subnet is required")
	}
	if len(vpc.Spec.Subnets) > 20 {
		return nil, errors.Errorf("too many subnets, max is 20")
	}
	if !slices.Contains(VPCModes, vpc.Spec.Mode) {
		return nil, errors.Errorf("invalid mode %s, must be one of %v", vpc.Spec.Mode, VPCModes)
	}
	// TODO remove L3Flat mode or make experimental flag for it
	if vpc.Spec.Mode == VPCModeL3Flat {
		return nil, errors.Errorf("L3Flat mode is not supported yet")
	}

	subnets := []*net.IPNet{}
	vlans := map[uint16]bool{}
	for subnetName, subnetCfg := range vpc.Spec.Subnets {
		if subnetCfg.Subnet == "" {
			return nil, errors.Errorf("subnet %s: missing subnet", subnetName)
		}

		_, ipNet, err := net.ParseCIDR(subnetCfg.Subnet)
		if err != nil {
			return nil, errors.Wrapf(err, "subnet %s: failed to parse subnet %s", subnetName, subnetCfg.Subnet)
		}

		if prefixLen, _ := ipNet.Mask.Size(); prefixLen > 30 {
			return nil, errors.Errorf("subnet %s: prefix length %d is too large, must be <= 30", subnetName, prefixLen)
		}

		if fabricCfg != nil {
			for _, reserved := range fabricCfg.ParsedReservedSubnets() {
				if reserved.Contains(ipNet.IP) {
					return nil, errors.Errorf("subnet %s: subnet %s is reserved", subnetName, subnetCfg.Subnet)
				}
			}
		}

		if subnetCfg.Gateway == "" {
			return nil, errors.Errorf("subnet %s: gateway is required", subnetName)
		}

		gateway := net.ParseIP(subnetCfg.Gateway)
		if !ipNet.Contains(gateway) {
			return nil, errors.Errorf("subnet %s: gateway %s is not in the subnet", subnetName, subnetCfg.Gateway)
		}

		if subnetCfg.VLAN == 0 {
			return nil, errors.Errorf("subnet %s: vlan is required", subnetName)
		}
		vlans[subnetCfg.VLAN] = true

		subnets = append(subnets, ipNet)

		if subnetCfg.DHCP.Relay != "" && subnetCfg.DHCP.Enable {
			return nil, errors.Errorf("subnet %s: dhcp relay and dhcp server cannot be enabled at the same time", subnetName)
		}

		if subnetCfg.DHCP.Relay != "" {
			_, _, err := net.ParseCIDR(subnetCfg.DHCP.Relay)
			if err != nil {
				return nil, errors.Wrapf(err, "subnet %s: failed to parse dhcp relay %s", subnetName, subnetCfg.DHCP.Relay)
			}
		}

		if subnetCfg.DHCP.Options != nil && !subnetCfg.DHCP.Enable {
			if subnetCfg.DHCP.Options.PXEURL != "" {
				return nil, errors.Errorf("subnet %s: pxeURL is set but dhcp is disabled", subnetName)
			}

			if len(subnetCfg.DHCP.Options.DNSServers) > 0 {
				return nil, errors.Errorf("subnet %s: DNSServer is set but dhcp is disabled", subnetName)
			}

			if len(subnetCfg.DHCP.Options.TimeServers) > 0 {
				return nil, errors.Errorf("subnet %s: TimeServer is set but dhcp is disabled", subnetName)
			}

			if subnetCfg.DHCP.Options.InterfaceMTU > 0 {
				return nil, errors.Errorf("subnet %s: InterfaceMTU is set but dhcp is disabled", subnetName)
			}

			if subnetCfg.DHCP.Options.LeaseTimeSeconds > 0 {
				return nil, errors.Errorf("subnet %s: LeaseTimeSeconds is set but dhcp is disabled", subnetName)
			}

			if subnetCfg.DHCP.Options.DisableDefaultRoute {
				return nil, errors.Errorf("subnet %s: DisableDefaultRoute is set but dhcp is disabled", subnetName)
			}

			if len(subnetCfg.DHCP.Options.AdvertisedRoutes) > 0 {
				return nil, errors.Errorf("subnet %s: AdvertisedRoutes is set but dhcp is disabled", subnetName)
			}
		}

		if subnetCfg.DHCP.Enable {
			if subnetCfg.DHCP.Range == nil {
				return nil, errors.Errorf("subnet %s: dhcp range is required", subnetName)
			}
			if subnetCfg.DHCP.Range.Start == "" {
				return nil, errors.Errorf("subnet %s: dhcp range start is required", subnetName)
			}

			ip := net.ParseIP(subnetCfg.DHCP.Range.Start)
			if ip == nil {
				return nil, errors.Errorf("subnet %s: invalid dhcp range start %s", subnetName, subnetCfg.DHCP.Range.Start)
			}
			if ip.Equal(ipNet.IP) {
				return nil, errors.Errorf("subnet %s: dhcp range start %s is equal to subnet", subnetName, subnetCfg.DHCP.Range.Start)
			}
			if ip.Equal(gateway) {
				return nil, errors.Errorf("subnet %s: dhcp range start %s is equal to gateway", subnetName, subnetCfg.DHCP.Range.Start)
			}
			if !ipNet.Contains(ip) {
				return nil, errors.Errorf("subnet %s: dhcp range start %s is not in the subnet", subnetName, subnetCfg.DHCP.Range.Start)
			}

			if subnetCfg.DHCP.Range.End == "" {
				return nil, errors.Errorf("subnet %s: dhcp range end is required", subnetName)
			}

			ip = net.ParseIP(subnetCfg.DHCP.Range.End)
			if ip == nil {
				return nil, errors.Errorf("subnet %s: invalid dhcp range end %s", subnetName, subnetCfg.DHCP.Range.End)
			}
			if ip.Equal(ipNet.IP) {
				return nil, errors.Errorf("subnet %s: dhcp range end %s is equal to subnet", subnetName, subnetCfg.DHCP.Range.End)
			}
			if ip.Equal(gateway) {
				return nil, errors.Errorf("subnet %s: dhcp range end %s is equal to gateway", subnetName, subnetCfg.DHCP.Range.End)
			}
			if !ipNet.Contains(ip) {
				return nil, errors.Errorf("subnet %s: dhcp range end %s is not in the subnet", subnetName, subnetCfg.DHCP.Range.End)
			}

			// TODO check start < end

			if subnetCfg.DHCP.Options != nil {
				for _, dnsServer := range subnetCfg.DHCP.Options.DNSServers {
					if ip := net.ParseIP(dnsServer); ip == nil {
						return nil, errors.Errorf("subnet %s: dns address %s is not a valid IP", subnetName, dnsServer)
					}
				}

				for _, timeServer := range subnetCfg.DHCP.Options.TimeServers {
					if ip := net.ParseIP(timeServer); ip == nil {
						return nil, errors.Errorf("subnet %s: time server %s address is not a valid IP", subnetName, timeServer)
					}
				}

				if subnetCfg.DHCP.Options.InterfaceMTU < 96 {
					return nil, errors.Errorf("subnet %s: MTU cannot be set smaller than 96", subnetName)
				}
				if subnetCfg.DHCP.Options.InterfaceMTU > 9036 {
					return nil, errors.Errorf("subnet %s: MTU cannot be set greater than 9036", subnetName)
				}
				if subnetCfg.DHCP.Options.LeaseTimeSeconds < 1 {
					return nil, errors.Errorf("subnet %s: LeaseTimeSeconds cannot be set smaller than 1", subnetName)
				}
				// TODO: max lease time?

				for _, advertisedRoute := range subnetCfg.DHCP.Options.AdvertisedRoutes {
					if advertisedRoute.Destination == "" {
						return nil, errors.Errorf("subnet %s: advertised route destination is required", subnetName)
					}
					if advertisedRoute.Gateway == "" {
						return nil, errors.Errorf("subnet %s: advertised route gateway is required", subnetName)
					}

					_, _, err := net.ParseCIDR(advertisedRoute.Destination)
					if err != nil {
						return nil, errors.Wrapf(err, "subnet %s: failed to parse advertised route destination %s", subnetName, advertisedRoute.Destination)
					}

					gwIP := net.ParseIP(advertisedRoute.Gateway)
					if gwIP == nil {
						return nil, errors.Errorf("subnet %s: invalid advertised route gateway %s", subnetName, advertisedRoute.Gateway)
					}
					if !ipNet.Contains(gwIP) {
						return nil, errors.Errorf("subnet %s: advertised route gateway %s is not in the subnet", subnetName, advertisedRoute.Gateway)
					}
				}
			}

			staticIPs := map[string]string{}
			for mac, static := range subnetCfg.DHCP.Static {
				if static.IP == "" {
					return nil, errors.Errorf("subnet %s: empty static IP assigned to MAC %s", subnetName, mac)
				}
				if mac == "" {
					return nil, errors.Errorf("subnet %s: static IP %s assigned to empty MAC address", subnetName, static.IP)
				}
				if _, err := net.ParseMAC(mac); err != nil {
					return nil, errors.Errorf("subnet %s: invalid MAC %s for static IP %s", subnetName, mac, static.IP)
				}
				ip := net.ParseIP(static.IP)
				if ip == nil {
					return nil, errors.Errorf("subnet %s: invalid static IP %s", subnetName, static.IP)
				}
				if !ipNet.Contains(ip) {
					return nil, errors.Errorf("subnet %s: static IP %s is not in the subnet", subnetName, static.IP)
				}
				if static.IP == ipNet.IP.String() {
					return nil, errors.Errorf("subnet %s: static IP %s is the same as the subnet IP", subnetName, static.IP)
				}
				if static.IP == subnetCfg.Gateway {
					return nil, errors.Errorf("subnet %s: static IP %s is the same as the gateway IP", subnetName, static.IP)
				}
				if static.IP == iputil.LastIP(ipNet).IP.String() {
					return nil, errors.Errorf("subnet %s: static IP %s is the same as the broadcast IP", subnetName, static.IP)
				}

				if _, ok := staticIPs[static.IP]; ok {
					return nil, errors.Errorf("subnet %s: static IP %s is already assigned to another MAC address", subnetName, static.IP)
				}
				staticIPs[static.IP] = mac
			}
		}

		if !subnetCfg.DHCP.Enable {
			if subnetCfg.DHCP.Range != nil && (subnetCfg.DHCP.Range.Start != "" || subnetCfg.DHCP.Range.End != "") {
				return nil, errors.Errorf("subnet %s: dhcp range start or end is set but dhcp is disabled", subnetName)
			}

			if len(subnetCfg.DHCP.Static) > 0 {
				return nil, errors.Errorf("subnet %s: static IPs are not supported when DHCP is disabled", subnetName)
			}
		}
	}

	if len(vlans) != len(vpc.Spec.Subnets) {
		return nil, errors.Errorf("duplicate subnet VLANs")
	}

	if err := iputil.VerifyNoOverlap(subnets); err != nil {
		return nil, errors.Wrapf(err, "failed to verify no overlap subnets")
	}

	for permitIdx, permit := range vpc.Spec.Permit {
		if len(permit) < 2 {
			return nil, errors.Errorf("each permit policy must have at least 2 subnets in it")
		}

		subnets := map[string]bool{}
		for _, subnetName := range permit {
			if _, ok := vpc.Spec.Subnets[subnetName]; !ok {
				return nil, errors.Errorf("permit policy #%d: subnet %s not found", permitIdx, subnetName)
			}

			subnets[subnetName] = true
		}

		if len(subnets) != len(permit) {
			return nil, errors.Errorf("permit policy #%d: duplicate subnets", permitIdx)
		}
	}

	for idx, staticRoute := range vpc.Spec.StaticRoutes {
		if staticRoute.Prefix == "" {
			return nil, errors.Errorf("static route #%d: prefix is required", idx)
		}

		ip, ipNet, err := net.ParseCIDR(staticRoute.Prefix)
		if err != nil {
			return nil, errors.Wrapf(err, "static route #%d: failed to parse prefix %s", idx, staticRoute.Prefix)
		}

		if !ipNet.IP.Equal(ip) {
			return nil, errors.Errorf("static route #%d: prefix %s is invalid: inconsistent IP address and mask", idx, staticRoute.Prefix)
		}

		if len(staticRoute.NextHops) == 0 {
			return nil, errors.Errorf("static route #%d: at least one next hop is required", idx)
		}
	}

	if kube != nil {
		// TODO Can we rely on Validation webhook for cross VPC subnet? if not - main VPC subnet validation should happen in the VPC controller

		ipNs := &IPv4Namespace{}
		err := kube.Get(ctx, ktypes.NamespacedName{Name: vpc.Spec.IPv4Namespace, Namespace: vpc.Namespace}, ipNs)
		if err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, errors.Errorf("IPv4Namespace %s not found", vpc.Spec.IPv4Namespace)
			}

			return nil, errors.Wrapf(err, "failed to get IPv4Namespace %s", vpc.Spec.IPv4Namespace) // TODO replace with some internal error to not expose to the user
		}

		vlanNs := &wiringapi.VLANNamespace{}
		err = kube.Get(ctx, ktypes.NamespacedName{Name: vpc.Spec.VLANNamespace, Namespace: vpc.Namespace}, vlanNs)
		if err != nil {
			if kapierrors.IsNotFound(err) {
				return nil, errors.Errorf("VLANNamespace %s not found", vpc.Spec.VLANNamespace)
			}

			return nil, errors.Wrapf(err, "failed to get VLANNamespace %s", vpc.Spec.VLANNamespace) // TODO replace with some internal error to not expose to the user
		}

		for subnetName, subnetCfg := range vpc.Spec.Subnets {
			_, vpcSubnet, err := net.ParseCIDR(subnetCfg.Subnet)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse vpc subnet %s", subnetCfg.Subnet)
			}

			ok := false
			for _, ipNsSubnetCfg := range ipNs.Spec.Subnets {
				_, ipNsSubnet, err := net.ParseCIDR(ipNsSubnetCfg)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse IPv4Namespace %s subnet %s", vpc.Spec.IPv4Namespace, ipNsSubnetCfg)
				}

				if ipNsSubnet.Contains(vpcSubnet.IP) {
					ok = true

					break
				}
			}

			if !ok {
				return nil, errors.Errorf("vpc subnet %s (%s) doesn't belong to the IPv4Namespace %s", subnetName, subnetCfg.Subnet, vpc.Spec.IPv4Namespace)
			}

			if !vlanNs.Spec.Contains(subnetCfg.VLAN) {
				return nil, errors.Errorf("vpc subnet %s (%s) vlan %d doesn't belong to the VLANNamespace %s", subnetName, subnetCfg.Subnet, subnetCfg.VLAN, vpc.Spec.VLANNamespace)
			}
		}

		vpcs := &VPCList{}
		err = kube.List(ctx, vpcs, kclient.MatchingLabels{
			LabelIPv4NS: vpc.Spec.IPv4Namespace,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list VPCs") // TODO replace with some internal error to not expose to the user
		}

		for _, other := range vpcs.Items {
			if other.Name == vpc.Name {
				continue
			}
			if other.Spec.IPv4Namespace != vpc.Spec.IPv4Namespace {
				continue
			}

			for _, otherSubnet := range other.Spec.Subnets {
				_, otherNet, err := net.ParseCIDR(otherSubnet.Subnet)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse subnet %s", otherSubnet.Subnet)
				}

				for _, subnet := range subnets {
					if subnet.Contains(otherNet.IP) {
						return nil, errors.Errorf("subnet %s overlaps with subnet %s of VPC %s", subnet.String(), otherSubnet.Subnet, other.Name)
					}
				}
			}
		}

		vpcs = &VPCList{}
		err = kube.List(ctx, vpcs, kclient.MatchingLabels{
			LabelVLANNS: vpc.Spec.VLANNamespace,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list VPCs") // TODO replace with some internal error to not expose to the user
		}

		for _, other := range vpcs.Items {
			if other.Name == vpc.Name {
				continue
			}
			if other.Spec.VLANNamespace != vpc.Spec.VLANNamespace {
				continue
			}

			for _, otherSubnet := range other.Spec.Subnets {
				for _, subnet := range vpc.Spec.Subnets {
					if subnet.VLAN == otherSubnet.VLAN {
						return nil, errors.Errorf("vlan %d is already used by other VPC", subnet.VLAN)
					}
				}
			}
		}
	}

	return nil, nil
}
