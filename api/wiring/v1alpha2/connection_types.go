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
	CONNECTION_TYPE_UNBUNDLED   = "unbundled"
	CONNECTION_TYPE_MANAGEMENT  = "management"
	CONNECTION_TYPE_MCLAG       = "mclag"
	CONNECTION_TYPE_MCLAGDOMAIN = "mclag-domain"
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

type ConnMgmtLinkServer struct {
	BasePortName `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}/([1-2]?[0-9]|3[0-2])$`
	IP string `json:"ip,omitempty"`
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
}

type SwitchToSwitchLink struct {
	Switch1 BasePortName `json:"switch1,omitempty"`
	Switch2 BasePortName `json:"switch2,omitempty"`
}

type ConnMCLAGDomain struct {
	//+kubebuilder:validation:MinItems=1
	PeerLinks    []SwitchToSwitchLink `json:"peerLinks,omitempty"`
	SessionLinks []SwitchToSwitchLink `json:"sessionLinks,omitempty"`
}

// ConnectionSpec defines the desired state of Connection
type ConnectionSpec struct {
	Unbundled   *ConnUnbundled   `json:"unbundled,omitempty"`
	Management  *ConnMgmt        `json:"management,omitempty"`
	MCLAG       *ConnMCLAG       `json:"mclag,omitempty"`
	MCLAGDomain *ConnMCLAGDomain `json:"mclagDomain,omitempty"`
}

// ConnectionStatus defines the observed state of Connection
type ConnectionStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:categories=hedgehog;wiring

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
		}
		if c.MCLAG != nil {
			role = "mclag"
			left = c.MCLAG.Links[0].Server.DeviceName()
			for _, link := range c.MCLAG.Links {
				// check we have the same server in each link // TODO add validation
				if link.Server.DeviceName() != left {
					return "<invalid>" // TODO replace with error?
				}
				right = append(right, link.Switch.DeviceName())
			}
		}

		if left != "" && role != "" && len(right) > 0 {
			return fmt.Sprintf("%s--%s--%s", left, role, strings.Join(right, "--"))
		}
	}

	return "<invalid>" // TODO replace with error?
}

func (c *ConnectionSpec) ConnectionLabels() map[string]string {
	res := map[string]string{}

	if c.Unbundled != nil {
		res[LabelConnectionType] = CONNECTION_TYPE_UNBUNDLED
		res[ListLabel(ConnectionLabelTypeServer, c.Unbundled.Link.Server.DeviceName())] = ListLabelValue
		res[ListLabel(ConnectionLabelTypeSwitch, c.Unbundled.Link.Switch.DeviceName())] = ListLabelValue
	} else if c.Management != nil {
		res[LabelConnectionType] = CONNECTION_TYPE_MANAGEMENT
		res[ListLabel(ConnectionLabelTypeServer, c.Management.Link.Server.DeviceName())] = ListLabelValue
		res[ListLabel(ConnectionLabelTypeSwitch, c.Management.Link.Switch.DeviceName())] = ListLabelValue
	} else if c.MCLAGDomain != nil {
		res[LabelConnectionType] = CONNECTION_TYPE_MCLAGDOMAIN
		for _, link := range c.MCLAGDomain.PeerLinks {
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch1.DeviceName())] = ListLabelValue
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch2.DeviceName())] = ListLabelValue
		}
		for _, link := range c.MCLAGDomain.SessionLinks {
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch1.DeviceName())] = ListLabelValue
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch2.DeviceName())] = ListLabelValue
		}
	} else if c.MCLAG != nil {
		res[LabelConnectionType] = CONNECTION_TYPE_MCLAG
		for _, link := range c.MCLAG.Links {
			res[ListLabel(ConnectionLabelTypeServer, link.Server.DeviceName())] = ListLabelValue
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch.DeviceName())] = ListLabelValue
		}
	}

	return res
}

func (s *ConnectionSpec) Endpoints() ([]string, []string, []string, error) {
	switches := map[string]struct{}{}
	servers := map[string]struct{}{}
	ports := map[string]struct{}{}

	nonNills := 0
	if s.Unbundled != nil {
		nonNills++

		switches[s.Unbundled.Link.Switch.DeviceName()] = struct{}{}
		servers[s.Unbundled.Link.Server.DeviceName()] = struct{}{}
		ports[s.Unbundled.Link.Switch.PortName()] = struct{}{}
		ports[s.Unbundled.Link.Server.PortName()] = struct{}{}

		if len(switches) != 1 {
			return nil, nil, nil, errors.Errorf("one switch must be used for unbundled connection")
		}
		if len(servers) != 1 {
			return nil, nil, nil, errors.Errorf("one server must be used for unbundled connection")
		}
		if len(ports) != 2 {
			return nil, nil, nil, errors.Errorf("two unique ports must be used for unbundled connection")
		}
	} else if s.Management != nil {
		nonNills++

		switches[s.Management.Link.Switch.DeviceName()] = struct{}{}
		servers[s.Management.Link.Server.DeviceName()] = struct{}{}
		ports[s.Management.Link.Switch.PortName()] = struct{}{}
		ports[s.Management.Link.Server.PortName()] = struct{}{}

		if len(switches) != 1 {
			return nil, nil, nil, errors.Errorf("one switch must be used for management connection")
		}
		if len(servers) != 1 {
			return nil, nil, nil, errors.Errorf("one server must be used for management connection")
		}
		if len(ports) != 2 {
			return nil, nil, nil, errors.Errorf("two unique ports must be used for management connection")
		}
	} else if s.MCLAGDomain != nil {
		nonNills++

		for _, link := range s.MCLAGDomain.PeerLinks {
			switches[link.Switch1.DeviceName()] = struct{}{}
			switches[link.Switch2.DeviceName()] = struct{}{}
			ports[link.Switch1.PortName()] = struct{}{}
			ports[link.Switch2.PortName()] = struct{}{}
		}
		for _, link := range s.MCLAGDomain.SessionLinks {
			switches[link.Switch1.DeviceName()] = struct{}{}
			switches[link.Switch2.DeviceName()] = struct{}{}
			ports[link.Switch1.PortName()] = struct{}{}
			ports[link.Switch2.PortName()] = struct{}{}
		}

		if len(s.MCLAGDomain.PeerLinks) < 1 {
			return nil, nil, nil, errors.Errorf("at least one peer link must be used for mclag domain connection")
		}
		if len(s.MCLAGDomain.SessionLinks) < 1 {
			return nil, nil, nil, errors.Errorf("at least one session link must be used for mclag domain connection")
		}
		if len(switches) != 2 {
			return nil, nil, nil, errors.Errorf("two switches must be used for mclag domain domain connection")
		}
		if len(ports) != 2*(len(s.MCLAGDomain.PeerLinks)+len(s.MCLAGDomain.SessionLinks)) {
			return nil, nil, nil, errors.Errorf("unique ports must be used for mclag domain domain connection")
		}
	} else if s.MCLAG != nil {
		nonNills++

		for _, link := range s.MCLAG.Links {
			switches[link.Switch.DeviceName()] = struct{}{}
			servers[link.Server.DeviceName()] = struct{}{}
			ports[link.Switch.PortName()] = struct{}{}
			ports[link.Server.PortName()] = struct{}{}
		}

		if len(switches) != 2 {
			return nil, nil, nil, errors.Errorf("two switches must be used for mclag connection")
		}
		if len(servers) != 1 {
			return nil, nil, nil, errors.Errorf("one server must be used for mclag connection")
		}
		if len(ports) != 2*len(s.MCLAG.Links) {
			return nil, nil, nil, errors.Errorf("unique ports must be used for mclag connection")
		}
	}

	if nonNills != 1 {
		return nil, nil, nil, errors.Errorf("exactly one connection type must be used")
	}

	for port := range ports {
		parts := SplitPortName(port)

		// TODO evaluate not allowing more than one separator in port name
		// if len(parts) != 2 {
		// 	return nil, nil, nil, errors.Errorf("invalid port name %q", port)
		// }

		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil, nil, nil, errors.Errorf("invalid port name %q, should be \"<device>/<port>\" format", port)
		}
	}

	return maps.Keys(switches), maps.Keys(servers), maps.Keys(ports), nil
}

func (conn *Connection) Default() {
	if conn.Labels == nil {
		conn.Labels = map[string]string{}
	}

	maps.Copy(conn.Labels, conn.Spec.ConnectionLabels())
}

func (conn *Connection) Validate(ctx context.Context, client validation.Client) (admission.Warnings, error) {
	// TODO validate local port names against server/switch profiles
	// TODO validate used port names across all connections

	switches, servers, _, err := conn.Spec.Endpoints()
	if err != nil {
		return nil, err
	}

	if client != nil {
		for _, switchName := range switches {
			err := client.Get(ctx, types.NamespacedName{Name: switchName, Namespace: conn.Namespace}, &Switch{}) // TODO namespace could be different?
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("switch %q not found", switchName)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get switch %q", switchName) // TODO replace with some internal error to not expose to the user
			}
		}
		for _, serverName := range servers {
			err := client.Get(ctx, types.NamespacedName{Name: serverName, Namespace: conn.Namespace}, &Server{}) // TODO namespace could be different?
			if apierrors.IsNotFound(err) {
				return nil, errors.Errorf("server %q not found", serverName)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get server %q", serverName) // TODO replace with some internal error to not expose to the user
			}
		}
	}

	return nil, nil
}
