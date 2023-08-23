package v1alpha2

var (
	LabelPrefix   = GroupVersion.Group + "/"
	LabelRack     = LabelName("rack")
	LabelSwitch   = LabelName("switch")
	LabelServer   = LabelName("server")
	LabelLocation = LabelName("location")
)

func LabelName(name string) string {
	return LabelPrefix + name
}
