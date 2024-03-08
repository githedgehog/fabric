package v1alpha2

const (
	VIRTUAL_EDGE_ANNOTATION = "virtual-edge.hhfab.fabric.githedgehog.com/external-cfg"
)

// +kubebuilder:skip
type VirtualEdgeConfig struct {
	ASN          string `json:"ASN"`
	VRF          string `json:"VRF"`
	CommunityIn  string `json:"CommunityIn"`
	CommunityOut string `json:"CommunityOut"`
	NeighborIP   string `json:"NeighborIP"`
	IfName       string `json:"ifName"`
	IfVlan       string `json:"ifVlan"`
	IfIP         string `json:"ifIP"`
}
