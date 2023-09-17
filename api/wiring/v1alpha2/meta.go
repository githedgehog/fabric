package v1alpha2

import (
	"net/url"

	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// TODO should it be same as group name? or just standard prefix for all APIs?
	LabelPrefix               = "fabric.githedgehog.com/"
	LabelRack                 = LabelName("rack")
	LabelSwitch               = LabelName("switch")
	LabelServer               = LabelName("server")
	LabelLocation             = LabelName("location")
	ConnectionLabelValue      = "true"
	ConnectionLabelTypeServer = "server"
	ConnectionLabelTypeSwitch = "switch"
	ConnectionLabelTypeRack   = "rack"
)

func LabelName(name string) string {
	return LabelPrefix + name
}

func ConnectionLabelPrefix(deviceType string) string {
	return deviceType + ".connection." + LabelPrefix
}

func ConnectionLabel(deviceType, deviceName string) string {
	return ConnectionLabelPrefix(deviceType) + deviceName
}

func MatchingLabelsForServerConnections(serverName string) client.MatchingLabels {
	return client.MatchingLabels{
		ConnectionLabel(ConnectionLabelTypeServer, serverName): ConnectionLabelValue,
	}
}

func MatchingLabelsForSwitchConnections(switchName string) client.MatchingLabels {
	return client.MatchingLabels{
		ConnectionLabel(ConnectionLabelTypeSwitch, switchName): ConnectionLabelValue,
	}
}

// Location defines the geopraphical position of the device in a datacenter
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
