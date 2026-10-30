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
	"slices"
	"strings"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
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

// ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled etc.) configuration
type ServerFacingConnectionConfig struct {
	// MTU is the MTU to be configured on the switch port or port channel
	MTU uint16 `json:"mtu,omitempty"`
}

// ConnUnbundled defines the unbundled connection (no port channel, single server to a single switch with a single link)
type ConnUnbundled struct {
	// Link is the server-to-switch link
	Link ServerToSwitchLink `json:"link,omitempty"`
	// ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled etc.) configuration
	ServerFacingConnectionConfig `json:",inline"`
}

// ConnBundled defines the bundled connection (port channel, single server to a single switch with multiple links)
type ConnBundled struct {
	// Links is the list of server-to-switch links
	Links []ServerToSwitchLink `json:"links,omitempty"`
	// ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled etc.) configuration
	ServerFacingConnectionConfig `json:",inline"`
	// Fallback is the optional flag that used to indicate one of the links in LACP port channel to be used as a fallback link
	Fallback bool `json:"fallback,omitempty"`
}

// Deprecated: ConnMCLAG defines the MCLAG connection (port channel, single server to pair of switches with multiple links)
type ConnMCLAG struct {
	// +kubebuilder:validation:MinItems=2
	// Links is the list of server-to-switch links
	Links []ServerToSwitchLink `json:"links,omitempty"`
	// ServerFacingConnectionConfig defines any server-facing connection (unbundled, bundled, mclag, etc.) configuration
	ServerFacingConnectionConfig `json:",inline"`
	// Fallback is the optional flag that used to indicate one of the links in LACP port channel to be used as a fallback link
	Fallback bool `json:"fallback,omitempty"`
}

// ConnESLAG defines the ESLAG connection (port channel, single server to 2-4 switches with multiple links)
type ConnESLAG struct {
	// +kubebuilder:validation:MinItems=2
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

// Deprecated: ConnMCLAGDomain defines the MCLAG domain connection which makes two switches into a single logical switch or
// redundancy group and allows to use MCLAG connections to connect servers in a multi-homed way.
type ConnMCLAGDomain struct {
	// +kubebuilder:validation:MinItems=1
	// PeerLinks is the list of peer links between the switches, used to pass server traffic between switch
	PeerLinks []SwitchToSwitchLink `json:"peerLinks,omitempty"`

	// +kubebuilder:validation:MinItems=1
	// SessionLinks is the list of session links between the switches, used only to pass MCLAG control plane and BGP
	// traffic between switches
	SessionLinks []SwitchToSwitchLink `json:"sessionLinks,omitempty"`
}

// ConnFabricLinkSwitch defines the switch side of the fabric (or gateway) link
type ConnFabricLinkSwitch struct {
	// BasePortName defines the full name of the switch port
	BasePortName `json:",inline"`
	// +kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
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
	// +kubebuilder:validation:MinItems=1
	// Links is the list of spine-to-leaf links
	Links []FabricLink `json:"links,omitempty"`
}

// ConnGatewayLinkGateway defines the gateway side of the gateway link
type ConnGatewayLinkGateway struct {
	// BasePortName defines the full name of the gateway port
	BasePortName `json:",inline"`
	// +kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
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
	// +kubebuilder:validation:MinItems=1
	// Links is the list of spine to gateway links
	Links []GatewayLink `json:"links,omitempty"`
}

// ConnMesh defines the mesh connection (direct leaf to leaf connection with at least one link)
type ConnMesh struct {
	// +kubebuilder:validation:MinItems=1
	// Links is the list of leaf to leaf links
	Links []MeshLink `json:"links,omitempty"`
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
	// +kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	// IP is the IP address of the switch side of the static external connection link (switch port configuration)
	IP string `json:"ip,omitempty"`
	// +kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}$`
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

// ConnectionTopology is where a Connection sits in the fabric topology
type ConnectionTopology struct {
	// Fabric is the name of the Fabric this connection belongs to (if not specified, "default" is used).
	// It is the Fabric the connected devices are in, unrelated to the fabric connection type
	Fabric string `json:"fabric,omitempty"`
}

// ConnectionSpec defines the desired state of Connection
type ConnectionSpec struct {
	// Topology is where the connection sits in the fabric topology
	Topology ConnectionTopology `json:"topology,omitempty"`
	// Unbundled defines the unbundled connection (no port channel, single server to a single switch with a single link)
	Unbundled *ConnUnbundled `json:"unbundled,omitempty"`
	// Bundled defines the bundled connection (port channel, single server to a single switch with multiple links)
	Bundled *ConnBundled `json:"bundled,omitempty"`
	// Deprecated: MCLAG defines the MCLAG connection (port channel, single server to pair of switches with multiple links)
	MCLAG *ConnMCLAG `json:"mclag,omitempty"`
	// ESLAG defines the ESLAG connection (port channel, single server to 2-4 switches with multiple links)
	ESLAG *ConnESLAG `json:"eslag,omitempty"`
	// Deprecated: MCLAGDomain defines the MCLAG domain connection which makes two switches into a single logical switch for server multi-homing
	MCLAGDomain *ConnMCLAGDomain `json:"mclagDomain,omitempty"`
	// Fabric defines the fabric connection (single spine to a single leaf with at least one link)
	Fabric *ConnFabric `json:"fabric,omitempty"`
	// Mesh defines the mesh connection (direct leaf to leaf connection with at least one link)
	Mesh *ConnMesh `json:"mesh,omitempty"`
	// Gateway defines the gateway connection (single spine to a single gateway with at least one link)
	Gateway *ConnGateway `json:"gateway,omitempty"`
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
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/connection-type`,priority=0
// +kubebuilder:printcolumn:name="Switches",type=string,JSONPath=`.metadata.annotations.fabric\.githedgehog\.com/switches`,priority=1
// +kubebuilder:printcolumn:name="Servers",type=string,JSONPath=`.metadata.annotations.fabric\.githedgehog\.com/servers`,priority=1
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

// +kubebuilder:object:root=true

// ConnectionList contains a list of Connection
type ConnectionList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []Connection `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &Connection{}, &ConnectionList{})

		return nil
	})
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
	} else if connSpec.ESLAG != nil {
		return ConnectionTypeESLAG
	} else if connSpec.Fabric != nil {
		return ConnectionTypeFabric
	} else if connSpec.Mesh != nil {
		return ConnectionTypeMesh
	} else if connSpec.Gateway != nil {
		return ConnectionTypeGateway
	} else if connSpec.External != nil {
		return ConnectionTypeExternal
	} else if connSpec.StaticExternal != nil {
		return ConnectionTypeStaticExternal
	}

	return INVALID
}

func (connSpec *ConnectionSpec) ConnectionLabelsAnnotations() (map[string]string, map[string]string) {
	labels := map[string]string{}
	anns := map[string]string{}

	labels[LabelConnectionType] = connSpec.Type()

	if connSpec.StaticExternal != nil && connSpec.StaticExternal.WithinVPC != "" {
		labels[LabelVPC] = connSpec.StaticExternal.WithinVPC
	}

	switches, servers, _, _, err := connSpec.Endpoints()
	// if error, we don't need to set labels
	if err != nil {
		return labels, anns
	}

	for _, switchName := range switches {
		labels[ListLabelSwitch(switchName)] = ListLabelValue
	}
	for _, serverName := range servers {
		labels[ListLabelServer(serverName)] = ListLabelValue
	}

	slices.Sort(switches)
	slices.Sort(servers)

	anns[LabelSwitches] = strings.Join(switches, ",")
	anns[LabelServers] = strings.Join(servers, ",")

	return labels, anns
}

func (connSpec *ConnectionSpec) Endpoints() ([]string, []string, []string, map[string]string, error) {
	switches := map[string]struct{}{}
	servers := map[string]struct{}{}
	gateways := map[string]struct{}{}
	ports := map[string]struct{}{}
	links := map[string]string{}

	nonNills := 0

	// TODO: delete when we fully remove MCLAG
	if connSpec.MCLAG != nil || connSpec.MCLAGDomain != nil {
		return []string{}, []string{}, []string{}, links, nil
	}
	if connSpec.Unbundled != nil { //nolint:gocritic
		nonNills++

		switches[connSpec.Unbundled.Link.Switch.DeviceName()] = struct{}{}
		servers[connSpec.Unbundled.Link.Server.DeviceName()] = struct{}{}
		ports[connSpec.Unbundled.Link.Switch.PortName()] = struct{}{}
		ports[connSpec.Unbundled.Link.Server.PortName()] = struct{}{}
		links[connSpec.Unbundled.Link.Switch.PortName()] = connSpec.Unbundled.Link.Server.PortName()

		if len(switches) != 1 {
			return nil, nil, nil, nil, fmt.Errorf("one switch must be used for unbundled connection") //nolint:err113
		}
		if len(servers) != 1 {
			return nil, nil, nil, nil, fmt.Errorf("one server must be used for unbundled connection") //nolint:err113
		}
		if len(ports) != 2 {
			return nil, nil, nil, nil, fmt.Errorf("two unique ports must be used for unbundled connection") //nolint:err113
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
			return nil, nil, nil, nil, fmt.Errorf("one switch must be used for bundled connection") //nolint:err113
		}
		if len(servers) != 1 {
			return nil, nil, nil, nil, fmt.Errorf("one server must be used for bundled connection") //nolint:err113
		}
		if len(ports) != 2*len(connSpec.Bundled.Links) {
			return nil, nil, nil, nil, fmt.Errorf("unique ports must be used for bundled connection") //nolint:err113
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
			return nil, nil, nil, nil, fmt.Errorf("at least one switch must be used for eslag connection") //nolint:err113
		}
		if len(switches) > 4 {
			return nil, nil, nil, nil, fmt.Errorf("at most four switches must be used for eslag connection") //nolint:err113
		}
		if len(servers) != 1 {
			return nil, nil, nil, nil, fmt.Errorf("one server must be used for eslag connection") //nolint:err113
		}
		if len(ports) != 2*len(connSpec.ESLAG.Links) {
			return nil, nil, nil, nil, fmt.Errorf("unique ports must be used for eslag connection") //nolint:err113
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
			return nil, nil, nil, nil, fmt.Errorf("two switches must be used for fabric connection") //nolint:err113
		}
		if len(ports) != 2*len(connSpec.Fabric.Links) {
			return nil, nil, nil, nil, fmt.Errorf("unique ports must be used for fabric connection") //nolint:err113
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
			return nil, nil, nil, nil, fmt.Errorf("two switches must be used for mesh connection") //nolint:err113
		}
		if len(ports) != 2*len(connSpec.Mesh.Links) {
			return nil, nil, nil, nil, fmt.Errorf("unique ports must be used for mesh connection") //nolint:err113
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
			return nil, nil, nil, nil, fmt.Errorf("one switches must be used for gateway connection") //nolint:err113
		}
		if len(ports) != 2*len(connSpec.Gateway.Links) {
			return nil, nil, nil, nil, fmt.Errorf("unique ports must be used for gateway connection") //nolint:err113
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
		return nil, nil, nil, nil, fmt.Errorf("exactly one connection type must be used") //nolint:err113
	}

	for port := range ports {
		parts := SplitPortName(port)

		if len(parts) != 2 {
			return nil, nil, nil, nil, fmt.Errorf("invalid port name %s", port) //nolint:err113
		}

		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil, nil, nil, nil, fmt.Errorf("invalid port name %s, should be \"<device>/<port>\" format", port) //nolint:err113
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
	} else if connSpec.External != nil {
		out = append(out, connSpec.External.Link.Switch.PortName())
	} else if connSpec.StaticExternal != nil {
		vpc := ""

		if connSpec.StaticExternal.WithinVPC != "" {
			vpc = fmt.Sprintf("(%s%s)", colored("vpc:"), connSpec.StaticExternal.WithinVPC)
		}

		out = append(out, fmt.Sprintf("%s%s", connSpec.StaticExternal.Link.Switch.PortName(), vpc))
	} else if connSpec.Unbundled != nil {
		out = append(out, fmt.Sprintf("%s%s%s", connSpec.Unbundled.Link.Server.PortName(), sep, connSpec.Unbundled.Link.Switch.PortName()))
	} else if connSpec.Bundled != nil {
		for _, link := range connSpec.Bundled.Links {
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

	if conn.Spec.Topology.Fabric == "" {
		conn.Spec.Topology.Fabric = DefaultFabric
	}

	if conn.Labels == nil {
		conn.Labels = map[string]string{}
	}
	if conn.Annotations == nil {
		conn.Annotations = map[string]string{}
	}

	CleanupFabricLabels(conn.Labels)
	CleanupFabricLabels(conn.Annotations)

	labels, anns := conn.Spec.ConnectionLabelsAnnotations()
	maps.Copy(conn.Labels, labels)
	maps.Copy(conn.Annotations, anns)

	conn.Labels[ListLabelFabric(conn.Spec.Topology.Fabric)] = ListLabelValue
}

// ValidateDomains checks the connection does not cross domains where it must not, given its switches by name
func (connSpec *ConnectionSpec) ValidateDomains(switches map[string]*Switch) error {
	// a stored switch the refresh on fabric-ctrl initialization had to leave alone may have none, and the checks
	// below would let a link through when there's nothing to compare
	for _, name := range slices.Sorted(maps.Keys(switches)) {
		if len(switches[name].Spec.Topology.Domains) == 0 {
			return fmt.Errorf("switch %s has no domains", name) //nolint:err113
		}
	}

	domainsOf := func(name string) []string {
		if sw, exists := switches[name]; exists {
			return sw.Spec.Topology.Domains
		}

		return nil
	}

	if connSpec.Fabric != nil {
		for _, link := range connSpec.Fabric.Links {
			spine, leaf := link.Spine.DeviceName(), link.Leaf.DeviceName()
			for _, domain := range domainsOf(spine) {
				if !slices.Contains(domainsOf(leaf), domain) {
					return fmt.Errorf("spine %s is in domain %s but leaf %s is in domains %v", spine, domain, leaf, domainsOf(leaf)) //nolint:err113
				}
			}
		}
	}

	// a mesh leaf transits by design, so a mesh link across domains would merge them
	if connSpec.Mesh != nil {
		for _, link := range connSpec.Mesh.Links {
			leaf1, leaf2 := link.Leaf1.DeviceName(), link.Leaf2.DeviceName()
			for _, leaf := range []string{leaf1, leaf2} {
				if domains := domainsOf(leaf); len(domains) != 1 {
					return fmt.Errorf("mesh leaf %s must be in exactly one domain, found %v", leaf, domains) //nolint:err113
				}
			}
			if domains1, domains2 := domainsOf(leaf1), domainsOf(leaf2); domains1[0] != domains2[0] {
				return fmt.Errorf("mesh leaf %s is in domain %s but leaf %s is in domain %s", leaf1, domains1[0], leaf2, domains2[0]) //nolint:err113
			}
		}
	}

	// a leaf with a gateway connection advertises every VTEP it knows to its spines, which would
	// hand one domain's VTEPs to the other
	if connSpec.Gateway != nil {
		for _, link := range connSpec.Gateway.Links {
			name := link.Switch.DeviceName()
			if domains := domainsOf(name); len(domains) != 1 {
				return fmt.Errorf("switch %s with a gateway connection must be in exactly one domain, found %v", name, domains) //nolint:err113
			}
		}
	}

	// routes from an external enter the fabric at its border leaf with no spine ASN in their path, so
	// on a leaf in two domains the spine filter could not keep them in one
	borderLeaf := ""
	if connSpec.External != nil {
		borderLeaf = connSpec.External.Link.Switch.DeviceName()
	}
	if connSpec.StaticExternal != nil {
		borderLeaf = connSpec.StaticExternal.Link.Switch.DeviceName()
	}
	if borderLeaf != "" {
		if domains := domainsOf(borderLeaf); len(domains) != 1 {
			return fmt.Errorf("switch %s with an external connection must be in exactly one domain, found %v", borderLeaf, domains) //nolint:err113
		}
	}

	return nil
}

func (connSpec *ConnectionSpec) ValidateServerFacingMTU(fabricMTU uint16, serverFacingMTUOffset uint16) error {
	if connSpec.Unbundled != nil && connSpec.Unbundled.MTU > fabricMTU-serverFacingMTUOffset {
		return field.Invalid(field.NewPath("spec", "unbundled", "mtu"), connSpec.Unbundled.MTU, fmt.Sprintf("greater than fabric mtu %d - server facing mtu offset %d", fabricMTU, serverFacingMTUOffset))
	}
	if connSpec.Bundled != nil && connSpec.Bundled.MTU > fabricMTU-serverFacingMTUOffset {
		return field.Invalid(field.NewPath("spec", "bundled", "mtu"), connSpec.Bundled.MTU, fmt.Sprintf("greater than fabric mtu %d - server facing mtu offset %d", fabricMTU, serverFacingMTUOffset))
	}

	return nil
}

func (conn *Connection) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(conn); err != nil {
		return nil, err //nolint:wrapcheck
	}

	spec := field.NewPath("spec")

	if conn.Spec.MCLAG != nil || conn.Spec.MCLAGDomain != nil {
		return admission.Warnings{"MCLAG is deprecated and no longer supported"}, nil
	}

	if fabricCfg != nil {
		if err := conn.Spec.ValidateServerFacingMTU(fabricCfg.FabricMTU, fabricCfg.ServerFacingMTUOffset); err != nil {
			return nil, err
		}
	}

	switches, servers, ports, _, err := conn.Spec.Endpoints()
	if err != nil {
		return nil, field.Invalid(spec, field.OmitValueType{}, err.Error())
	}

	if conn.Spec.StaticExternal != nil {
		se := conn.Spec.StaticExternal.Link.Switch
		sePath := spec.Child("staticExternal", "link", "switch")

		_, ipNet, err := net.ParseCIDR(se.IP)
		if err != nil {
			return nil, field.Invalid(sePath.Child("ip"), se.IP, err.Error())
		}

		nextHop := net.ParseIP(se.NextHop)
		if nextHop == nil {
			return nil, field.Invalid(sePath.Child("nextHop"), se.NextHop, "not a valid IP address")
		}

		if !ipNet.Contains(nextHop) {
			return nil, field.Invalid(sePath.Child("nextHop"), se.NextHop, "not in cidr "+ipNet.String())
		}

		subnets := []netip.Prefix{}
		for idx, subnet := range se.Subnets {
			subnetPath := sePath.Child("subnets").Index(idx)
			prefix, err := netip.ParsePrefix(subnet)
			if err != nil {
				return nil, field.Invalid(subnetPath, subnet, err.Error())
			}

			if subnet == "0.0.0.0/0" {
				if len(se.Subnets) > 1 {
					return nil, field.Invalid(subnetPath, subnet, "default route should be the only subnet")
				}

				break
			}

			if prefix.Addr() != prefix.Masked().Addr() {
				return nil, field.Invalid(subnetPath, subnet, "inconsistent IP address and mask")
			}

			subnets = append(subnets, prefix)
		}

		if err := iputil.VerifyNoOverlapNetip(subnets); err != nil {
			return nil, field.Invalid(sePath.Child("subnets"), field.OmitValueType{}, err.Error())
		}

		if fabricCfg != nil {
			for idx, subnet := range subnets {
				if reserved, overlaps := fabricCfg.OverlappingReservedSubnet(subnet); overlaps {
					return nil, field.Invalid(sePath.Child("subnets").Index(idx), se.Subnets[idx], "overlaps with reserved subnet "+reserved.String())
				}
			}
		}
	}

	if conn.Spec.ESLAG != nil && fabricCfg != nil && fabricCfg.FabricMode != meta.FabricModeSpineLeaf {
		return nil, field.Forbidden(spec.Child("eslag"), "eslag connection is not allowed in current fabric configuration")
	}

	if conn.Spec.Gateway != nil && fabricCfg != nil && fabricCfg.FabricMode != meta.FabricModeSpineLeaf {
		return nil, field.Forbidden(spec.Child("gateway"), "gateway connection is not allowed in current fabric configuration")
	}

	if _, err := CheckFabricExists(ctx, kube, conn.Namespace, conn.Spec.Topology.Fabric); err != nil {
		return nil, err
	}

	if kube != nil {
		rGroup := ""
		rType := meta.RedundancyTypeNone
		connFabric := conn.Spec.Topology.Fabric

		switchObjs := map[string]*Switch{}
		for _, switchName := range switches {
			sw := &Switch{}
			err := kube.Get(ctx, ktypes.NamespacedName{Name: switchName, Namespace: conn.Namespace}, sw) // TODO namespace could be different?
			if kapierrors.IsNotFound(err) {
				return nil, field.Invalid(spec, field.OmitValueType{}, fmt.Sprintf("switch %s not found", switchName))
			}
			if err != nil {
				return nil, fmt.Errorf("failed to get switch %s: %w", switchName, err) // TODO replace with some internal error to not expose to the user
			}

			if swFabric := sw.Spec.Topology.Fabric; swFabric != connFabric {
				return nil, field.Invalid(spec, field.OmitValueType{}, fmt.Sprintf("connection is in fabric %s but switch %s is in fabric %s", connFabric, switchName, swFabric))
			}
			switchObjs[switchName] = sw

			if conn.Spec.ESLAG != nil {
				if sw.Spec.Redundancy.Group != "" {
					if rGroup != "" && rGroup != sw.Spec.Redundancy.Group {
						return nil, field.Invalid(spec, field.OmitValueType{}, fmt.Sprintf("all switches in ESLAG connection should belong to the same redundancy group, found %s in %s", switchName, rGroup))
					}
					rGroup = sw.Spec.Redundancy.Group
				}
				if sw.Spec.Redundancy.Type != "" {
					if rType != "" && rType != sw.Spec.Redundancy.Type {
						return nil, field.Invalid(spec, field.OmitValueType{}, fmt.Sprintf("all switches in ESLAG connection should belong to the same redundancy type, found %s in %s", switchName, rType))
					}
					rType = sw.Spec.Redundancy.Type
				}
			}

			sp := &SwitchProfile{}
			err = kube.Get(ctx, ktypes.NamespacedName{Name: sw.Spec.Profile, Namespace: conn.Namespace}, sp) // TODO namespace could be different?
			if kapierrors.IsNotFound(err) {
				return nil, field.Invalid(spec, field.OmitValueType{}, fmt.Sprintf("switch profile %s not found", sw.Spec.Profile))
			}
			if err != nil {
				return nil, fmt.Errorf("failed to get switch profile %s: %w", sw.Spec.Profile, err) // TODO replace with some internal error to not expose to the user
			}

			allowedPorts, err := sp.Spec.GetAPI2NOSPortsFor(&sw.Spec)
			if err != nil {
				return nil, fmt.Errorf("failed to get NOS port mapping for switch %s: %w", switchName, err)
			}

			for _, port := range ports {
				if !strings.HasPrefix(port, switchName+"/") {
					continue
				}

				portName := strings.TrimPrefix(port, switchName+"/")

				if _, ok := allowedPorts[portName]; !ok {
					return nil, field.Invalid(spec, field.OmitValueType{}, fmt.Sprintf("port %s is not allowed for switch %s", port, switchName))
				}
			}
		}

		if err := conn.Spec.ValidateDomains(switchObjs); err != nil {
			return nil, field.Invalid(spec, field.OmitValueType{}, err.Error())
		}

		if conn.Spec.ESLAG != nil {
			if rGroup == "" {
				return nil, field.Invalid(spec, field.OmitValueType{}, "all switches in ESLAG connection should have redundancy group")
			}
			if rType != meta.RedundancyTypeESLAG {
				return nil, field.Invalid(spec, field.OmitValueType{}, fmt.Sprintf("all switches in ESLAG connection should have ESLAG redundancy type, found %s", rType))
			}
		}

		for _, serverName := range servers {
			err := kube.Get(ctx, ktypes.NamespacedName{Name: serverName, Namespace: conn.Namespace}, &Server{}) // TODO namespace could be different?
			if kapierrors.IsNotFound(err) {
				return nil, field.Invalid(spec, field.OmitValueType{}, fmt.Sprintf("server %s not found", serverName))
			}
			if err != nil {
				return nil, fmt.Errorf("failed to get server %s: %w", serverName, err) // TODO replace with some internal error to not expose to the user
			}
		}

		// track ports used including breakouts
		// recall that leaf-01/E1/4 is an alias for leaf-01/E1/4/1 is E1/4 is configured for breakout!
		connPorts := map[string]bool{}
		for _, port := range ports {
			connPorts[port] = true
			// if this is not an obvious breakout port, consider the case it is an alias
			// if it is not, it won't collide with anything anyway
			if strings.Count(port, "/") == 2 {
				connPorts[port+"/1"] = true
			}
		}

		// a port can only be used by another connection on the same device, and a gateway port only by another
		// gateway connection, which are all listed for the fabric IPs below
		others := map[string]*Connection{}
		listOthers := func(labels kclient.MatchingLabels) error {
			conns := &ConnectionList{}
			if err := kube.List(ctx, conns, kclient.InNamespace(conn.Namespace), labels); err != nil { // TODO namespace could be different?
				return fmt.Errorf("failed to list connections: %w", err)
			}
			for idx := range conns.Items {
				others[conns.Items[idx].Name] = &conns.Items[idx]
			}

			return nil
		}
		for _, switchName := range switches {
			if err := listOthers(MatchingLabelsForListLabelSwitch(switchName)); err != nil {
				return nil, err
			}
		}
		for _, serverName := range servers {
			if err := listOthers(MatchingLabelsForListLabelServer(serverName)); err != nil {
				return nil, err
			}
		}
		// fabric IPs are only set on, and checked for, these connection types
		if conn.Spec.Fabric != nil || conn.Spec.Mesh != nil || conn.Spec.Gateway != nil {
			for _, connType := range []string{ConnectionTypeFabric, ConnectionTypeMesh, ConnectionTypeGateway} {
				if err := listOthers(kclient.MatchingLabels{LabelConnectionType: connType}); err != nil {
					return nil, err
				}
			}
		}

		fabricSubnet, err := netip.ParsePrefix(fabricCfg.FabricSubnet)
		if err != nil {
			return nil, fmt.Errorf("failed to parse fabric subnet %s: %w", fabricCfg.FabricSubnet, err)
		}
		fabricIPs := map[netip.Addr]bool{}
		for _, name := range slices.Sorted(maps.Keys(others)) {
			other := others[name]
			if other.Name == conn.Name {
				continue
			}
			// skip deprecated MCLAG links
			if other.Spec.MCLAG != nil || other.Spec.MCLAGDomain != nil {
				continue
			}

			_, _, ports, _, err := other.Spec.Endpoints()
			if err != nil {
				return nil, fmt.Errorf("failed to get endpoints for connection %s: %w", other.Name, err)
			}

			for _, port := range ports {
				_, alreadyInUse := connPorts[port]
				if strings.Count(port, "/") == 2 {
					alreadyInUse = alreadyInUse || connPorts[port+"/1"]
				}
				if alreadyInUse {
					return nil, field.Invalid(spec, field.OmitValueType{}, fmt.Sprintf("port %s is already used by other connection %s", port, other.Name))
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
				linkPath := spec.Child("fabric", "links").Index(idx)
				spineIPPath, leafIPPath := linkPath.Child("spine", "ip"), linkPath.Child("leaf", "ip")

				// no IPs on either side means BGP unnumbered, but a link with just one IP is broken
				if (link.Spine.IP == "") != (link.Leaf.IP == "") {
					return nil, field.Invalid(linkPath, field.OmitValueType{}, "has an IP on only one side, it must be set on both for a numbered link or on neither for an unnumbered one")
				}
				if link.Spine.IP == "" {
					continue
				}

				spinePrefix, err := netip.ParsePrefix(link.Spine.IP)
				if err != nil {
					return nil, field.Invalid(spineIPPath, link.Spine.IP, err.Error())
				}
				if spinePrefix.Bits() != 31 {
					return nil, field.Invalid(spineIPPath, link.Spine.IP, "not a /31")
				}

				spineIP := spinePrefix.Addr()
				if !fabricSubnet.Contains(spineIP) {
					return nil, field.Invalid(spineIPPath, link.Spine.IP, "not in the fabric subnet "+fabricSubnet.String())
				}
				if _, exist := fabricIPs[spineIP]; exist {
					return nil, field.Invalid(spineIPPath, link.Spine.IP, "already in use")
				}
				fabricIPs[spineIP] = true

				leafPrefix, err := netip.ParsePrefix(link.Leaf.IP)
				if err != nil {
					return nil, field.Invalid(leafIPPath, link.Leaf.IP, err.Error())
				}
				if leafPrefix.Bits() != 31 {
					return nil, field.Invalid(leafIPPath, link.Leaf.IP, "not a /31")
				}

				leafIP := leafPrefix.Addr()
				if !fabricSubnet.Contains(leafIP) {
					return nil, field.Invalid(leafIPPath, link.Leaf.IP, "not in the fabric subnet "+fabricSubnet.String())
				}
				if _, exist := fabricIPs[leafIP]; exist {
					return nil, field.Invalid(leafIPPath, link.Leaf.IP, "already in use")
				}
				fabricIPs[leafIP] = true

				if spinePrefix.Masked() != leafPrefix.Masked() {
					return nil, field.Invalid(linkPath, field.OmitValueType{}, fmt.Sprintf("spine IP %s and leaf IP %s are not in the same subnet", spineIP, leafIP))
				}
			}
		} else if conn.Spec.Mesh != nil {
			cm := conn.Spec.Mesh
			for idx, link := range cm.Links {
				linkPath := spec.Child("mesh", "links").Index(idx)
				leaf1IPPath, leaf2IPPath := linkPath.Child("leaf1", "ip"), linkPath.Child("leaf2", "ip")

				// no IPs on either side means BGP unnumbered, but a link with just one IP is broken
				if (link.Leaf1.IP == "") != (link.Leaf2.IP == "") {
					return nil, field.Invalid(linkPath, field.OmitValueType{}, "has an IP on only one side, it must be set on both for a numbered link or on neither for an unnumbered one")
				}
				if link.Leaf1.IP == "" {
					continue
				}

				leaf1Prefix, err := netip.ParsePrefix(link.Leaf1.IP)
				if err != nil {
					return nil, field.Invalid(leaf1IPPath, link.Leaf1.IP, err.Error())
				}
				if leaf1Prefix.Bits() != 31 {
					return nil, field.Invalid(leaf1IPPath, link.Leaf1.IP, "not a /31")
				}

				leaf1IP := leaf1Prefix.Addr()
				if !fabricSubnet.Contains(leaf1IP) {
					return nil, field.Invalid(leaf1IPPath, link.Leaf1.IP, "not in the fabric subnet "+fabricSubnet.String())
				}
				if _, exist := fabricIPs[leaf1IP]; exist {
					return nil, field.Invalid(leaf1IPPath, link.Leaf1.IP, "already in use")
				}
				fabricIPs[leaf1IP] = true

				leaf2Prefix, err := netip.ParsePrefix(link.Leaf2.IP)
				if err != nil {
					return nil, field.Invalid(leaf2IPPath, link.Leaf2.IP, err.Error())
				}
				if leaf2Prefix.Bits() != 31 {
					return nil, field.Invalid(leaf2IPPath, link.Leaf2.IP, "not a /31")
				}

				leaf2IP := leaf2Prefix.Addr()
				if !fabricSubnet.Contains(leaf2IP) {
					return nil, field.Invalid(leaf2IPPath, link.Leaf2.IP, "not in the fabric subnet "+fabricSubnet.String())
				}
				if _, exist := fabricIPs[leaf2IP]; exist {
					return nil, field.Invalid(leaf2IPPath, link.Leaf2.IP, "already in use")
				}
				fabricIPs[leaf2IP] = true

				if leaf1Prefix.Masked() != leaf2Prefix.Masked() {
					return nil, field.Invalid(linkPath, field.OmitValueType{}, fmt.Sprintf("leaf1 IP %s and leaf2 IP %s are not in the same subnet", leaf1IP, leaf2IP))
				}
			}
		} else if conn.Spec.Gateway != nil {
			cg := conn.Spec.Gateway
			for idx, link := range cg.Links {
				linkPath := spec.Child("gateway", "links").Index(idx)
				switchIPPath, gatewayIPPath := linkPath.Child("switch", "ip"), linkPath.Child("gateway", "ip")

				// unlike fabric and mesh links, gateway links can't be unnumbered yet, so both IPs are required
				if link.Switch.IP == "" || link.Gateway.IP == "" {
					return nil, field.Invalid(linkPath, field.OmitValueType{}, "is missing an IP, it must be set on both sides as unnumbered gateway links are not supported yet")
				}

				switchPrefix, err := netip.ParsePrefix(link.Switch.IP)
				if err != nil {
					return nil, field.Invalid(switchIPPath, link.Switch.IP, err.Error())
				}
				if switchPrefix.Bits() != 31 {
					return nil, field.Invalid(switchIPPath, link.Switch.IP, "not a /31")
				}

				switchIP := switchPrefix.Addr()
				if !fabricSubnet.Contains(switchIP) {
					return nil, field.Invalid(switchIPPath, link.Switch.IP, "not in the fabric subnet "+fabricSubnet.String())
				}
				if _, exist := fabricIPs[switchIP]; exist {
					return nil, field.Invalid(switchIPPath, link.Switch.IP, "already in use")
				}
				fabricIPs[switchIP] = true

				gwPrefix, err := netip.ParsePrefix(link.Gateway.IP)
				if err != nil {
					return nil, field.Invalid(gatewayIPPath, link.Gateway.IP, err.Error())
				}
				if gwPrefix.Bits() != 31 {
					return nil, field.Invalid(gatewayIPPath, link.Gateway.IP, "not a /31")
				}

				gwIP := gwPrefix.Addr()
				if !fabricSubnet.Contains(gwIP) {
					return nil, field.Invalid(gatewayIPPath, link.Gateway.IP, "not in the fabric subnet "+fabricSubnet.String())
				}
				if _, exist := fabricIPs[gwIP]; exist {
					return nil, field.Invalid(gatewayIPPath, link.Gateway.IP, "already in use")
				}
				fabricIPs[gwIP] = true

				if switchPrefix.Masked() != gwPrefix.Masked() {
					return nil, field.Invalid(linkPath, field.OmitValueType{}, fmt.Sprintf("switch IP %s and gateway IP %s are not in the same subnet", switchIP, gwIP))
				}
			}
		}

		// TODO require it once fabricator sets it as well
		if fabricCfg.ExtraValidators.Connection != nil {
			if err := fabricCfg.ExtraValidators.Connection(ctx, kube, conn); err != nil {
				return nil, err //nolint:wrapcheck
			}
		}
	}

	return nil, nil
}
