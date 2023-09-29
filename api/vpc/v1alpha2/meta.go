package v1alpha2

import "strings"

var (
	LabelPrefix    = "fabric.githedgehog.com/"
	LabelVPC       = LabelName("vpc")
	LabelSubnet    = LabelName("subnet")
	ListLabelValue = "true"
)

func LabelName(name string) string {
	return LabelPrefix + name
}

func EncodeSubnet(subnet string) string {
	return strings.ReplaceAll(subnet, "/", "_")
}

func ListLabelPrefix(listType string) string {
	return listType + "." + LabelPrefix
}

func ListLabel(listType, val string) string {
	return ListLabelPrefix(listType) + val
}

func ListLabelVPC(serverName string) string {
	return ListLabel("vpc", serverName)
}
