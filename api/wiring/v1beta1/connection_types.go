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
	"strings"

	"github.com/fatih/color"
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
	ConnectionTypeUnbundled      = "unbundled"
	ConnectionTypeBundled        = "bundled"
	ConnectionTypeMCLAG          = "mclag"
	ConnectionTypeMCLAGDomain    = "mclag-domain"
	ConnectionTypeESLAG          = "eslag"
	ConnectionTypeFabric         = "fabric"
	ConnectionTypeVPCLoopback    = "vpc-loopback"
	ConnectionTypeExternal       = "external"
	ConnectionTypeStaticExternal = "static-external"
)

var ConnectionTypesServerFacing = []string{
	ConnectionTypeUnbundled,
	ConnectionTypeBundled,
	ConnectionTypeMCLAG,
	ConnectionTypeESLAG,
}

const INVALID = "<invalid>"

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

var (
	_ meta.Object     = (*Connection)(nil)
	_ meta.ObjectList = (*ConnectionList)(nil)
)

func (connList *ConnectionList) GetItems() []meta.Object {
	items := make([]meta.Object, len(connList.Items))
	for i := range connList.Items {
		items[i] = &connList.Items[i]
	}

	return items
}

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

var _ IPort = &BasePortName{}

func (pn *BasePortName) PortName() string {
	return pn.Port
}

func SplitPortName(name string) []string {
	return strings.SplitN(name, PortNameSeparator, 2)
}

func (pn *BasePortName) LocalPortName() string {
	return SplitPortName(pn.Port)[1]
}

func (pn *BasePortName) DeviceName() string {
	return SplitPortName(pn.Port)[0]
}

func (connSpec *ConnectionSpec) GenerateName() string {
	if connSpec != nil {
		role := ""
		left := ""
		right := []string{}

		if connSpec.Unbundled != nil {
			role = "unbundled"
			left = connSpec.Unbundled.Link.Server.DeviceName()
			right = []string{connSpec.Unbundled.Link.Switch.DeviceName()}
		} else if connSpec.Bundled != nil {
			role = "bundled"
			left = connSpec.Bundled.Links[0].Server.DeviceName()
			right = []string{connSpec.Bundled.Links[0].Switch.DeviceName()}
			for _, link := range connSpec.Bundled.Links {
				// check we have the same server in each link // TODO add validation
				if link.Server.DeviceName() != left {
					return INVALID // TODO replace with error?
				}
				if link.Switch.DeviceName() != right[0] {
					return INVALID // TODO replace with error?
				}
			}
		} else if connSpec.MCLAGDomain != nil {
			role = "mclag-domain"
			left = connSpec.MCLAGDomain.PeerLinks[0].Switch1.DeviceName() // TODO check session links?
			right = []string{connSpec.MCLAGDomain.PeerLinks[0].Switch2.DeviceName()}
			for _, link := range connSpec.MCLAGDomain.PeerLinks {
				// check that we have the same switches on both ends in each link // TODO add validation
				if link.Switch1.DeviceName() != left {
					return INVALID // TODO replace with error?
				}
				if link.Switch2.DeviceName() != right[0] {
					return INVALID // TODO replace with error?
				}
			}
		} else if connSpec.MCLAG != nil {
			role = "mclag"
			left = connSpec.MCLAG.Links[0].Server.DeviceName()
			for _, link := range connSpec.MCLAG.Links {
				// check we have the same server in each link // TODO add validation
				if link.Server.DeviceName() != left {
					return INVALID // TODO replace with error?
				}
				right = append(right, link.Switch.DeviceName())
			}
		} else if connSpec.ESLAG != nil {
			role = "eslag"
			left = connSpec.ESLAG.Links[0].Server.DeviceName()
			for _, link := range connSpec.ESLAG.Links {
				// check we have the same server in each link // TODO add validation
				if link.Server.DeviceName() != left {
					return INVALID // TODO replace with error?
				}
				right = append(right, link.Switch.DeviceName())
			}
		} else if connSpec.Fabric != nil {
			role = "fabric"
			left = connSpec.Fabric.Links[0].Spine.DeviceName()
			right = []string{connSpec.Fabric.Links[0].Leaf.DeviceName()}
		} else if connSpec.VPCLoopback != nil {
			role = "vpc-loopback"
			left = connSpec.VPCLoopback.Links[0].Switch1.DeviceName()
		} else if connSpec.External != nil {
			role = "external"
			left = connSpec.External.Link.Switch.DeviceName()
		} else if connSpec.StaticExternal != nil {
			role = "static-external"
			left = connSpec.StaticExternal.Link.Switch.DeviceName()
		}

		if left != "" && role != "" {
			if len(right) > 0 {
				return fmt.Sprintf("%s--%s--%s", left, role, strings.Join(right, "--"))
			}

			return fmt.Sprintf("%s--%s", left, role)
		}
	}

	return INVALID // TODO replace with error?
}

func (connSpec *ConnectionSpec) Type() string {
	if connSpec.Unbundled != nil {
		return ConnectionTypeUnbundled
	} else if connSpec.Bundled != nil {
		return ConnectionTypeBundled
	} else if connSpec.MCLAGDomain != nil {
		return ConnectionTypeMCLAGDomain
	} else if connSpec.MCLAG != nil {
		return ConnectionTypeMCLAG
	} else if connSpec.ESLAG != nil {
		return ConnectionTypeESLAG
	} else if connSpec.Fabric != nil {
		return ConnectionTypeFabric
	} else if connSpec.VPCLoopback != nil {
		return ConnectionTypeVPCLoopback
	} else if connSpec.External != nil {
		return ConnectionTypeExternal
	} else if connSpec.StaticExternal != nil {
		return ConnectionTypeStaticExternal
	}

	return INVALID
}

func (connSpec *ConnectionSpec) ConnectionLabels() map[string]string {
	labels := map[string]string{}

	labels[LabelConnectionType] = connSpec.Type()

	if connSpec.StaticExternal != nil && connSpec.StaticExternal.WithinVPC != "" {
		labels[LabelVPC] = connSpec.StaticExternal.WithinVPC
	}

	switches, servers, _, _, err := connSpec.Endpoints()
	// if error, we don't need to set labels
	if err != nil {
		return labels
	}

	for _, switchName := range switches {
		labels[ListLabelSwitch(switchName)] = ListLabelValue
	}
	for _, serverName := range servers {
		labels[ListLabelServer(serverName)] = ListLabelValue
	}

	return labels
}

func (connSpec *ConnectionSpec) Endpoints() ([]string, []string, []string, map[string]string, error) {
	switches := map[string]struct{}{}
	servers := map[string]struct{}{}
	ports := map[string]struct{}{}
	links := map[string]string{}

	nonNills := 0
	if connSpec.Unbundled != nil {
		nonNills++

		switches[connSpec.Unbundled.Link.Switch.DeviceName()] = struct{}{}
		servers[connSpec.Unbundled.Link.Server.DeviceName()] = struct{}{}
		ports[connSpec.Unbundled.Link.Switch.PortName()] = struct{}{}
		ports[connSpec.Unbundled.Link.Server.PortName()] = struct{}{}
		links[connSpec.Unbundled.Link.Switch.PortName()] = connSpec.Unbundled.Link.Server.PortName()

		if len(switches) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one switch must be used for unbundled connection")
		}
		if len(servers) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one server must be used for unbundled connection")
		}
		if len(ports) != 2 {
			return nil, nil, nil, nil, errors.Errorf("two unique ports must be used for unbundled connection")
		}
	} else if connSpec.Bundled != nil {
		nonNills++

		for _, link := range connSpec.Bundled.Links {
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
		if len(ports) != 2*len(connSpec.Bundled.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for bundled connection")
		}
	} else if connSpec.MCLAGDomain != nil {
		nonNills++

		for _, link := range connSpec.MCLAGDomain.PeerLinks {
			switches[link.Switch1.DeviceName()] = struct{}{}
			switches[link.Switch2.DeviceName()] = struct{}{}
			ports[link.Switch1.PortName()] = struct{}{}
			ports[link.Switch2.PortName()] = struct{}{}
			links[link.Switch1.PortName()] = link.Switch2.PortName()
		}
		for _, link := range connSpec.MCLAGDomain.SessionLinks {
			switches[link.Switch1.DeviceName()] = struct{}{}
			switches[link.Switch2.DeviceName()] = struct{}{}
			ports[link.Switch1.PortName()] = struct{}{}
			ports[link.Switch2.PortName()] = struct{}{}
			links[link.Switch1.PortName()] = link.Switch2.PortName()
		}

		if len(connSpec.MCLAGDomain.PeerLinks) < 1 {
			return nil, nil, nil, nil, errors.Errorf("at least one peer link must be used for mclag domain connection")
		}
		if len(connSpec.MCLAGDomain.SessionLinks) < 1 {
			return nil, nil, nil, nil, errors.Errorf("at least one session link must be used for mclag domain connection")
		}
		if len(switches) != 2 {
			return nil, nil, nil, nil, errors.Errorf("two switches must be used for mclag domain connection")
		}
		if len(ports) != 2*(len(connSpec.MCLAGDomain.PeerLinks)+len(connSpec.MCLAGDomain.SessionLinks)) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for mclag domain connection")
		}
	} else if connSpec.MCLAG != nil {
		nonNills++

		for _, link := range connSpec.MCLAG.Links {
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
		if len(ports) != 2*len(connSpec.MCLAG.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for mclag connection")
		}
	} else if connSpec.ESLAG != nil {
		nonNills++

		for _, link := range connSpec.ESLAG.Links {
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
		if len(ports) != 2*len(connSpec.ESLAG.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for eslag connection")
		}
	} else if connSpec.Fabric != nil {
		nonNills++

		for _, link := range connSpec.Fabric.Links {
			switches[link.Spine.DeviceName()] = struct{}{}
			switches[link.Leaf.DeviceName()] = struct{}{}
			ports[link.Spine.PortName()] = struct{}{}
			ports[link.Leaf.PortName()] = struct{}{}
			links[link.Spine.PortName()] = link.Leaf.PortName()
		}

		if len(switches) != 2 {
			return nil, nil, nil, nil, errors.Errorf("two switches must be used for fabric connection")
		}
		if len(ports) != 2*len(connSpec.Fabric.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for fabric connection")
		}
	} else if connSpec.VPCLoopback != nil {
		nonNills++

		for _, link := range connSpec.VPCLoopback.Links {
			switches[link.Switch1.DeviceName()] = struct{}{}
			switches[link.Switch2.DeviceName()] = struct{}{}
			ports[link.Switch1.PortName()] = struct{}{}
			ports[link.Switch2.PortName()] = struct{}{}
			links[link.Switch1.PortName()] = link.Switch2.PortName()
		}

		if len(switches) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one switches must be used for vpc-loopback connection")
		}
		if len(ports) != 2*len(connSpec.VPCLoopback.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for fabric connection")
		}
	} else if connSpec.External != nil {
		nonNills++

		switches[connSpec.External.Link.Switch.DeviceName()] = struct{}{}
		ports[connSpec.External.Link.Switch.PortName()] = struct{}{}
	} else if connSpec.StaticExternal != nil {
		nonNills++

		switches[connSpec.StaticExternal.Link.Switch.DeviceName()] = struct{}{}
		ports[connSpec.StaticExternal.Link.Switch.PortName()] = struct{}{}
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

func (connSpec *ConnectionSpec) LinkSummary(noColor bool) []string {
	colored := color.New(color.FgCyan).SprintFunc()
	if noColor {
		colored = func(a ...interface{}) string { return fmt.Sprint(a...) }
	}

	sep := colored("←→")

	out := []string{}
	if connSpec.Fabric != nil {
		for _, link := range connSpec.Fabric.Links {
			out = append(out, fmt.Sprintf("%s%s%s", link.Spine.PortName(), sep, link.Leaf.PortName()))
		}
	} else if connSpec.VPCLoopback != nil {
		for _, link := range connSpec.VPCLoopback.Links {
			out = append(out, fmt.Sprintf("%s%s%s", link.Switch1.PortName(), sep, link.Switch2.PortName()))
		}
	} else if connSpec.External != nil {
		out = append(out, connSpec.External.Link.Switch.PortName())
	} else if connSpec.StaticExternal != nil {
		vpc := ""

		if connSpec.StaticExternal.WithinVPC != "" {
			vpc = fmt.Sprintf("(%s%s)", colored("vpc:"), connSpec.StaticExternal.WithinVPC)
		}

		out = append(out, fmt.Sprintf("%s%s", connSpec.StaticExternal.Link.Switch.PortName(), vpc))
	} else if connSpec.MCLAGDomain != nil {
		for _, link := range connSpec.MCLAGDomain.PeerLinks {
			out = append(out, fmt.Sprintf("%s%s%s%s", colored("peer"), link.Switch1.PortName(), sep, link.Switch2.PortName()))
		}
		for _, link := range connSpec.MCLAGDomain.SessionLinks {
			out = append(out, fmt.Sprintf("%s%s%s%s", colored("session"), link.Switch1.PortName(), sep, link.Switch2.PortName()))
		}
	} else if connSpec.Unbundled != nil {
		out = append(out, fmt.Sprintf("%s%s%s", connSpec.Unbundled.Link.Server.PortName(), sep, connSpec.Unbundled.Link.Switch.PortName()))
	} else if connSpec.Bundled != nil {
		for _, link := range connSpec.Bundled.Links {
			out = append(out, fmt.Sprintf("%s%s%s", link.Server.PortName(), sep, link.Switch.PortName()))
		}
	} else if connSpec.MCLAG != nil {
		for _, link := range connSpec.MCLAG.Links {
			out = append(out, fmt.Sprintf("%s%s%s", link.Server.PortName(), sep, link.Switch.PortName()))
		}
	} else if connSpec.ESLAG != nil {
		for _, link := range connSpec.ESLAG.Links {
			out = append(out, fmt.Sprintf("%s%s%s", link.Server.PortName(), sep, link.Switch.PortName()))
		}
	}

	return out
}

func (conn *Connection) Default() {
	meta.DefaultObjectMetadata(conn)

	if conn.Labels == nil {
		conn.Labels = map[string]string{}
	}

	CleanupFabricLabels(conn.Labels)

	maps.Copy(conn.Labels, conn.Spec.ConnectionLabels())
}

func (connSpec *ConnectionSpec) ValidateServerFacingMTU(fabricMTU uint16, serverFacingMTUOffset uint16) error {
	if connSpec.Unbundled != nil && connSpec.Unbundled.MTU > fabricMTU-serverFacingMTUOffset {
		return errors.Errorf("unbundled connection mtu %d is greater than fabric mtu %d - server facing mtu offset %d", connSpec.Unbundled.MTU, fabricMTU, serverFacingMTUOffset)
	}
	if connSpec.Bundled != nil && connSpec.Bundled.MTU > fabricMTU-serverFacingMTUOffset {
		return errors.Errorf("bundled connection mtu %d is greater than fabric mtu %d - server facing mtu offset %d", connSpec.Bundled.MTU, fabricMTU, serverFacingMTUOffset)
	}
	if connSpec.MCLAG != nil && connSpec.MCLAG.MTU > fabricMTU-serverFacingMTUOffset {
		return errors.Errorf("mclag connection mtu %d is greater than fabric mtu %d - server facing mtu offset %d", connSpec.MCLAG.MTU, fabricMTU, serverFacingMTUOffset)
	}

	return nil
}

func (conn *Connection) Validate(ctx context.Context, kube client.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(conn); err != nil {
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

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
			ip, ipNet, err := net.ParseCIDR(subnet)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse cidr %s", subnet)
			}

			if subnet == "0.0.0.0/0" {
				if len(se.Subnets) > 1 {
					return nil, errors.Errorf("default route should be the only subnet")
				}

				break
			}

			if !ipNet.IP.Equal(ip) {
				return nil, errors.Errorf("invalid subnet %s: inconsistent IP address and mask", subnet)
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

			sp := &SwitchProfile{}
			err = kube.Get(ctx, types.NamespacedName{Name: sw.Spec.Profile, Namespace: conn.Namespace}, sp) // TODO namespace could be different?
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("switch profile %s not found", sw.Spec.Profile)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get switch profile %s", sw.Spec.Profile) // TODO replace with some internal error to not expose to the user
			}

			allowedPorts, err := sp.Spec.GetAPI2NOSPortsFor(&sw.Spec)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get NOS port mapping for switch %s", switchName)
			}

			for _, port := range ports {
				if !strings.HasPrefix(port, switchName+"/") {
					continue
				}

				portName := strings.TrimPrefix(port, switchName+"/")

				if _, ok := allowedPorts[portName]; !ok {
					return nil, errors.Errorf("port %s is not allowed for switch %s", port, switchName)
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
