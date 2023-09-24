package v1alpha2

var (
	LabelPrefix    = "fabric.githedgehog.com/"
	LabelVPC       = LabelName("vpc")
	LabelVPCSubnet = LabelName("vpc") + "/subnet"
)

func LabelName(name string) string {
	return LabelPrefix + name
}
