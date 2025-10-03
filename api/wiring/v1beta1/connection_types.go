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
	"maps"
	"net"
	"net/netip"
	"strings"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	ConnectionTypeUnbundled      = "unbundled"
	ConnectionTypeBundled        = "bundled"
	ConnectionTypeMCLAG          = "mclag"
	ConnectionTypeMCLAGDomain    = "mclag-domain"
	ConnectionTypeESLAG          = "eslag"
	ConnectionTypeFabric         = "fabric"
	ConnectionTypeMesh           = "mesh"
	ConnectionTypeGateway        = "gateway"
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
	// Port defines the full name of the switch port in the format of "device/port", such as "spine-1/E1/1".
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

// ConnFabricLinkSwitch defines the switch side of the fabric (or gateway) link
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

// MeshLink defines the mesh connection link, i.e. a direct leaf to leaf connection
type MeshLink struct {
	Leaf1 ConnFabricLinkSwitch `json:"leaf1,omitempty"`
	Leaf2 ConnFabricLinkSwitch `json:"leaf2,omitempty"`
}

// ConnFabric defines the fabric connection (single spine to a single leaf with at least one link)
type ConnFabric struct {
	//+kubebuilder:validation:MinItems=1
	// Links is the list of spine-to-leaf links
	Links []FabricLink `json:"links,omitempty"`
}

// ConnGatewayLinkGateway defines the gateway side of the gateway link
type ConnGatewayLinkGateway struct {
	// BasePortName defines the full name of the gateway port
	BasePortName `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	// IP is the IP address of the switch side of the fabric link (switch port configuration)
	IP string `json:"ip,omitempty"`
}

// GatewayLink defines the gateway connection link
type GatewayLink struct {
	// Switch is the switch (spine or leaf) side of the gateway link
	Switch ConnFabricLinkSwitch `json:"switch,omitempty"`
	// Gateway is the gateway side of the gateway link
	Gateway ConnGatewayLinkGateway `json:"gateway,omitempty"`
}

// ConnGateway defines the gateway connection (single spine to a single gateway with at least one link)
type ConnGateway struct {
	//+kubebuilder:validation:MinItems=1
	// Links is the list of spine to gateway links
	Links []GatewayLink `json:"links,omitempty"`
}

// ConnMesh defines the mesh connection (direct leaf to leaf connection with at least one link)
type ConnMesh struct {
	//+kubebuilder:validation:MinItems=1
	// Links is the list of leaf to leaf links
	Links []MeshLink `json:"links,omitempty"`
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
	// Mesh defines the mesh connection (direct leaf to leaf connection with at least one link)
	Mesh *ConnMesh `json:"mesh,omitempty"`
	// Gateway defines the gateway connection (single spine to a single gateway with at least one link)
	Gateway *ConnGateway `json:"gateway,omitempty"`
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
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the Connection
	Spec ConnectionSpec `json:"spec,omitempty"`
	// Status is the observed state of the Connection
	Status ConnectionStatus `json:"status,omitempty"`
}

const KindConnection = "Connection"

//+kubebuilder:object:root=true

// ConnectionList contains a list of Connection
type ConnectionList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []Connection `json:"items"`
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

		if connSpec.Unbundled != nil { //nolint:gocritic
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
		} else if connSpec.Mesh != nil {
			role = "mesh"
			left = connSpec.Mesh.Links[0].Leaf1.DeviceName()
			right = []string{connSpec.Mesh.Links[0].Leaf2.DeviceName()}
		} else if connSpec.Gateway != nil {
			role = "gateway"
			left = connSpec.Gateway.Links[0].Switch.DeviceName()
			right = []string{connSpec.Gateway.Links[0].Gateway.DeviceName()}
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
	if connSpec.Unbundled != nil { //nolint:gocritic
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
	} else if connSpec.Mesh != nil {
		return ConnectionTypeMesh
	} else if connSpec.Gateway != nil {
		return ConnectionTypeGateway
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
	gateways := map[string]struct{}{}
	ports := map[string]struct{}{}
	links := map[string]string{}

	nonNills := 0
	if connSpec.Unbundled != nil { //nolint:gocritic
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

		if len(switches) < 1 {
			return nil, nil, nil, nil, errors.Errorf("at least one switch must be used for mclag connection")
		}
		if len(switches) > 2 {
			return nil, nil, nil, nil, errors.Errorf("at most two switches must be used for mclag connection")
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

		if len(switches) < 1 {
			return nil, nil, nil, nil, errors.Errorf("at least one switch must be used for eslag connection")
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
	} else if connSpec.Mesh != nil {
		nonNills++

		for _, link := range connSpec.Mesh.Links {
			switches[link.Leaf1.DeviceName()] = struct{}{}
			switches[link.Leaf2.DeviceName()] = struct{}{}
			ports[link.Leaf1.PortName()] = struct{}{}
			ports[link.Leaf2.PortName()] = struct{}{}
			links[link.Leaf1.PortName()] = link.Leaf2.PortName()
		}

		if len(switches) != 2 {
			return nil, nil, nil, nil, errors.Errorf("two switches must be used for mesh connection")
		}
		if len(ports) != 2*len(connSpec.Mesh.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for mesh connection")
		}
	} else if connSpec.Gateway != nil {
		nonNills++

		for _, link := range connSpec.Gateway.Links {
			switches[link.Switch.DeviceName()] = struct{}{}
			gateways[link.Gateway.DeviceName()] = struct{}{}
			ports[link.Switch.PortName()] = struct{}{}
			ports[link.Gateway.PortName()] = struct{}{}
			links[link.Switch.PortName()] = link.Gateway.PortName()
		}

		if len(switches) != 1 {
			return nil, nil, nil, nil, errors.Errorf("one switches must be used for gateway connection")
		}
		if len(ports) != 2*len(connSpec.Gateway.Links) {
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for gateway connection")
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
			return nil, nil, nil, nil, errors.Errorf("unique ports must be used for vpc-loopback connection")
		}
	} else if connSpec.External != nil {
		nonNills++

		switches[connSpec.External.Link.Switch.DeviceName()] = struct{}{}
		ports[connSpec.External.Link.Switch.PortName()] = struct{}{}
		links[connSpec.External.Link.Switch.PortName()] = "/"
	} else if connSpec.StaticExternal != nil {
		nonNills++

		switches[connSpec.StaticExternal.Link.Switch.DeviceName()] = struct{}{}
		ports[connSpec.StaticExternal.Link.Switch.PortName()] = struct{}{}
		links[connSpec.StaticExternal.Link.Switch.PortName()] = "/"
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

	return lo.Keys(switches), lo.Keys(servers), lo.Keys(ports), links, nil
}

func (connSpec *ConnectionSpec) LinkSummary(noColor bool) []string {
	colored := color.New(color.FgCyan).SprintFunc()
	if noColor {
		colored = fmt.Sprint
	}

	sep := colored("←→")

	out := []string{}
	if connSpec.Fabric != nil { //nolint:gocritic
		for _, link := range connSpec.Fabric.Links {
			out = append(out, fmt.Sprintf("%s%s%s", link.Spine.PortName(), sep, link.Leaf.PortName()))
		}
	} else if connSpec.Mesh != nil {
		for _, link := range connSpec.Mesh.Links {
			out = append(out, fmt.Sprintf("%s%s%s", link.Leaf1.PortName(), sep, link.Leaf2.PortName()))
		}
	} else if connSpec.Gateway != nil {
		for _, link := range connSpec.Gateway.Links {
			out = append(out, fmt.Sprintf("%s%s%s", link.Switch.PortName(), sep, link.Gateway.PortName()))
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

func (conn *Connection) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
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

	if conn.Spec.Gateway != nil && fabricCfg != nil && fabricCfg.FabricMode != meta.FabricModeSpineLeaf {
		return nil, errors.Errorf("gateway connection is not allowed in current fabric configuration")
	}

	if kube != nil {
		rGroup := ""
		rType := meta.RedundancyTypeNone

		for _, switchName := range switches {
			sw := &Switch{}
			err := kube.Get(ctx, ktypes.NamespacedName{Name: switchName, Namespace: conn.Namespace}, sw) // TODO namespace could be different?
			if kapierrors.IsNotFound(err) {
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
			err = kube.Get(ctx, ktypes.NamespacedName{Name: sw.Spec.Profile, Namespace: conn.Namespace}, sp) // TODO namespace could be different?
			if kapierrors.IsNotFound(err) {
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
			err := kube.Get(ctx, ktypes.NamespacedName{Name: serverName, Namespace: conn.Namespace}, &Server{}) // TODO namespace could be different?
			if kapierrors.IsNotFound(err) {
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
		if err := kube.List(ctx, conns, &kclient.ListOptions{Namespace: conn.Namespace}); err != nil { // TODO namespace could be different?
			return nil, errors.Wrapf(err, "failed to list connections")
		}

		fabricSubnet, err := netip.ParsePrefix(fabricCfg.FabricSubnet)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse fabric subnet %s", fabricCfg.FabricSubnet)
		}
		fabricIPs := map[netip.Addr]bool{}
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
			switch {
			case other.Spec.Fabric != nil:
				for _, link := range other.Spec.Fabric.Links {
					if link.Spine.IP != "" {
						if ip, err := netip.ParsePrefix(link.Spine.IP); err == nil {
							fabricIPs[ip.Addr()] = true
						}
					}
					if link.Leaf.IP != "" {
						if ip, err := netip.ParsePrefix(link.Leaf.IP); err == nil {
							fabricIPs[ip.Addr()] = true
						}
					}
				}
			case other.Spec.Mesh != nil:
				for _, link := range other.Spec.Mesh.Links {
					if link.Leaf1.IP != "" {
						if ip, err := netip.ParsePrefix(link.Leaf1.IP); err == nil {
							fabricIPs[ip.Addr()] = true
						}
					}
					if link.Leaf2.IP != "" {
						if ip, err := netip.ParsePrefix(link.Leaf2.IP); err == nil {
							fabricIPs[ip.Addr()] = true
						}
					}
				}
			case other.Spec.Gateway != nil:
				for _, link := range other.Spec.Gateway.Links {
					if link.Gateway.IP != "" {
						if ip, err := netip.ParsePrefix(link.Gateway.IP); err == nil {
							fabricIPs[ip.Addr()] = true
						}
					}
					if link.Switch.IP != "" {
						if ip, err := netip.ParsePrefix(link.Switch.IP); err == nil {
							fabricIPs[ip.Addr()] = true
						}
					}
				}
			}
		}

		if conn.Spec.Fabric != nil { //nolint:gocritic
			cf := conn.Spec.Fabric
			for idx, link := range cf.Links {
				if link.Spine.IP == "" || link.Leaf.IP == "" {
					continue
				}

				spinePrefix, err := netip.ParsePrefix(link.Spine.IP)
				if err != nil {
					return nil, errors.Wrapf(err, "parsing fabric connection %s link %d spine IP %s", conn.Name, idx, link.Spine.IP)
				}
				if spinePrefix.Bits() != 31 {
					return nil, errors.Errorf("fabric connection %s link %d spine IP %s is not a /31", conn.Name, idx, spinePrefix) //nolint:goerr113
				}

				spineIP := spinePrefix.Addr()
				if !fabricSubnet.Contains(spineIP) {
					return nil, errors.Errorf("fabric connection %s link %d spine IP %s is not in the fabric subnet %s", conn.Name, idx, spineIP, fabricSubnet) //nolint:goerr113
				}
				if _, exist := fabricIPs[spineIP]; exist {
					return nil, errors.Errorf("fabric connection %s link %d spine IP %s is already in use", conn.Name, idx, spineIP) //nolint:goerr113
				}
				fabricIPs[spineIP] = true

				leafPrefix, err := netip.ParsePrefix(link.Leaf.IP)
				if err != nil {
					return nil, errors.Wrapf(err, "parsing fabric connection %s link %d leaf IP %s", conn.Name, idx, link.Leaf.IP)
				}
				if leafPrefix.Bits() != 31 {
					return nil, errors.Errorf("fabric connection %s link %d leaf IP %s is not a /31", conn.Name, idx, leafPrefix) //nolint:goerr113
				}

				leafIP := leafPrefix.Addr()
				if !fabricSubnet.Contains(leafIP) {
					return nil, errors.Errorf("fabric connection %s link %d leaf IP %s is not in the fabric subnet %s", conn.Name, idx, leafIP, fabricSubnet) //nolint:goerr113
				}
				if _, exist := fabricIPs[leafIP]; exist {
					return nil, errors.Errorf("fabric connection %s link %d leaf IP %s is already in use", conn.Name, idx, leafIP) //nolint:goerr113
				}
				fabricIPs[leafIP] = true

				if spinePrefix.Masked() != leafPrefix.Masked() {
					return nil, errors.Errorf("fabric connection %s link %d spine IP %s and leaf IP %s are not in the same subnet", conn.Name, idx, spineIP, leafIP) //nolint:goerr113
				}
			}
		} else if conn.Spec.Mesh != nil {
			cm := conn.Spec.Mesh
			for idx, link := range cm.Links {
				if link.Leaf1.IP == "" || link.Leaf2.IP == "" {
					continue
				}

				leaf1Prefix, err := netip.ParsePrefix(link.Leaf1.IP)
				if err != nil {
					return nil, errors.Wrapf(err, "parsing mesh connection %s link %d leaf1 IP %s", conn.Name, idx, link.Leaf1.IP)
				}
				if leaf1Prefix.Bits() != 31 {
					return nil, errors.Errorf("mesh connection %s link %d leaf1 IP %s is not a /31", conn.Name, idx, leaf1Prefix) //nolint:goerr113
				}

				leaf1IP := leaf1Prefix.Addr()
				if !fabricSubnet.Contains(leaf1IP) {
					return nil, errors.Errorf("mesh connection %s link %d leaf1 IP %s is not in the fabric subnet %s", conn.Name, idx, leaf1IP, fabricSubnet) //nolint:goerr113
				}
				if _, exist := fabricIPs[leaf1IP]; exist {
					return nil, errors.Errorf("mesh connection %s link %d leaf1 IP %s is already in use", conn.Name, idx, leaf1IP) //nolint:goerr113
				}
				fabricIPs[leaf1IP] = true

				leaf2Prefix, err := netip.ParsePrefix(link.Leaf2.IP)
				if err != nil {
					return nil, errors.Wrapf(err, "parsing mesh connection %s link %d leaf2 IP %s", conn.Name, idx, link.Leaf2.IP)
				}
				if leaf2Prefix.Bits() != 31 {
					return nil, errors.Errorf("mesh connection %s link %d leaf2 IP %s is not a /31", conn.Name, idx, leaf2Prefix) //nolint:goerr113
				}

				leaf2IP := leaf2Prefix.Addr()
				if !fabricSubnet.Contains(leaf2IP) {
					return nil, errors.Errorf("mesh connection %s link %d leaf2 IP %s is not in the fabric subnet %s", conn.Name, idx, leaf2IP, fabricSubnet) //nolint:goerr113
				}
				if _, exist := fabricIPs[leaf2IP]; exist {
					return nil, errors.Errorf("mesh connection %s link %d leaf2 IP %s is already in use", conn.Name, idx, leaf2IP) //nolint:goerr113
				}
				fabricIPs[leaf2IP] = true

				if leaf1Prefix.Masked() != leaf2Prefix.Masked() {
					return nil, errors.Errorf("mesh connection %s link %d leaf1 IP %s and leaf2 IP %s are not in the same subnet", conn.Name, idx, leaf1IP, leaf2IP) //nolint:goerr113
				}
			}
		} else if conn.Spec.Gateway != nil {
			cg := conn.Spec.Gateway
			for idx, link := range cg.Links {
				if link.Switch.IP == "" || link.Gateway.IP == "" {
					continue
				}

				switchPrefix, err := netip.ParsePrefix(link.Switch.IP)
				if err != nil {
					return nil, errors.Wrapf(err, "parsing gateway connection %s link %d switch IP %s", conn.Name, idx, link.Switch.IP)
				}
				if switchPrefix.Bits() != 31 {
					return nil, errors.Errorf("gateway connection %s link %d switch IP %s is not a /31", conn.Name, idx, switchPrefix) //nolint:goerr113
				}

				switchIP := switchPrefix.Addr()
				if !fabricSubnet.Contains(switchIP) {
					return nil, errors.Errorf("gateway connection %s link %d switch IP %s is not in the fabric subnet %s", conn.Name, idx, switchIP, fabricSubnet) //nolint:goerr113
				}
				if _, exist := fabricIPs[switchIP]; exist {
					return nil, errors.Errorf("gateway connection %s link %d switch IP %s is already in use", conn.Name, idx, switchIP) //nolint:goerr113
				}
				fabricIPs[switchIP] = true

				gwPrefix, err := netip.ParsePrefix(link.Gateway.IP)
				if err != nil {
					return nil, errors.Wrapf(err, "parsing gateway connection %s link %d gateway IP %s", conn.Name, idx, link.Gateway.IP)
				}
				if gwPrefix.Bits() != 31 {
					return nil, errors.Errorf("gateway connection %s link %d gateway IP %s is not a /31", conn.Name, idx, gwPrefix) //nolint:goerr113
				}

				gwIP := gwPrefix.Addr()
				if !fabricSubnet.Contains(gwIP) {
					return nil, errors.Errorf("gateway connection %s link %d gateway IP %s is not in the fabric subnet %s", conn.Name, idx, gwIP, fabricSubnet) //nolint:goerr113
				}
				if _, exist := fabricIPs[gwIP]; exist {
					return nil, errors.Errorf("gateway connection %s link %d gateway IP %s is already in use", conn.Name, idx, gwIP) //nolint:goerr113
				}
				fabricIPs[gwIP] = true

				if switchPrefix.Masked() != gwPrefix.Masked() {
					return nil, errors.Errorf("gateway connection %s link %d switch IP %s and gateway IP %s are not in the same subnet", conn.Name, idx, switchIP, gwIP) //nolint:goerr113
				}
			}
		}
	}

	return nil, nil
}
