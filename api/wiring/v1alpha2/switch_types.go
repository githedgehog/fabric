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
	"net/url"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TODO :
// This is where we should define our route filter criteria for outgoing and incoming prefixes. We should plan to support a small subset
// of filtering criteria we want to use support in the fabric.
// At small route scales these features are not critical but might become important later
type FilterInfo string

// +kubebuilder:validation:Enum=leaf;service-leaf;border-leaf;spine
type SwitchRole string

const (
	SwitchRoleLeaf        SwitchRole = "leaf"
	SwitchRoleServiceLeaf SwitchRole = "service-leaf"
	SwitchRoleBorderLeaf  SwitchRole = "border-leaf"
	SwitchRoleSpine       SwitchRole = "spine"
)

// SwitchLocation defines the geopraphical position of the switch in a datacenter
type SwitchLocation struct {
	Location string `json:"location,omitempty"`
	Aisle    string `json:"aisle,omitempty"`
	Row      string `json:"row,omitempty"`
	Rack     string `json:"rack,omitempty"`
	Slot     string `json:"slot,omitempty"`
}

// SwitchLocationSig contains signatures for the location UUID as well as the Switch location itself
type SwitchLocationSig struct {
	Sig     string `json:"sig,omitempty"`
	UUIDSig string `json:"uuidSig,omitempty"`
}

type LLDPConfig struct {
	HelloTimer        time.Duration `json:"helloTimer,omitempty"`
	ManagementIP      string        `json:"managementIP,omitempty"`
	SystemDescription string        `json:"systemDescription,omitempty"`
	SystemName        string        `json:"systemName,omitempty"`
}

type BorderConfig struct {
	VRF              string `json:"vrf,omitempty"`
	DefaultRoute     string `json:"defaultRoute,omitempty"`
	ExportSummarized string `json:"exportSummarized,omitempty"`
}

type AddressFamily struct {
	Family       string   `json:"family,omitempty"`
	ImportTarget []string `json:"importTarget,omitempty"`
	ExportTarget []string `json:"exportTarget,omitempty"`
}

type BGPNeighborInfo struct {
	ID         string     `json:"id,omitempty"`
	ASN        int        `json:"asn,omitempty"`
	Filterinfo FilterInfo `json:"filterInfo,omitempty"`
}
type BGPRouterConfig struct {
	ASN           int               `json:"asn,omitempty"`
	VRF           string            `json:"vrf,omitempty"`
	RouterID      string            `json:"routerID,omitempty"`
	NeighborInfo  []BGPNeighborInfo `json:"neighborInfo,omitempty"`
	AddressFamily AddressFamily     `json:"addressFamily,omitempty"`
}

type BGPConfig struct {
	LoopbackInterfaceNum uint32            `json:"loopbackInterfaceNum"`
	LoopbackAddress      string            `json:"loopbackAddress,omitempty"`
	BGPRouterConfig      []BGPRouterConfig `json:"bgpRouterConfig,omitempty"`
	BorderConfig         BorderConfig      `json:"borderConfig,omitempty"`
}

type VlanInfo struct {
	VlanID               uint16 `json:"vlanID,omitempty"`
	VlanInterfaceEnabled bool   `json:"vlanInterfaceEnabled,omitempty"`
	TaggedVlan           bool   `json:"taggedVlan,omitempty"`
}

// SwitchSpec defines the desired state of Switch
type SwitchSpec struct {
	SecureBootCapable         bool              `json:"secureBootCapable,omitempty"`
	RemoteAttestationRequired bool              `json:"remoteAttestationRequired,omitempty"`
	Location                  SwitchLocation    `json:"location,omitempty"`
	LocationUUID              string            `json:"locationUUID,omitempty"`
	LocationSig               SwitchLocationSig `json:"locationSig,omitempty"`
	ConnectedPorts            uint32            `json:"connectedPorts,omitempty"`
	MaxPorts                  uint32            `json:"maxPorts,omitempty"`
	ServerFacingPorts         int               `json:"serverFacingPorts,omitempty"`
	FabricFacingPorts         int               `json:"fabricFacingPorts,omitempty"`
	Role                      SwitchRole        `json:"role,omitempty"`
	BGPConfig                 []BGPConfig       `json:"bgpConfig,omitempty"`
	LLDPConfig                LLDPConfig        `json:"lldpConfig,omitempty"`
	VendorName                string            `json:"vendorName,omitempty"`
	ModelNumber               string            `json:"modelNumber,omitempty"`
	SONiCVersion              string            `json:"sonicVersion,omitempty"`
	Vlan                      []VlanInfo        `json:"vlan,omitempty"`
	Vrfs                      []string          `json:"vrfs,omitempty"`
}

// SwitchStatus defines the observed state of Switch
type SwitchStatus struct {
	// TODO: add port status fields
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Switch is the Schema for the switches API
//
// All switches should always have 1 labels defined: wiring.githedgehog.com/rack. It represents names of the rack it
// belongs to.
type Switch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SwitchSpec   `json:"spec,omitempty"`
	Status SwitchStatus `json:"status,omitempty"`
}

const KindSwitch = "Switch"

//+kubebuilder:object:root=true

// SwitchList contains a list of Switch
type SwitchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Switch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Switch{}, &SwitchList{})
}

// GenerateUUID generates the location UUID which is a version 5 UUID over the fields of `SwitchLocation`.
// It also returns the URL representation that was used in order to generate the UUID.
func (l *SwitchLocation) GenerateUUID() (string, string) {
	// we use the location field for the "opaque" part
	location := "location"
	if l.Location != "" {
		location = url.QueryEscape(l.Location)
	}

	// and we build URL query components for the rest
	q := ""
	addAmpersand := func() {
		if q != "" {
			q += "&"
		}
	}
	if l.Aisle != "" {
		addAmpersand()
		q += "aisle=" + url.QueryEscape(l.Aisle)
	}
	if l.Row != "" {
		addAmpersand()
		q += "row=" + url.QueryEscape(l.Row)
	}
	if l.Rack != "" {
		addAmpersand()
		q += "rack=" + url.QueryEscape(l.Rack)
	}
	if l.Slot != "" {
		addAmpersand()
		q += "slot=" + url.QueryEscape(l.Slot)
	}

	// return nothing if nothing was set
	if location == "location" && q == "" {
		return "", ""
	}

	// now we build a URL
	u := &url.URL{
		Scheme:   "hhloc",
		Opaque:   location,
		RawQuery: q,
	}

	// and return a version 5 UUID based on the URL namespace with it
	us := u.String()
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(us)).String(), us
}
