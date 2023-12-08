package v1alpha2

var (
	LabelPrefix    = "fabric.githedgehog.com/"
	LabelVPC       = LabelName("vpc")
	LabelVPC1      = LabelName("vpc1")
	LabelVPC2      = LabelName("vpc2")
	LabelSubnet    = LabelName("subnet")
	LabelIPv4NS    = LabelName("ipv4ns")
	LabelVLANNS    = LabelName("vlanns")
	LabelExternal  = LabelName("external")
	ListLabelValue = "true"
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

func ListLabelVPC(vpcName string) string {
	return ListLabel("vpc", vpcName)
}
