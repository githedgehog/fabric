package meta

// +kubebuilder:validation:Enum=mclag;eslag
// RedundancyType is the type of the redundancy group, could be mclag or eslag. It defines how redundancy will be
// configured and handled on the switch as well as which connection types will be available.
type RedundancyType string

const (
	RedundancyTypeNone  RedundancyType = ""
	RedundancyTypeMCLAG RedundancyType = "mclag"
	RedundancyTypeESLAG RedundancyType = "eslag"
)

var RedundancyTypes = []RedundancyType{
	RedundancyTypeNone,
	RedundancyTypeMCLAG,
	RedundancyTypeESLAG,
}
