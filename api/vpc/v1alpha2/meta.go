package v1alpha2

import "strings"

var (
	LabelPrefix = "fabric.githedgehog.com/"
	LabelVPC    = LabelName("vpc")
	LabelSubnet = LabelName("subnet")
)

func LabelName(name string) string {
	return LabelPrefix + name
}

func EncodeSubnet(subnet string) string {
	return strings.ReplaceAll(subnet, "/", "_")
}
