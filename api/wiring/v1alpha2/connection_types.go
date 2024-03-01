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
	"net"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	"golang.org/x/exp/maps"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	CONNECTION_TYPE_UNBUNDLED    = "unbundled"
	CONNECTION_TYPE_BUNDLED      = "bundled"
	CONNECTION_TYPE_MANAGEMENT   = "management" // TODO rename to control?
	CONNECTION_TYPE_MCLAG        = "mclag"
	CONNECTION_TYPE_MCLAGDOMAIN  = "mclag-domain"
	CONNECTION_TYPE_ESLAG        = "eslag"
	CONNECTION_TYPE_FABRIC       = "fabric"
	CONNECTION_TYPE_VPC_LOOPBACK = "vpc-loopback"
	CONNECTION_EXTERNAL          = "external"
	CONNECTION_STATIC_EXTERNAL   = "static-external"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BasePortName defines the full name of the switch port
type BasePortName struct {
	// Port defines the full name of the switch port in the format of "device/port", such as "spine-1/Ethernet1".
	// SONiC port name is used as a port name and switch name should be same as the name of the Switch object.
	Port string `json:"port,omitempty"`
}

// ServerToSwitchLink defines the server-to-switch link
type ServerToSwitchLink struct {
	// Server is the server side of the connection
	Server BasePortName `json:"server,omitempty"`
	// Switch is the switch side of the connection
	Switch BasePortName `json:"switch,omitempty"`
}

// ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled, mclag, etc.) configuration
type ServerFacingConnectionConfig struct {
	// MTU is the MTU to be configured on the switch port or port channel
	MTU uint16 `json:"mtu,omitempty"`
}

// ConnUnbundled defines the unbundled connection (no port channel, single server to a single switch with a single link)
type ConnUnbundled struct {
	// Link is the server-to-switch link
	Link ServerToSwitchLink `json:"link,omitempty"`
	// ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled, mclag, etc.) configuration
	ServerFacingConnectionConfig `json:",inline"`
}

// ConnBundled defines the bundled connection (port channel, single server to a single switch with multiple links)
type ConnBundled struct {
	// Links is the list of server-to-switch links
	Links []ServerToSwitchLink `json:"links,omitempty"`
	// ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled, mclag, etc.) configuration
	ServerFacingConnectionConfig `json:",inline"`
}

// ConnMgmtLinkServer defines the server side of the management link
type ConnMgmtLinkServer struct {
	// BasePortName defines the full name of the switch port
	BasePortName `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	// IP is the IP address of the server side of the management link (control node port configuration)
	IP string `json:"ip,omitempty"`
	// MAC is an optional MAC address of the control node port for the management link, if specified will be used to
	// create a "virtual" link with the connection names on the control node
	MAC string `json:"mac,omitempty"`
}

// ConnMgmtLinkSwitch defines the switch side of the management link
type ConnMgmtLinkSwitch struct {
	// BasePortName defines the full name of the switch port
	BasePortName `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	// IP is the IP address of the switch side of the management link (switch port configuration)
	IP string `json:"ip,omitempty"`
	// ONIEPortName is an optional ONIE port name of the switch side of the management link that's only used by the IPv6 Link Local discovery
	ONIEPortName string `json:"oniePortName,omitempty"`
}

// ConnMgmtLink defines the management connection link
type ConnMgmtLink struct {
	// Server is the server side of the management link
	Server ConnMgmtLinkServer `json:"server,omitempty"`
	// Switch is the switch side of the management link
	Switch ConnMgmtLinkSwitch `json:"switch,omitempty"`
}

// ConnMgmt defines the management connection (single control node/server to a single switch with a single link)
type ConnMgmt struct {
	Link ConnMgmtLink `json:"link,omitempty"`
}

// ConnMCLAG defines the MCLAG connection (port channel, single server to pair of switches with multiple links)
type ConnMCLAG struct {
	//+kubebuilder:validation:MinItems=2
	// Links is the list of server-to-switch links
	Links []ServerToSwitchLink `json:"links,omitempty"`
	// ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled, mclag, etc.) configuration
	ServerFacingConnectionConfig `json:",inline"`
	// Fallback is the optional flag that used to indicate one of the links in LACP port channel to be used as a fallback link
	Fallback bool `json:"fallback,omitempty"`
}

// ConnESLAG defines the ESLAG connection (port channel, single server to 2-4 switches with multiple links)
type ConnESLAG struct {
	//+kubebuilder:validation:MinItems=2
	// Links is the list of server-to-switch links
	Links []ServerToSwitchLink `json:"links,omitempty"`
	// ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled, eslag, etc.) configuration
	ServerFacingConnectionConfig `json:",inline"`
	// Fallback is the optional flag that used to indicate one of the links in LACP port channel to be used as a fallback link
	Fallback bool `json:"fallback,omitempty"`
}

// SwitchToSwitchLink defines the switch-to-switch link
type SwitchToSwitchLink struct {
	// Switch1 is the first switch side of the connection
	Switch1 BasePortName `json:"switch1,omitempty"`
	// Switch2 is the second switch side of the connection
	Switch2 BasePortName `json:"switch2,omitempty"`
}

// ConnMCLAGDomain defines the MCLAG domain connection which makes two switches into a single logical switch or
// redundancy group and allows to use MCLAG connections to connect servers in a multi-homed way.
type ConnMCLAGDomain struct {
	//+kubebuilder:validation:MinItems=1
	// PeerLinks is the list of peer links between the switches, used to pass server traffic between switch
	PeerLinks []SwitchToSwitchLink `json:"peerLinks,omitempty"`

	//+kubebuilder:validation:MinItems=1
	// SessionLinks is the list of session links between the switches, used only to pass MCLAG control plane and BGP
	// traffic between switches
	SessionLinks []SwitchToSwitchLink `json:"sessionLinks,omitempty"`
}

// ConnFabricLinkSwitch defines the switch side of the fabric link
type ConnFabricLinkSwitch struct {
	// BasePortName defines the full name of the switch port
	BasePortName `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	// IP is the IP address of the switch side of the fabric link (switch port configuration)
	IP string `json:"ip,omitempty"`
}

// FabricLink defines the fabric connection link
type FabricLink struct {
	// Spine is the spine side of the fabric link
	Spine ConnFabricLinkSwitch `json:"spine,omitempty"`
	// Leaf is the leaf side of the fabric link
	Leaf ConnFabricLinkSwitch `json:"leaf,omitempty"`
}

// ConnFabric defines the fabric connection (single spine to a single leaf with at least one link)
type ConnFabric struct {
	//+kubebuilder:validation:MinItems=1
	// Links is the list of spine-to-leaf links
	Links []FabricLink `json:"links,omitempty"`
}

// ConnVPCLoopback defines the VPC loopback connection (multiple port pairs on a single switch) that enables automated
// workaround named "VPC Loopback" that allow to avoid switch hardware limitations and traffic going through CPU in some
// cases
type ConnVPCLoopback struct {
	//+kubebuilder:validation:MinItems=1
	// Links is the list of VPC loopback links
	Links []SwitchToSwitchLink `json:"links,omitempty"`
}

// ConnExternalLink defines the external connection link
type ConnExternalLink struct {
	Switch BasePortName `json:"switch,omitempty"`
}

// ConnExternal defines the external connection (single switch to a single external device with a single link)
type ConnExternal struct {
	// Link is the external connection link
	Link ConnExternalLink `json:"link,omitempty"`
}

// ConnStaticExternalLinkSwitch defines the switch side of the static external connection link
type ConnStaticExternalLinkSwitch struct {
	// BasePortName defines the full name of the switch port
	BasePortName `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	// IP is the IP address of the switch side of the static external connection link (switch port configuration)
	IP string `json:"ip,omitempty"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}$`
	// NextHop is the next hop IP address for static routes that will be created for the subnets
	NextHop string `json:"nextHop,omitempty"`
	// Subnets is the list of subnets that will get static routes using the specified next hop
	Subnets []string `json:"subnets,omitempty"`
	// VLAN is the optional VLAN ID to be configured on the switch port
	VLAN uint16 `json:"vlan,omitempty"`
}

// ConnStaticExternalLink defines the static external connection link
type ConnStaticExternalLink struct {
	// Switch is the switch side of the static external connection link
	Switch ConnStaticExternalLinkSwitch `json:"switch,omitempty"`
}

// ConnStaticExternal defines the static external connection (single switch to a single external device with a single link)
type ConnStaticExternal struct {
	// Link is the static external connection link
	Link ConnStaticExternalLink `json:"link,omitempty"`
	// WithinVPC is the optional VPC name to provision the static external connection within the VPC VRF instead of default one to make resource available to the specific VPC
	WithinVPC string `json:"withinVPC,omitempty"`
}

// ConnectionSpec defines the desired state of Connection
type ConnectionSpec struct {
	// Unbundled defines the unbundled connection (no port channel, single server to a single switch with a single link)
	Unbundled *ConnUnbundled `json:"unbundled,omitempty"`
	// Bundled defines the bundled connection (port channel, single server to a single switch with multiple links)
	Bundled *ConnBundled `json:"bundled,omitempty"`
	// Management defines the management connection (single control node/server to a single switch with a single link)
	Management *ConnMgmt `json:"management,omitempty"`
	// MCLAG defines the MCLAG connection (port channel, single server to pair of switches with multiple links)
	MCLAG *ConnMCLAG `json:"mclag,omitempty"`
	// ESLAG defines the ESLAG connection (port channel, single server to 2-4 switches with multiple links)
	ESLAG *ConnESLAG `json:"eslag,omitempty"`
	// MCLAGDomain defines the MCLAG domain connection which makes two switches into a single logical switch for server multi-homing
	MCLAGDomain *ConnMCLAGDomain `json:"mclagDomain,omitempty"`
	// Fabric defines the fabric connection (single spine to a single leaf with at least one link)
	Fabric *ConnFabric `json:"fabric,omitempty"`
	// VPCLoopback defines the VPC loopback connection (multiple port pairs on a single switch) for automated workaround
	VPCLoopback *ConnVPCLoopback `json:"vpcLoopback,omitempty"`
	// External defines the external connection (single switch to a single external device with a single link)
	External *ConnExternal `json:"external,omitempty"`
	// StaticExternal defines the static external connection (single switch to a single external device with a single link)
	StaticExternal *ConnStaticExternal `json:"staticExternal,omitempty"`
}

// ConnectionStatus defines the observed state of Connection
type ConnectionStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=conn
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/connection-type`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// Connection object represents a logical and physical connections between any devices in the Fabric (Switch, Server
// and External objects). It's needed to define all physical and logical connections between the devices in the Wiring
// Diagram. Connection type is defined by the top-level field in the ConnectionSpec. Exactly one of them could be used
// in a single Connection object.
type Connection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the Connection
	Spec ConnectionSpec `json:"spec,omitempty"`
	// Status is the observed state of the Connection
	Status ConnectionStatus `json:"status,omitempty"`
}

const KindConnection = "Connection"

//+kubebuilder:object:root=true

// ConnectionList contains a list of Connection
type ConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Connection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Connection{}, &ConnectionList{})
}

var _ meta.Object = (*Connection)(nil)

func NewBasePortName(name string) BasePortName {
	return BasePortName{
		Port: name,
	}
}

// +kubebuilder:object:generate=false
type IPort interface {
	PortName() string
	LocalPortName() string
	DeviceName() string
}

var (
	_ IPort = &BasePortName{}
	_ IPort = &ConnMgmtLinkSwitch{}
)

func (pn *BasePortName) PortName() string {
	return pn.Port
}

func SplitPortName(name string) []string {
	return strings.SplitN(name, PORT_NAME_SEPARATOR, 2)
}

func (pn *BasePortName) LocalPortName() string {
	return SplitPortName(pn.Port)[1]
}

func (pn *BasePortName) DeviceName() string {
	return SplitPortName(pn.Port)[0]
}

func (c *ConnectionSpec) GenerateName() string {
	if c != nil {
		role := ""
		left := ""
		right := []string{}

		if c.Unbundled != nil {
			role = "unbundled"
			left = c.Unbundled.Link.Server.DeviceName()
			right = []string{c.Unbundled.Link.Switch.DeviceName()}
		} else if c.Bundled != nil {
			role = "bundled"
			left = c.Bundled.Links[0].Server.DeviceName()
			right = []string{c.Bundled.Links[0].Switch.DeviceName()}
			for _, link := range c.Bundled.Links {
				// check we have the same server in each link // TODO add validation
				if link.Server.DeviceName() != left {
					return "<invalid>" // TODO replace with error?
				}
				if link.Switch.DeviceName() != right[0] {
					return "<invalid>" // TODO replace with error?
				}
			}
		} else if c.Management != nil {
			role = "mgmt"
			left = c.Management.Link.Server.DeviceName()
			right = []string{c.Management.Link.Switch.DeviceName()}
		} else if c.MCLAGDomain != nil {
			role = "mclag-domain"
			left = c.MCLAGDomain.PeerLinks[0].Switch1.DeviceName() // TODO check session links?
			right = []string{c.MCLAGDomain.PeerLinks[0].Switch2.DeviceName()}
			for _, link := range c.MCLAGDomain.PeerLinks {
				// check that we have the same switches on both ends in each link // TODO add validation
				if link.Switch1.DeviceName() != left {
					return "<invalid>" // TODO replace with error?
				}
				if link.Switch2.DeviceName() != right[0] {
					return "<invalid>" // TODO replace with error?
				}
			}
		} else if c.MCLAG != nil {
			role = "mclag"
			left = c.MCLAG.Links[0].Server.DeviceName()
			for _, link := range c.MCLAG.Links {
				// check we have the same server in each link // TODO add validation
				if link.Server.DeviceName() != left {
					return "<invalid>" // TODO replace with error?
				}
				right = append(right, link.Switch.DeviceName())
			}
		} else if c.ESLAG != nil {
			role = "eslag"
			left = c.ESLAG.Links[0].Server.DeviceName()
			for _, link := range c.ESLAG.Links {
				// check we have the same server in each link // TODO add validation
				if link.Server.DeviceName() != left {
					return "<invalid>" // TODO replace with error?
				}
				right = append(right, link.Switch.DeviceName())
			}
		} else if c.Fabric != nil {
			role = "fabric"
			left = c.Fabric.Links[0].Spine.DeviceName()
			right = []string{c.Fabric.Links[0].Leaf.DeviceName()}
		} else if c.VPCLoopback != nil {
			role = "vpc-loopback"
			left = c.VPCLoopback.Links[0].Switch1.DeviceName()
		} else if c.External != nil {
			role = "external"
			left = c.External.Link.Switch.DeviceName()
		} else if c.StaticExternal != nil {
			role = "static-external"
			left = c.StaticExternal.Link.Switch.DeviceName()
		}

		if left != "" && role != "" {
			if len(right) > 0 {
				return fmt.Sprintf("%s--%s--%s", left, role, strings.Join(right, "--"))
			} else {
				return fmt.Sprintf("%s--%s", left, role)
			}
		}
	}

	return "<invalid>" // TODO replace with error?
}

func (c *ConnectionSpec) Type() string {
	if c.Unbundled != nil {
		return CONNECTION_TYPE_UNBUNDLED
	} else if c.Bundled != nil {
		return CONNECTION_TYPE_BUNDLED
	} else if c.Management != nil {
		return CONNECTION_TYPE_MANAGEMENT
	} else if c.MCLAGDomain != nil {
		return CONNECTION_TYPE_MCLAGDOMAIN
	} else if c.MCLAG != nil {
		return CONNECTION_TYPE_MCLAG
	} else if c.ESLAG != nil {
		return CONNECTION_TYPE_ESLAG
	} else if c.Fabric != nil {
		return CONNECTION_TYPE_FABRIC
	} else if c.VPCLoopback != nil {
		return CONNECTION_TYPE_VPC_LOOPBACK
	} else if c.External != nil {
		return CONNECTION_EXTERNAL
	} else if c.StaticExternal != nil {
		return CONNECTION_STATIC_EXTERNAL
	}

	return "<invalid>"
}

func (c *ConnectionSpec) ConnectionLabels() map[string]string {
	labels := map[string]string{}

	switches, servers, _, _, err := c.Endpoints()
	// if error, we don't need to set labels
	if err != nil {
		return labels
	}

	sort.Strings(switches)
	sort.Strings(servers)

	for _, switchName := range switches {
		labels[ListLabelSwitch(switchName)] = ListLabelValue
	}
	for _, serverName := range servers {
		labels[ListLabelServer(serverName)] = ListLabelValue
	}

	labels[LabelConnectionType] = c.Type()

	if c.StaticExternal != nil && c.StaticExternal.WithinVPC != "" {
		labels[LabelVPC] = c.StaticExternal.WithinVPC
	}

	return labels
}

func (s *ConnectionSpec) Endpoints() ([]string, []string, []string, map[string]string, error) {
	switches := map[string]struct{}{}
	servers := map[string]struct{}{}
	ports := map[string]struct{}{}
	links := map[string]string{}

	nonNills := 0
	if s.Unbundled != nil {
		nonNills++

		switches[s.Unbundled.Link.Switch.DeviceName()] = struct{}{}
		servers[s.Unbundled.Link.Server.DeviceName()] = struct{}{}
		ports[s.Unbundled.Link.Switch.PortName()] = struct{}{}
		ports[s.Unbundled.Link.Server.PortName()] = struct{}{}
		links[s.Unbundled.Link.Switch.PortName()] = s.Unbundled.Link.Server.PortName()

		if len(switches) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one switch must be used for unbundled connection")
		}
		if len(servers) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one server must be used for unbundled connection")
		}
		if len(ports) != 2 {
			return nil, nil, nil, nil, errors.Errorf("two unique ports must be used for unbundled connection")
		}
	} else if s.Bundled != nil {
		nonNills++

		for _, link := range s.Bundled.Links {
			switches[link.Switch.DeviceName()] = struct{}{}
			servers[link.Server.DeviceName()] = struct{}{}
			ports[link.Switch.PortName()] = struct{}{}
			ports[link.Server.PortName()] = struct{}{}
			links[link.Switch.PortName()] = link.Server.PortName()
		}

		if len(switches) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one switch must be used for bundled connection")
		}
		if len(servers) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one server must be used for bundled connection")
		}
		if len(ports) != 2*len(s.Bundled.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for bundled connection")
		}
	} else if s.Management != nil {
		nonNills++

		switches[s.Management.Link.Switch.DeviceName()] = struct{}{}
		servers[s.Management.Link.Server.DeviceName()] = struct{}{}
		ports[s.Management.Link.Switch.PortName()] = struct{}{}
		ports[s.Management.Link.Server.PortName()] = struct{}{}
		links[s.Management.Link.Switch.PortName()] = s.Management.Link.Server.PortName()

		if len(switches) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one switch must be used for management connection")
		}
		if len(servers) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one server must be used for management connection")
		}
		if len(ports) != 2 {
			return nil, nil, nil, nil, errors.Errorf("two unique ports must be used for management connection")
		}
	} else if s.MCLAGDomain != nil {
		nonNills++

		for _, link := range s.MCLAGDomain.PeerLinks {
			switches[link.Switch1.DeviceName()] = struct{}{}
			switches[link.Switch2.DeviceName()] = struct{}{}
			ports[link.Switch1.PortName()] = struct{}{}
			ports[link.Switch2.PortName()] = struct{}{}
			links[link.Switch1.PortName()] = link.Switch2.PortName()
		}
		for _, link := range s.MCLAGDomain.SessionLinks {
			switches[link.Switch1.DeviceName()] = struct{}{}
			switches[link.Switch2.DeviceName()] = struct{}{}
			ports[link.Switch1.PortName()] = struct{}{}
			ports[link.Switch2.PortName()] = struct{}{}
			links[link.Switch1.PortName()] = link.Switch2.PortName()
		}

		if len(s.MCLAGDomain.PeerLinks) < 1 {
			return nil, nil, nil, nil, errors.Errorf("at least one peer link must be used for mclag domain connection")
		}
		if len(s.MCLAGDomain.SessionLinks) < 1 {
			return nil, nil, nil, nil, errors.Errorf("at least one session link must be used for mclag domain connection")
		}
		if len(switches) != 2 {
			return nil, nil, nil, nil, errors.Errorf("two switches must be used for mclag domain connection")
		}
		if len(ports) != 2*(len(s.MCLAGDomain.PeerLinks)+len(s.MCLAGDomain.SessionLinks)) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for mclag domain connection")
		}
	} else if s.MCLAG != nil {
		nonNills++

		for _, link := range s.MCLAG.Links {
			switches[link.Switch.DeviceName()] = struct{}{}
			servers[link.Server.DeviceName()] = struct{}{}
			ports[link.Switch.PortName()] = struct{}{}
			ports[link.Server.PortName()] = struct{}{}
			links[link.Switch.PortName()] = link.Server.PortName()
		}

		if len(switches) != 2 {
			return nil, nil, nil, nil, errors.Errorf("two switches must be used for mclag connection")
		}
		if len(servers) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one server must be used for mclag connection")
		}
		if len(ports) != 2*len(s.MCLAG.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for mclag connection")
		}
	} else if s.ESLAG != nil {
		nonNills++

		for _, link := range s.ESLAG.Links {
			switches[link.Switch.DeviceName()] = struct{}{}
			servers[link.Server.DeviceName()] = struct{}{}
			ports[link.Switch.PortName()] = struct{}{}
			ports[link.Server.PortName()] = struct{}{}
			links[link.Switch.PortName()] = link.Server.PortName()
		}

		if len(switches) < 2 {
			return nil, nil, nil, nil, errors.Errorf("at least two switches must be used for eslag connection")
		}
		if len(switches) > 4 {
			return nil, nil, nil, nil, errors.Errorf("at most four switches must be used for eslag connection")
		}
		if len(servers) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one server must be used for eslag connection")
		}
		if len(ports) != 2*len(s.ESLAG.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for eslag connection")
		}
	} else if s.Fabric != nil {
		nonNills++

		for _, link := range s.Fabric.Links {
			switches[link.Spine.DeviceName()] = struct{}{}
			switches[link.Leaf.DeviceName()] = struct{}{}
			ports[link.Spine.PortName()] = struct{}{}
			ports[link.Leaf.PortName()] = struct{}{}
			links[link.Spine.PortName()] = link.Leaf.PortName()
		}

		if len(switches) != 2 {
			return nil, nil, nil, nil, errors.Errorf("two switches must be used for fabric connection")
		}
		if len(ports) != 2*len(s.Fabric.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for fabric connection")
		}
	} else if s.VPCLoopback != nil {
		nonNills++

		for _, link := range s.VPCLoopback.Links {
			switches[link.Switch1.DeviceName()] = struct{}{}
			switches[link.Switch2.DeviceName()] = struct{}{}
			ports[link.Switch1.PortName()] = struct{}{}
			ports[link.Switch2.PortName()] = struct{}{}
			links[link.Switch1.PortName()] = link.Switch2.PortName()
		}

		if len(switches) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one switches must be used for vpc-loopback connection")
		}
		if len(ports) != 2*len(s.VPCLoopback.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for fabric connection")
		}
	} else if s.External != nil {
		nonNills++

		switches[s.External.Link.Switch.DeviceName()] = struct{}{}
		ports[s.External.Link.Switch.PortName()] = struct{}{}
	} else if s.StaticExternal != nil {
		nonNills++

		switches[s.StaticExternal.Link.Switch.DeviceName()] = struct{}{}
		ports[s.StaticExternal.Link.Switch.PortName()] = struct{}{}
	}

	if nonNills != 1 {
		return nil, nil, nil, nil, errors.Errorf("exactly one connection type must be used")
	}

	for port := range ports {
		parts := SplitPortName(port)

		if len(parts) != 2 {
			return nil, nil, nil, nil, errors.Errorf("invalid port name %s", port)
		}

		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil, nil, nil, nil, errors.Errorf("invalid port name %s, should be \"<device>/<port>\" format", port)
		}
	}

	return maps.Keys(switches), maps.Keys(servers), maps.Keys(ports), links, nil
}

func (conn *Connection) Default() {
	meta.DefaultObjectMetadata(conn)

	if conn.Labels == nil {
		conn.Labels = map[string]string{}
	}

	CleanupFabricLabels(conn.Labels)

	maps.Copy(conn.Labels, conn.Spec.ConnectionLabels())
}

func (conn *ConnectionSpec) ValidateServerFacingMTU(fabricMTU uint16, serverFacingMTUOffset uint16) error {
	if conn.Unbundled != nil && conn.Unbundled.MTU > fabricMTU-serverFacingMTUOffset {
		return errors.Errorf("unbundled connection mtu %d is greater than fabric mtu %d - server facing mtu offset %d", conn.Unbundled.MTU, fabricMTU, serverFacingMTUOffset)
	}
	if conn.Bundled != nil && conn.Bundled.MTU > fabricMTU-serverFacingMTUOffset {
		return errors.Errorf("bundled connection mtu %d is greater than fabric mtu %d - server facing mtu offset %d", conn.Bundled.MTU, fabricMTU, serverFacingMTUOffset)
	}
	if conn.MCLAG != nil && conn.MCLAG.MTU > fabricMTU-serverFacingMTUOffset {
		return errors.Errorf("mclag connection mtu %d is greater than fabric mtu %d - server facing mtu offset %d", conn.MCLAG.MTU, fabricMTU, serverFacingMTUOffset)
	}

	return nil
}

func (conn *Connection) Validate(ctx context.Context, kube client.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(conn); err != nil {
		return nil, err
	}

	// TODO validate local port names against server/switch profiles
	// TODO validate used port names across all connections

	if fabricCfg != nil {
		if err := conn.Spec.ValidateServerFacingMTU(fabricCfg.FabricMTU, fabricCfg.ServerFacingMTUOffset); err != nil {
			return nil, err
		}
	}

	switches, servers, ports, _, err := conn.Spec.Endpoints()
	if err != nil {
		return nil, err
	}

	if conn.Spec.StaticExternal != nil {
		se := conn.Spec.StaticExternal.Link.Switch

		_, ipNet, err := net.ParseCIDR(se.IP)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse cidr %s", se.IP)
		}

		nextHop := net.ParseIP(se.NextHop)
		if nextHop == nil {
			return nil, errors.Errorf("failed to parse next hop %s", se.NextHop)
		}

		if !ipNet.Contains(nextHop) {
			return nil, errors.Errorf("next hop %s is not in cidr %s", nextHop, ipNet)
		}

		subnets := []*net.IPNet{}
		for _, subnet := range se.Subnets {
			_, ipNet, err := net.ParseCIDR(subnet)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse cidr %s", subnet)
			}

			subnets = append(subnets, ipNet)
		}

		if err := iputil.VerifyNoOverlap(subnets); err != nil {
			return nil, errors.Wrapf(err, "subnets overlap")
		}

		if fabricCfg != nil {
			subnets = append(subnets, fabricCfg.ParsedReservedSubnets()...)
		}

		if err := iputil.VerifyNoOverlap(subnets); err != nil {
			return nil, errors.Wrapf(err, "subnets overlap with reserved subnets")
		}
	}

	if conn.Spec.ESLAG != nil && fabricCfg != nil && fabricCfg.FabricMode != meta.FabricModeSpineLeaf {
		return nil, errors.Errorf("eslag connection is not allowed in current fabric configuration")
	}

	if kube != nil {
		rGroup := ""
		rType := meta.RedundancyTypeNone

		for _, switchName := range switches {
			sw := &Switch{}
			err := kube.Get(ctx, types.NamespacedName{Name: switchName, Namespace: conn.Namespace}, sw) // TODO namespace could be different?
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("switch %s not found", switchName)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get switch %s", switchName) // TODO replace with some internal error to not expose to the user
			}

			if conn.Spec.MCLAG != nil || conn.Spec.ESLAG != nil || conn.Spec.MCLAGDomain != nil {
				if sw.Spec.Redundancy.Group != "" {
					if rGroup != "" && rGroup != sw.Spec.Redundancy.Group {
						return nil, errors.Errorf("all switches in MCLAG/ESLAG/MCLAGDomain connection should belong to the same redundancy group, found %s in %s", switchName, rGroup)
					}
					rGroup = sw.Spec.Redundancy.Group
				}
				if sw.Spec.Redundancy.Type != "" {
					if rType != "" && rType != sw.Spec.Redundancy.Type {
						return nil, errors.Errorf("all switches in MCLAG/ESLAG/MCLAGDomain connection should belong to the same redundancy type, found %s in %s", switchName, rType)
					}
					rType = sw.Spec.Redundancy.Type
				}
			}
		}

		if conn.Spec.MCLAG != nil {
			if rGroup == "" {
				return nil, errors.Errorf("all switches in MCLAG connection should have redundancy group")
			}
			if rType != meta.RedundancyTypeMCLAG {
				return nil, errors.Errorf("all switches in MCLAG connection should have MCLAG redundancy type, found %s", rType)
			}
		}
		if conn.Spec.ESLAG != nil {
			if rGroup == "" {
				return nil, errors.Errorf("all switches in ESLAG connection should have redundancy group")
			}
			if rType != meta.RedundancyTypeESLAG {
				return nil, errors.Errorf("all switches in ESLAG connection should have ESLAG redundancy type, found %s", rType)
			}
		}
		if conn.Spec.MCLAGDomain != nil {
			if rGroup == "" {
				return nil, errors.Errorf("all switches in MCLAGDomain connection should have redundancy group")
			}
			if rType != meta.RedundancyTypeMCLAG {
				return nil, errors.Errorf("all switches in MCLAGDomain connection should have MCLAG redundancy type, found %s", rType)
			}
		}

		for _, serverName := range servers {
			err := kube.Get(ctx, types.NamespacedName{Name: serverName, Namespace: conn.Namespace}, &Server{}) // TODO namespace could be different?
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("server %s not found", serverName)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get server %s", serverName) // TODO replace with some internal error to not expose to the user
			}
		}

		connPorts := map[string]bool{}
		for _, port := range ports {
			connPorts[port] = true
		}

		conns := &ConnectionList{}
		if err := kube.List(ctx, conns, &client.ListOptions{Namespace: conn.Namespace}); err != nil { // TODO namespace could be different?
			return nil, errors.Wrapf(err, "failed to list connections")
		}

		for _, other := range conns.Items {
			if other.Name == conn.Name {
				continue
			}

			_, _, ports, _, err := other.Spec.Endpoints()
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get endpoints for connection %s", other.Name)
			}

			for _, port := range ports {
				if connPorts[port] {
					return nil, errors.Errorf("port %s is already used by other connection", port)
				}
			}
		}
	}

	return nil, nil
}
