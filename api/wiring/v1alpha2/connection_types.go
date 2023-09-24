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
	"fmt"
	"strings"

	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (pn *BasePortName) LocalPortName() string {
	return strings.SplitN(pn.Port, PORT_NAME_SEPARATOR, 2)[1] // TODO ensure objects are validated first
}

func (pn *BasePortName) DeviceName() string {
	return strings.SplitN(pn.Port, PORT_NAME_SEPARATOR, 2)[0] // TODO ensure objects are validated first
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
		res[ListLabel(ConnectionLabelTypeServer, c.Unbundled.Link.Server.DeviceName())] = ListLabelValue
		res[ListLabel(ConnectionLabelTypeSwitch, c.Unbundled.Link.Switch.DeviceName())] = ListLabelValue
	} else if c.Management != nil {
		res[ListLabel(ConnectionLabelTypeServer, c.Management.Link.Server.DeviceName())] = ListLabelValue
		res[ListLabel(ConnectionLabelTypeSwitch, c.Management.Link.Switch.DeviceName())] = ListLabelValue
	} else if c.MCLAGDomain != nil {
		for _, link := range c.MCLAGDomain.PeerLinks {
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch1.DeviceName())] = ListLabelValue
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch2.DeviceName())] = ListLabelValue
		}
		for _, link := range c.MCLAGDomain.SessionLinks {
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch1.DeviceName())] = ListLabelValue
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch2.DeviceName())] = ListLabelValue
		}
	}
	if c.MCLAG != nil {
		for _, link := range c.MCLAG.Links {
			res[ListLabel(ConnectionLabelTypeServer, link.Server.DeviceName())] = ListLabelValue
			res[ListLabel(ConnectionLabelTypeSwitch, link.Switch.DeviceName())] = ListLabelValue
		}
	}

	return res
}

func (c *Connection) GenerateLabels() {
	if c.Labels == nil {
		c.Labels = map[string]string{}
	}

	maps.Copy(c.Labels, c.Spec.ConnectionLabels())
}

func (c *Connection) Validate() {
}
