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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ConnLinkPort struct {
	Name string `json:"name,omitempty"`
}

type ConnLinkPart struct {
	SwitchPort *ConnLinkPort `json:"switchPort,omitempty"`
	ServerPort *ConnLinkPort `json:"serverPort,omitempty"`
}

// +kubebuilder:validation:MaxItems=2
// +kubebuilder:validation:MinItems=2
type ConnLink []ConnLinkPart

type UnbundledConn struct {
	Link ConnLink `json:"link,omitempty"`
}

type ManagementConnSwitchPort struct {
	ConnLinkPort `json:",inline"`
	//+kubebuilder:validation:Pattern=`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}$`
	IP string `json:"ip,omitempty"`
}

type ManagementConnLinkPart struct {
	SwitchPort *ManagementConnSwitchPort `json:"switchPort,omitempty"`
	ServerPort *ConnLinkPort             `json:"serverPort,omitempty"`
}

// +kubebuilder:validation:MaxItems=2
// +kubebuilder:validation:MinItems=2
type ManagementConnLink []ManagementConnLinkPart

type ManagementConn struct {
	Link ManagementConnLink `json:"link,omitempty"`
}

type MCLAGConn struct {
	Links []ConnLink `json:"links,omitempty"`
}

type MCLAGDomainConn struct {
	Links []ConnLink `json:"links,omitempty"`
}

// ConnectionSpec defines the desired state of Connection
type ConnectionSpec struct {
	Unbundled   *UnbundledConn   `json:"unbundled,omitempty"`
	Management  *ManagementConn  `json:"management,omitempty"`
	MCLAG       *MCLAGConn       `json:"mclag,omitempty"`
	MCLAGDomain *MCLAGDomainConn `json:"mclagDomain,omitempty"`
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

func (c *ConnectionSpec) GenerateName() string {
	if c != nil {
		if c.Unbundled != nil {
			left := c.Unbundled.Link[0].DeviceName() // TODO make sure server is listed first
			right := c.Unbundled.Link[1].DeviceName()

			return fmt.Sprintf("%s--unbundled--%s", left, right)
		}
		if c.Management != nil {
			control := c.Management.Link[0].ServerPort.DeviceName() // TODO make sure control is listed first
			sw := c.Management.Link[1].SwitchPort.DeviceName()

			return fmt.Sprintf("%s--mgmt--%s", control, sw)
		}
		if c.MCLAGDomain != nil {
			switch1 := c.MCLAGDomain.Links[0][0].DeviceName()
			switch2 := c.MCLAGDomain.Links[0][1].DeviceName()

			return fmt.Sprintf("%s--mclag-domain--%s", switch1, switch2)
		}
		if c.MCLAG != nil {
			server := c.MCLAG.Links[0][0].DeviceName() // TODO make sure server is listed first
			switch1 := c.MCLAG.Links[0][1].DeviceName()
			switch2 := c.MCLAG.Links[1][1].DeviceName() // TODO iterate over all links

			return fmt.Sprintf("%s--mclag--%s--%s", server, switch1, switch2)
		}
	}

	return "<invalid>" // TODO replace with error?
}

func (l *ConnLinkPort) DeviceName() string {
	if l != nil {
		return strings.SplitN(l.Name, "/", 2)[0] // TODO check result, extract sepatator to const
	}

	return "<invalid>" // TODO replace with error?
}

func (l *ConnLinkPart) DeviceName() string {
	if l != nil {
		if l.SwitchPort != nil {
			return l.SwitchPort.DeviceName()
		}
		if l.ServerPort != nil {
			return l.ServerPort.DeviceName()
		}
	}

	return "<invalid>" // TODO replace with error?
}

func (l *ConnLinkPort) PortName() string {
	if l != nil {
		return l.Name
	}

	return "<invalid>" // TODO replace with error?
}

func (l *ConnLinkPart) PortName() string {
	if l != nil {
		if l.SwitchPort != nil {
			return l.SwitchPort.PortName()
		}
		if l.ServerPort != nil {
			return l.ServerPort.PortName()
		}
	}

	return "<invalid>" // TODO replace with error?
}

func (c *ConnectionSpec) PortNames() [][2]string {
	if c != nil {
		if c.Unbundled != nil {
			left := c.Unbundled.Link[0].PortName() // TODO make sure server is listed first
			right := c.Unbundled.Link[1].PortName()

			return [][2]string{{left, right}}
		}
		if c.Management != nil {
			control := c.Management.Link[0].ServerPort.PortName() // TODO make sure control is listed first
			sw := c.Management.Link[1].SwitchPort.PortName()

			return [][2]string{{control, sw}}
		}
		if c.MCLAGDomain != nil {
			switch1 := c.MCLAGDomain.Links[0][0].PortName()
			switch2 := c.MCLAGDomain.Links[0][1].PortName()

			return [][2]string{{switch1, switch2}}
		}
		if c.MCLAG != nil {
			server := c.MCLAG.Links[0][0].PortName() // TODO make sure server is listed first
			switch1 := c.MCLAG.Links[0][1].PortName()
			switch2 := c.MCLAG.Links[1][1].PortName() // TODO iterate over all links

			return [][2]string{{server, switch1}, {server, switch2}}
		}
	}

	return [][2]string{} // TODO replace with error?
}
