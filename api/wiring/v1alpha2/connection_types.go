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
	"sort"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"golang.org/x/exp/maps"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	CONNECTION_TYPE_UNBUNDLED    = "unbundled"
	CONNECTION_TYPE_BUNDLED      = "bundled"
	CONNECTION_TYPE_MANAGEMENT   = "management" // TODO rename to control?
	CONNECTION_TYPE_MCLAG        = "mclag"
	CONNECTION_TYPE_MCLAGDOMAIN  = "mclag-domain"
	CONNECTION_TYPE_NAT          = "nat"
	CONNECTION_TYPE_FABRIC       = "fabric"
	CONNECTION_TYPE_VPC_LOOPBACK = "vpc-loopback"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type BasePortName struct {
	Port string `json:"port,omitempty"`
}

type ServerToSwitchLink struct {
	Server BasePortName `json:"server,omitempty"`
	Switch BasePortName `json:"switch,omitempty"`
}

type ConnUnbundled struct {
	Link ServerToSwitchLink `json:"link,omitempty"`
}

type ConnBundled struct {
	Links []ServerToSwitchLink `json:"links,omitempty"`
}

type ConnMgmtLinkServer struct {
	BasePortName `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	IP  string `json:"ip,omitempty"`
	MAC string `json:"mac,omitempty"`
}

type ConnMgmtLinkSwitch struct {
	BasePortName `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	IP           string `json:"ip,omitempty"`
	ONIEPortName string `json:"oniePortName,omitempty"`
}

type ConnMgmtLink struct {
	Server ConnMgmtLinkServer `json:"server,omitempty"`
	Switch ConnMgmtLinkSwitch `json:"switch,omitempty"`
}

type ConnMgmt struct {
	Link ConnMgmtLink `json:"link,omitempty"`
}

type ConnMCLAG struct {
	//+kubebuilder:validation:MinItems=2
	Links []ServerToSwitchLink `json:"links,omitempty"`
	MTU   uint16               `json:"mtu,omitempty"`
}

type SwitchToSwitchLink struct {
	Switch1 BasePortName `json:"switch1,omitempty"`
	Switch2 BasePortName `json:"switch2,omitempty"`
}

type ConnMCLAGDomain struct {
	//+kubebuilder:validation:MinItems=1
	PeerLinks []SwitchToSwitchLink `json:"peerLinks,omitempty"`

	//+kubebuilder:validation:MinItems=1
	SessionLinks []SwitchToSwitchLink `json:"sessionLinks,omitempty"`
}

type ConnNATLinkSwitch struct {
	BasePortName `json:",inline"`
	IP           string `json:"ip,omitempty"`
	NeighborIP   string `json:"neighborIP,omitempty"`
	RemoteAS     uint32 `json:"remoteAS,omitempty"`
	SNAT         SNAT   `json:"snat,omitempty"`
}

type SNAT struct {
	Pool []string `json:"pool"`
}

type ConnNATLink struct {
	Switch ConnNATLinkSwitch `json:"switch,omitempty"`
	NAT    BasePortName      `json:"nat,omitempty"`
}

type ConnNAT struct {
	Link ConnNATLink `json:"link,omitempty"`
}

type ConnFabricLinkSwitch struct {
	BasePortName `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	IP string `json:"ip,omitempty"`
}

type FabricLink struct {
	Spine ConnFabricLinkSwitch `json:"spine,omitempty"`
	Leaf  ConnFabricLinkSwitch `json:"leaf,omitempty"`
}

type ConnFabric struct {
	//+kubebuilder:validation:MinItems=1
	Links []FabricLink `json:"links,omitempty"`
}

type ConnVPCLoopback struct {
	//+kubebuilder:validation:MinItems=1
	Links []SwitchToSwitchLink `json:"links,omitempty"`
}

// ConnectionSpec defines the desired state of Connection
type ConnectionSpec struct {
	Unbundled   *ConnUnbundled   `json:"unbundled,omitempty"`
	Bundled     *ConnBundled     `json:"bundled,omitempty"`
	Management  *ConnMgmt        `json:"management,omitempty"`
	MCLAG       *ConnMCLAG       `json:"mclag,omitempty"`
	MCLAGDomain *ConnMCLAGDomain `json:"mclagDomain,omitempty"`
	NAT         *ConnNAT         `json:"nat,omitempty"`
	Fabric      *ConnFabric      `json:"fabric,omitempty"`
	VPCLoopback *ConnVPCLoopback `json:"vpcLoopback,omitempty"`
}

// ConnectionStatus defines the observed state of Connection
type ConnectionStatus struct {
	// Applied ApplyStatus `json:"applied,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=conn
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.metadata.labels.fabric\.githedgehog\.com/connection-type`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// Connection is the Schema for the connections API
type Connection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConnectionSpec   `json:"spec,omitempty"`
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
		} else if c.NAT != nil {
			role = "nat"
			left = c.NAT.Link.Switch.DeviceName()
			right = []string{c.NAT.Link.NAT.DeviceName()}
		} else if c.Fabric != nil {
			role = "fabric"
			left = c.Fabric.Links[0].Spine.DeviceName()
			right = []string{c.Fabric.Links[0].Leaf.DeviceName()}
		} else if c.VPCLoopback != nil {
			role = "vpc-loopback"
			left = c.VPCLoopback.Links[0].Switch1.DeviceName()
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

	if c.Unbundled != nil {
		labels[LabelConnectionType] = CONNECTION_TYPE_UNBUNDLED
	} else if c.Bundled != nil {
		labels[LabelConnectionType] = CONNECTION_TYPE_BUNDLED
	} else if c.Management != nil {
		labels[LabelConnectionType] = CONNECTION_TYPE_MANAGEMENT
	} else if c.MCLAGDomain != nil {
		labels[LabelConnectionType] = CONNECTION_TYPE_MCLAGDOMAIN
	} else if c.MCLAG != nil {
		labels[LabelConnectionType] = CONNECTION_TYPE_MCLAG
	} else if c.NAT != nil {
		labels[LabelConnectionType] = CONNECTION_TYPE_NAT
	} else if c.Fabric != nil {
		labels[LabelConnectionType] = CONNECTION_TYPE_FABRIC
	} else if c.VPCLoopback != nil {
		labels[LabelConnectionType] = CONNECTION_TYPE_VPC_LOOPBACK
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
	} else if s.NAT != nil {
		nonNills++

		switches[s.NAT.Link.Switch.DeviceName()] = struct{}{}
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
	if conn.Labels == nil {
		conn.Labels = map[string]string{}
	}

	CleanupFabricLabels(conn.Labels)

	maps.Copy(conn.Labels, conn.Spec.ConnectionLabels())
}

func (conn *Connection) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	// TODO validate local port names against server/switch profiles
	// TODO validate used port names across all connections

	switches, servers, _, _, err := conn.Spec.Endpoints()
	if err != nil {
		return nil, err
	}

	if client != nil {
		for _, switchName := range switches {
			err := client.Get(ctx, types.NamespacedName{Name: switchName, Namespace: conn.Namespace}, &Switch{}) // TODO namespace could be different?
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("switch %s not found", switchName)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get switch %s", switchName) // TODO replace with some internal error to not expose to the user
			}
		}
		for _, serverName := range servers {
			err := client.Get(ctx, types.NamespacedName{Name: serverName, Namespace: conn.Namespace}, &Server{}) // TODO namespace could be different?
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("server %s not found", serverName)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get server %s", serverName) // TODO replace with some internal error to not expose to the user
			}
		}
	}

	// TODO validate that snat pool is in the nat subnet and unique per conn type=nat

	return nil, nil
}
