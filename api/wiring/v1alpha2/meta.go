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

package v1alpha2

import (
	"net/url"
	"strings"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PortNameSeparator    = "/"
	DefaultVLANNamespace = "default"
)

var (
	// TODO should it be same as group name? or just standard prefix for all APIs?
	LabelPrefix               = "fabric.githedgehog.com/"
	LabelRack                 = LabelName("rack")
	LabelSwitch               = LabelName("switch")
	LabelServer               = LabelName("server")
	LabelServerType           = LabelName("server-type")
	LabelLocation             = LabelName("location")
	LabelConnection           = LabelName("connection")
	LabelConnectionType       = LabelName("connection-type")
	LabelSwitches             = LabelName("switches")
	LabelServers              = LabelName("servers")
	LabelGroups               = LabelName("groups")
	LabelVPC                  = LabelName("vpc")
	ListLabelValue            = "true"
	ConnectionLabelTypeServer = "server"
	ConnectionLabelTypeSwitch = "switch"
	ConnectionLabelTypeRack   = "rack"
)

func LabelName(name string) string {
	return LabelPrefix + name
}

func ListLabelPrefix(listType string) string {
	return listType + "." + LabelPrefix
}

func ListLabel(listType, val string) string {
	return ListLabelPrefix(listType) + val
}

func ListLabelServer(serverName string) string {
	return ListLabel(ConnectionLabelTypeServer, serverName)
}

func ListLabelSwitch(switchName string) string {
	return ListLabel(ConnectionLabelTypeSwitch, switchName)
}

func ListLabelRack(rackName string) string {
	return ListLabel(ConnectionLabelTypeRack, rackName)
}

func ListLabelVLANNamespace(vlanNamespace string) string {
	return ListLabel("vlanns", vlanNamespace)
}

func MatchingLabelsForListLabelServer(serverName string) client.MatchingLabels {
	return client.MatchingLabels{
		ListLabel(ConnectionLabelTypeServer, serverName): ListLabelValue,
	}
}

func MatchingLabelsForListLabelSwitch(switchName string) client.MatchingLabels {
	return client.MatchingLabels{
		ListLabel(ConnectionLabelTypeSwitch, switchName): ListLabelValue,
	}
}

// Location defines the geographical position of the device in a datacenter
type Location struct {
	Location string `json:"location,omitempty"`
	Aisle    string `json:"aisle,omitempty"`
	Row      string `json:"row,omitempty"`
	Rack     string `json:"rack,omitempty"`
	Slot     string `json:"slot,omitempty"`
}

// LocationSig contains signatures for the location UUID as well as the device location itself
type LocationSig struct {
	Sig     string `json:"sig,omitempty"`
	UUIDSig string `json:"uuidSig,omitempty"`
}

type ApplyStatus struct {
	Generation int64            `json:"gen,omitempty"`
	Time       metav1.Time      `json:"time,omitempty"`
	Detailed   map[string]int64 `json:"detailed,omitempty"`
}

// GenerateUUID generates the location UUID which is a version 5 UUID over the fields of `Location`.
// It also returns the URL representation that was used in order to generate the UUID.
func (l *Location) GenerateUUID() (string, string) {
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

func (l *Location) IsEmpty() bool {
	return l.Location == "" && l.Aisle == "" && l.Row == "" && l.Rack == "" && l.Slot == ""
}

func CleanupFabricLabels(labels map[string]string) {
	for key := range labels {
		if strings.Contains(key, LabelPrefix) {
			delete(labels, key)
		}
	}
}
