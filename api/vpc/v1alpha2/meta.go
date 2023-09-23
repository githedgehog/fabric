package v1alpha2

var (
	LabelPrefix = "fabric.githedgehog.com/"
	LabelVPC    = LabelName("vpc")
)

func LabelName(name string) string {
	return LabelPrefix + name
}
