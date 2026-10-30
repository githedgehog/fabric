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

package v1beta1

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/maruel/natural"
	"github.com/samber/lo"
	"go.githedgehog.com/fabric/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	ManagementPortPrefix        = "M"
	ManagementPortNOSNamePrefix = "Management"
	DataPortPrefix              = "E"
	DataPortNOSNamePrefix       = "Ethernet"
	BreakoutNOSNamePrefix       = "1/"
	ONIEPortNamePrefix          = "eth"

	// it's used to mark ports that are supposed to be breakout but not like last port of the 32 port switch being non-breakout
	NonBreakoutPortExceptionSuffix = "-nb"
)

// Defines features supported by a specific switch which is later used for roles and Fabric API features usage validation
type SwitchProfileFeatures struct {
	// Subinterfaces defines if switch supports subinterfaces
	Subinterfaces bool `json:"subinterfaces,omitempty"`
	// ACLs defines if switch supports ACLs
	ACLs bool `json:"acls,omitempty"`
	// L2VNI defines if switch supports L2 VNIs
	L2VNI bool `json:"l2vni,omitempty"`
	// L3VNI defines if switch supports L3 VNIs
	L3VNI bool `json:"l3vni,omitempty"`
	// RoCE defines if switch supports RoCEv2 over VXLAN and related features used by the fabric
	RoCE bool `json:"roce,omitempty"`
	// MCLAG defines if switch supports MCLAG (with VXLAN)
	MCLAG bool `json:"mclag,omitempty"`
	// ESLAG defines if switch supports ESLAG (ESI multi-homing)
	ESLAG bool `json:"eslag,omitempty"`
	// ECMPRoCEQPN defines if switch supports ECMP QPN hashing
	ECMPRoCEQPN bool `json:"ecmpRoCEQPN,omitempty"`
	// PortLocator defines if switch supports port locator LED control
	PortLocator bool `json:"portLocator,omitempty"`
}

// Defines switch-specific configuration options
type SwitchProfileConfig struct {
	// MaxPathsIBGP defines the maximum number of IBGP paths to be configured
	MaxPathsEBGP uint32 `json:"maxPathsEBGP,omitempty"`
}

// Defines a switch port configuration
// Only one of Profile or Group can be set
type SwitchProfilePort struct {
	// NOSName defines how port is named in the NOS
	NOSName string `json:"nos,omitempty"`
	// BaseNOSName defines the base NOS name that could be used together with the profile to generate the actual NOS name (e.g. breakouts)
	BaseNOSName string `json:"baseNOSName,omitempty"`
	// Label defines the physical port label you can see on the actual switch
	Label string `json:"label,omitempty"`
	// If port isn't directly manageable, group defines the group it belongs to, exclusive with profile
	Group string `json:"group,omitempty"`
	// If port is directly configurable, profile defines the profile it belongs to, exclusive with group
	Profile string `json:"profile,omitempty"`
	// Management defines if port is a management port, it's a special case and it can't have a group or profile
	Management bool `json:"management,omitempty"`
	// OniePortName defines the ONIE port name for management ports only
	OniePortName string `json:"oniePortName,omitempty"`
	// Pipeline defines the pipeline used by the port, on some platforms multiple ports share the same pipeline
	Pipeline string `json:"pipeline,omitempty"`
}

// Defines a switch port group configuration
type SwitchProfilePortGroup struct {
	// NOSName defines how group is named in the NOS
	NOSName string `json:"nos,omitempty"`
	// Profile defines the possible configuration profile for the group, could only have speed profile
	Profile string `json:"profile,omitempty"`
}

// Defines a switch port profile speed configuration
type SwitchProfilePortProfileSpeed struct {
	// Default defines the default speed for the profile
	Default string `json:"default,omitempty"`
	// Supported defines the supported speeds for the profile
	Supported []string `json:"supported,omitempty"`
}

// Defines a switch port profile breakout configuration
type SwitchProfilePortProfileBreakout struct {
	// Default defines the default breakout mode for the profile
	Default string `json:"default,omitempty"`
	// Supported defines the supported breakout modes for the profile with the NOS name offsets
	Supported map[string]SwitchProfilePortProfileBreakoutMode `json:"supported,omitempty"`
}

// Defines a switch port profile breakout mode configuration
type SwitchProfilePortProfileBreakoutMode struct {
	// Offsets defines the breakout NOS port name offset from the port NOS Name for each breakout mode
	Offsets []string `json:"offsets,omitempty"`
}

// Defines a switch port profile configuration
type SwitchProfilePortProfile struct {
	// Speed defines the speed configuration for the profile, exclusive with breakout
	Speed *SwitchProfilePortProfileSpeed `json:"speed,omitempty"`
	// Breakout defines the breakout configuration for the profile, exclusive with speed
	Breakout *SwitchProfilePortProfileBreakout `json:"breakout,omitempty"`
	// AutoNegAllowed defines if configuring auto-negotiation is allowed for the port
	AutoNegAllowed bool `json:"autoNegAllowed,omitempty"`
	// AutoNegDefault defines the default auto-negotiation state for the port
	AutoNegDefault bool `json:"autoNegDefault,omitempty"`
}

type SwitchProfilePipeline struct {
	// MaxPorts defines the maximum number of ports (breakouts) allowed for the pipeline
	MaxPorts uint16 `json:"maxPorts,omitempty"`
}

// SwitchProfileSpec defines the desired state of SwitchProfile
type SwitchProfileSpec struct {
	// DisplayName defines the human-readable name of the switch
	DisplayName string `json:"displayName,omitempty"`
	// OtherNames defines alternative names for the switch
	OtherNames []string `json:"otherNames,omitempty"`
	// SwitchSilicon defines the switch silicon name
	SwitchSilicon string `json:"switchSilicon,omitempty"`
	// Features defines the features supported by the switch
	Features SwitchProfileFeatures `json:"features,omitempty"`
	// Notes is a free-form field for any additional information about the switch
	Notes string `json:"notes,omitempty"`
	// Config defines the switch-specific configuration options
	Config SwitchProfileConfig `json:"config,omitempty"`
	// Ports defines the switch port configuration
	Ports map[string]SwitchProfilePort `json:"ports,omitempty"`
	// PortGroups defines the switch port group configuration
	PortGroups map[string]SwitchProfilePortGroup `json:"portGroups,omitempty"`
	// PortProfiles defines the switch port profile configuration
	PortProfiles map[string]SwitchProfilePortProfile `json:"portProfiles,omitempty"`
	// NOSType defines the NOS type to be used for the switch
	NOSType meta.NOSType `json:"nosType,omitempty"`
	// Platform is what expected to be request by ONIE and displayed in the NOS
	Platform string `json:"platform,omitempty"`
	// Pipelines defines the switch pipeline configuration
	Pipelines map[string]SwitchProfilePipeline `json:"pipelines,omitempty"`
	// MaxPorts defines the maximum number of ports (breakouts) allowed for the switch
	MaxPorts uint16 `json:"maxPorts,omitempty"`
}

// SwitchProfileStatus defines the observed state of SwitchProfile
type SwitchProfileStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=sp
// +kubebuilder:printcolumn:name="DisplayName",type=string,JSONPath=`.spec.displayName`,priority=0
// +kubebuilder:printcolumn:name="OtherNames",type=string,JSONPath=`.spec.otherNames`,priority=0
// +kubebuilder:printcolumn:name="Silicon",type=string,JSONPath=`.spec.switchSilicon`,priority=0
// +kubebuilder:printcolumn:name="Ports",type=string,JSONPath=`.metadata.annotations.fabric\.githedgehog\.com/ports`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
// SwitchProfile represents switch capabilities and configuration
type SwitchProfile struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SwitchProfileSpec   `json:"spec,omitempty"`
	Status SwitchProfileStatus `json:"status,omitempty"`
}

const KindSwitchProfile = "SwitchProfile"

// +kubebuilder:object:root=true

// SwitchProfileList contains a list of SwitchProfile
type SwitchProfileList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []SwitchProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &SwitchProfile{}, &SwitchProfileList{})

		return nil
	})
}

var (
	_ meta.Object     = (*SwitchProfile)(nil)
	_ meta.ObjectList = (*SwitchProfileList)(nil)
)

func (spList *SwitchProfileList) GetItems() []meta.Object {
	items := make([]meta.Object, len(spList.Items))
	for i := range spList.Items {
		items[i] = &spList.Items[i]
	}

	return items
}

func (sp *SwitchProfile) Default() {
	meta.DefaultObjectMetadata(sp)

	if sp.Annotations == nil {
		sp.Annotations = map[string]string{}
	}

	portsStr, err := sp.Spec.GetPortsShortSummary()
	if err == nil {
		sp.Annotations[AnnotationPorts] = portsStr
	}
}

func (sp *SwitchProfile) Validate(_ context.Context, _ kclient.Reader, _ *meta.FabricConfig) (admission.Warnings, error) {
	if err := meta.ValidateObjectMetadata(sp); err != nil {
		return nil, err //nolint:wrapcheck
	}

	spec := field.NewPath("spec")

	if sp.Spec.DisplayName == "" {
		return nil, field.Required(spec.Child("displayName"), "")
	}

	if len(sp.Spec.OtherNames) > 5 {
		return nil, field.TooMany(spec.Child("otherNames"), len(sp.Spec.OtherNames), 5)
	}
	for curr, name := range sp.Spec.OtherNames {
		if name == "" {
			return nil, field.Required(spec.Child("otherNames").Index(curr), "")
		}

		if idx := slices.Index(sp.Spec.OtherNames, name); idx != curr {
			return nil, field.Duplicate(spec.Child("otherNames").Index(curr), name)
		}
	}

	if sp.Spec.SwitchSilicon == "" {
		return nil, field.Required(spec.Child("switchSilicon"), "")
	}

	if sp.Spec.NOSType == "" {
		return nil, field.Required(spec.Child("nosType"), "")
	}
	if !slices.Contains(meta.NOSTypes, sp.Spec.NOSType) {
		return nil, field.NotSupported(spec.Child("nosType"), sp.Spec.NOSType, meta.NOSTypes)
	}

	if sp.Spec.Platform == "" {
		return nil, field.Required(spec.Child("platform"), "")
	}

	profiles := map[string]bool{}
	groups := map[string]bool{}
	nosPortNames := map[string]bool{}
	baseNOSPortNames := map[string]bool{}
	labels := map[string]bool{}

	for name, port := range sp.Spec.Ports {
		if port.NOSName == "" {
			return nil, field.Invalid(spec.Child("ports").Key(name).Child("nos"), field.OmitValueType{}, "must have a NOS name")
		}

		if len(name) < 2 {
			return nil, field.Invalid(spec.Child("ports").Key(name), field.OmitValueType{}, "name must have with at least two characters")
		}

		if _, ok := nosPortNames[port.NOSName]; ok {
			return nil, field.Invalid(spec.Child("ports").Key(name).Child("nos"), field.OmitValueType{}, fmt.Sprintf("NOS name %q is duplicated", port.NOSName))
		}
		nosPortNames[port.NOSName] = true

		if port.BaseNOSName != "" {
			if _, ok := baseNOSPortNames[port.BaseNOSName]; ok {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("baseNOSName"), field.OmitValueType{}, fmt.Sprintf("base NOS name %q is duplicated", port.BaseNOSName))
			}
			baseNOSPortNames[port.BaseNOSName] = true
		}

		if port.Management {
			if !strings.HasPrefix(name, ManagementPortPrefix) {
				return nil, field.Invalid(spec.Child("ports").Key(name), field.OmitValueType{}, "management port name must start with M")
			}

			if port, err := strconv.Atoi(name[1:]); err != nil || port <= 0 {
				return nil, field.Invalid(spec.Child("ports").Key(name), field.OmitValueType{}, "management port name must end with a positive integer")
			}

			if !strings.HasPrefix(port.NOSName, ManagementPortNOSNamePrefix) {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("nos"), field.OmitValueType{}, "management port NOS name must start with Management")
			}

			if port, err := strconv.Atoi(port.NOSName[len(ManagementPortNOSNamePrefix):]); err != nil || port < 0 {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("nos"), field.OmitValueType{}, "management port NOS name must end with a positive integer")
			}

			if port.OniePortName == "" {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("oniePortName"), field.OmitValueType{}, "management port must have an ONIE port name")
			}

			if !strings.HasPrefix(port.OniePortName, ONIEPortNamePrefix) {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("oniePortName"), field.OmitValueType{}, "management port ONIE port name must start with eth")
			}

			if port, err := strconv.Atoi(port.OniePortName[len(ONIEPortNamePrefix):]); err != nil || port < 0 {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("oniePortName"), field.OmitValueType{}, "management port ONIE port name must end with a zero or positive integer")
			}

			if port.Group != "" {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("group"), field.OmitValueType{}, "management port must not have a group")
			}

			if port.Profile != "" {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("profile"), field.OmitValueType{}, "management port must not have a profile")
			}

			if port.BaseNOSName != "" {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("baseNOSName"), field.OmitValueType{}, "management port must not have a base NOS name")
			}

			continue
		}

		if !strings.HasPrefix(name, DataPortPrefix) {
			return nil, field.Invalid(spec.Child("ports").Key(name), field.OmitValueType{}, "data port must start with E")
		}

		portParts := strings.Split(name[len(DataPortPrefix):], "/")
		if len(portParts) != 2 {
			return nil, field.Invalid(spec.Child("ports").Key(name), field.OmitValueType{}, "data port name must have two segments separated by a slash (e.g. asic/port)")
		}
		if asic, err := strconv.Atoi(portParts[0]); err != nil || portParts[0] == "" || asic <= 0 {
			return nil, field.Invalid(spec.Child("ports").Key(name), field.OmitValueType{}, "data port name must contain a positive integer as the first segment (e.g. asic)")
		}
		if port, err := strconv.Atoi(portParts[1]); err != nil || portParts[1] == "" || port <= 0 {
			return nil, field.Invalid(spec.Child("ports").Key(name), field.OmitValueType{}, "data port name must contain a positive integer as the second segment (port)")
		}

		if port.Label == "" {
			return nil, field.Invalid(spec.Child("ports").Key(name).Child("label"), field.OmitValueType{}, "must have a label")
		}

		if _, ok := labels[port.Label]; ok {
			return nil, field.Invalid(spec.Child("ports").Key(name).Child("label"), field.OmitValueType{}, fmt.Sprintf("label %q is duplicated", port.Label))
		}
		labels[port.Label] = true

		if port.Profile == "" && port.Group == "" {
			return nil, field.Invalid(spec.Child("ports").Key(name), field.OmitValueType{}, "must have a profile or group")
		}

		if port.Profile != "" && port.Group != "" {
			return nil, field.Invalid(spec.Child("ports").Key(name), field.OmitValueType{}, "must have either a profile or group, not both")
		}

		isBreakout := false
		if port.Profile != "" {
			profile, exists := sp.Spec.PortProfiles[port.Profile]
			if !exists {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("profile"), field.OmitValueType{}, fmt.Sprintf("references non-existent profile %q", port.Profile))
			}

			profiles[port.Profile] = true

			if profile.Breakout != nil {
				isBreakout = true

				if port.NOSName == "" {
					return nil, field.Invalid(spec.Child("ports").Key(name).Child("nos"), field.OmitValueType{}, "breakout port must have a NOS name")
				}

				if !strings.HasPrefix(port.NOSName, BreakoutNOSNamePrefix) {
					return nil, field.Invalid(spec.Child("ports").Key(name).Child("nos"), field.OmitValueType{}, fmt.Sprintf("breakout port NOS name must start with %s", BreakoutNOSNamePrefix))
				}

				if _, err := strconv.Atoi(port.NOSName[len(BreakoutNOSNamePrefix):]); err != nil {
					return nil, field.Invalid(spec.Child("ports").Key(name).Child("nos"), field.OmitValueType{}, "breakout port NOS name must end with a positive integer")
				}

				if port.BaseNOSName == "" {
					return nil, field.Invalid(spec.Child("ports").Key(name).Child("baseNOSName"), field.OmitValueType{}, "breakout port must have a base NOS name")
				}

				if !strings.HasPrefix(port.BaseNOSName, DataPortNOSNamePrefix) {
					return nil, field.Invalid(spec.Child("ports").Key(name).Child("baseNOSName"), field.OmitValueType{}, fmt.Sprintf("breakout port base NOS name must start with %s", DataPortNOSNamePrefix))
				}

				if port, err := strconv.Atoi(port.BaseNOSName[len(DataPortNOSNamePrefix):]); err != nil || port < 0 {
					return nil, field.Invalid(spec.Child("ports").Key(name).Child("baseNOSName"), field.OmitValueType{}, "breakout port base NOS name must end with a zero or positive integer")
				}
			}
		}

		if port.Pipeline != "" {
			_, ok := sp.Spec.Pipelines[port.Pipeline]
			if !ok {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("pipeline"), field.OmitValueType{}, fmt.Sprintf("references non-existent pipeline %q", port.Pipeline))
			}

			if !isBreakout {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("pipeline"), field.OmitValueType{}, "must be a breakout port to reference a pipeline")
			}
		}

		if !isBreakout {
			if !strings.HasPrefix(port.NOSName, DataPortNOSNamePrefix) {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("nos"), field.OmitValueType{}, fmt.Sprintf("data port NOS name must start with %s", DataPortNOSNamePrefix))
			}

			if port, err := strconv.Atoi(port.NOSName[len(DataPortNOSNamePrefix):]); err != nil || port < 0 {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("nos"), field.OmitValueType{}, "data port NOS name must end with a zero or positive integer")
			}
		}

		if port.Group != "" {
			if _, ok := sp.Spec.PortGroups[port.Group]; !ok {
				return nil, field.Invalid(spec.Child("ports").Key(name).Child("group"), field.OmitValueType{}, fmt.Sprintf("references non-existent group %q", port.Group))
			}

			groups[port.Group] = true
		}
	}

	for name, group := range sp.Spec.PortGroups {
		if _, ok := groups[name]; !ok {
			return nil, field.Invalid(spec.Child("portGroups").Key(name), field.OmitValueType{}, "is not referenced by any port")
		}

		if group.NOSName == "" {
			return nil, field.Invalid(spec.Child("portGroups").Key(name).Child("nos"), field.OmitValueType{}, "must have a NOS name")
		}

		if nosName, err := strconv.Atoi(group.NOSName); err != nil || nosName <= 0 {
			return nil, field.Invalid(spec.Child("portGroups").Key(name).Child("nos"), field.OmitValueType{}, "NOS name must be a positive integer")
		}

		if group.Profile == "" {
			return nil, field.Invalid(spec.Child("portGroups").Key(name).Child("profile"), field.OmitValueType{}, "must have a profile")
		}

		if _, ok := sp.Spec.PortProfiles[group.Profile]; !ok {
			return nil, field.Invalid(spec.Child("portGroups").Key(name).Child("profile"), field.OmitValueType{}, fmt.Sprintf("references non-existent profile %q", group.Profile))
		}

		profiles[group.Profile] = true

		if sp.Spec.PortProfiles[group.Profile].Speed == nil {
			return nil, field.Invalid(spec.Child("portGroups").Key(name).Child("profile"), field.OmitValueType{}, fmt.Sprintf("references non-speed profile %q", group.Profile))
		}

		if profile := sp.Spec.PortProfiles[group.Profile]; profile.AutoNegAllowed {
			return nil, field.Invalid(spec.Child("portGroups").Key(name).Child("profile"), field.OmitValueType{}, fmt.Sprintf("profile %q must not have auto-negotiation allowed", group.Profile))
		}
	}

	for name, profile := range sp.Spec.PortProfiles {
		if _, ok := profiles[name]; !ok {
			return nil, field.Invalid(spec.Child("portProfiles").Key(name), field.OmitValueType{}, "is not referenced by any port or group")
		}

		if profile.Speed == nil && profile.Breakout == nil {
			return nil, field.Invalid(spec.Child("portProfiles").Key(name), field.OmitValueType{}, "must have a speed or breakout")
		}

		if profile.Speed != nil && profile.Breakout != nil {
			return nil, field.Invalid(spec.Child("portProfiles").Key(name), field.OmitValueType{}, "must have either a speed or breakout, not both")
		}

		if profile.AutoNegDefault && !profile.AutoNegAllowed {
			return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("autoNegDefault"), field.OmitValueType{}, "must not have default auto-negotiation enabled if it's not allowed")
		}

		if profile.Speed != nil {
			if profile.Speed.Default == "" {
				return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("speed", "default"), field.OmitValueType{}, "must have a default speed")
			}

			if len(profile.Speed.Supported) == 0 {
				return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("speed", "supported"), field.OmitValueType{}, "must have supported speeds")
			}

			if !slices.Contains(profile.Speed.Supported, profile.Speed.Default) {
				return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("speed", "default"), field.OmitValueType{}, "must have default speed in supported speeds")
			}

			for _, speed := range profile.Speed.Supported {
				if speed == "" {
					return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("speed", "supported"), field.OmitValueType{}, "must have non-empty speeds")
				}

				if err := ValidatePortSpeed(speed); err != nil {
					return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("speed", "supported"), speed, err.Error())
				}
			}
		}

		if profile.Breakout != nil {
			if profile.Breakout.Default == "" {
				return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("breakout", "default"), field.OmitValueType{}, "must have a default breakout")
			}

			if len(profile.Breakout.Supported) == 0 {
				return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("breakout", "supported"), field.OmitValueType{}, "must have supported breakouts")
			}

			if _, ok := profile.Breakout.Supported[profile.Breakout.Default]; !ok {
				return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("breakout", "default"), field.OmitValueType{}, "must have default breakout in supported breakouts")
			}

			for mode, offsets := range profile.Breakout.Supported {
				if len(offsets.Offsets) == 0 {
					return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("breakout", "supported"), field.OmitValueType{}, fmt.Sprintf("must have non-empty offsets for mode %q", mode))
				}

				if mode == "" {
					return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("breakout", "supported"), field.OmitValueType{}, "must have non-empty modes")
				}

				if err := ValidatePortBreakoutMode(mode); err != nil {
					return nil, field.Invalid(spec.Child("portProfiles").Key(name).Child("breakout", "supported"), mode, err.Error())
				}
			}
		}
	}

	for name, pipeline := range sp.Spec.Pipelines {
		if name == "" {
			return nil, field.Invalid(spec.Child("pipelines").Key(name), field.OmitValueType{}, "must have a name")
		}

		if pipeline.MaxPorts == 0 {
			return nil, field.Invalid(spec.Child("pipelines").Key(name).Child("maxPorts"), field.OmitValueType{}, "must have a non-zero max ports")
		}
	}

	return nil, nil
}

var allowedPortSpeeds = map[string]bool{
	"1G":   true,
	"2.5G": true,
	"5G":   true,
	"10G":  true,
	"20G":  true,
	"25G":  true,
	"40G":  true,
	"50G":  true,
	"100G": true,
	"200G": true,
	"400G": true,
	"800G": true,
}

func ValidatePortSpeed(speed string) error {
	if !strings.HasSuffix(speed, "G") {
		return fmt.Errorf("speed %q must have a G suffix", speed) //nolint:err113
	}

	if !allowedPortSpeeds[speed] {
		return fmt.Errorf("speed %q is not allowed", speed) //nolint:err113
	}

	return nil
}

var allowedPortBreakoutNumbers = map[string]bool{
	"1": true,
	"2": true,
	"4": true,
	"8": true,
}

func ValidatePortBreakoutMode(mode string) error {
	parts := strings.Split(mode, "x")
	if len(parts) != 2 {
		return fmt.Errorf("mode %q must have axactly one 'x' as a separator", mode) //nolint:err113
	}

	number := parts[0]
	speed := parts[1]

	if number == "" {
		return fmt.Errorf("mode %q must have a number before 'x'", mode) //nolint:err113
	}

	if speed == "" {
		return fmt.Errorf("mode %q must have a speed after 'x'", mode) //nolint:err113
	}

	if !allowedPortBreakoutNumbers[number] {
		return fmt.Errorf("mode %q must have a valid number", mode) //nolint:err113
	}

	return ValidatePortSpeed(speed)
}

func (sp *SwitchProfileSpec) GetAPI2NOSPortsFor(sw *SwitchSpec) (map[string]string, error) {
	if sp == nil {
		return nil, fmt.Errorf("switch profile spec is nil") //nolint:err113
	}
	if sw == nil {
		return nil, fmt.Errorf("switch spec is nil") //nolint:err113
	}

	ports := map[string]string{}

	for portName, port := range sp.Ports {
		if port.Management {
			ports[portName] = port.NOSName

			continue
		}

		if port.Profile != "" && sp.PortProfiles[port.Profile].Breakout != nil {
			breakoutProfile := sp.PortProfiles[port.Profile].Breakout

			swBreakout, ok := sw.PortBreakouts[portName]
			if !ok {
				swBreakout = breakoutProfile.Default
			}

			if breakoutMode, ok := breakoutProfile.Supported[swBreakout]; ok {
				nosNameBaseStr, cut := strings.CutPrefix(port.BaseNOSName, DataPortNOSNamePrefix)
				if !cut {
					return nil, fmt.Errorf("port %q base NOS name %q is invalid (no expected prefix)", portName, port.NOSName) //nolint:err113
				}
				nosNameBase, err := strconv.Atoi(nosNameBaseStr)
				if err != nil {
					return nil, fmt.Errorf("port %q base NOS name %q is invalid (suffix isn't a number)", portName, port.NOSName) //nolint:err113
				}

				for breakoutIdx, offsetStr := range breakoutMode.Offsets {
					offset, err := strconv.Atoi(offsetStr)
					if err != nil {
						return nil, fmt.Errorf("port %q NOS name %q breakout mode %q offset %q is invalid (not a number)", portName, port.NOSName, swBreakout, offsetStr) //nolint:err113
					}

					nosName := fmt.Sprintf("%s%d", DataPortNOSNamePrefix, nosNameBase+offset)

					if breakoutIdx == 0 {
						ports[portName] = nosName
					}

					ports[fmt.Sprintf("%s/%d", portName, breakoutIdx+1)] = nosName
				}
			} else {
				return nil, fmt.Errorf("port %q has a breakout %q not supported by profile %q", portName, swBreakout, port.Profile) //nolint:err113
			}
		} else {
			ports[portName] = port.NOSName
		}
	}

	return ports, nil
}

func (sp *SwitchProfileSpec) GetNOS2APIPortsFor(sw *SwitchSpec) (map[string]string, error) {
	if sp == nil {
		return nil, fmt.Errorf("switch profile spec is nil") //nolint:err113
	}
	if sw == nil {
		return nil, fmt.Errorf("switch spec is nil") //nolint:err113
	}

	api2NOS, err := sp.GetAPI2NOSPortsFor(sw)
	if err != nil {
		return nil, fmt.Errorf("failed to get API to NOS ports: %w", err)
	}

	ports := map[string]string{}
	for apiPort, nosPort := range api2NOS {
		if prevAPIPort, exists := ports[nosPort]; exists {
			if prevAPIPort+"/1" == apiPort {
				ports[nosPort] = apiPort

				continue
			} else if prevAPIPort == apiPort+"/1" {
				continue
			}

			return nil, fmt.Errorf("NOS port %q is duplicated", nosPort) //nolint:err113
		}

		ports[nosPort] = apiPort
	}

	return ports, nil
}

func (sp *SwitchProfileSpec) GetNOS2APIBreakouts() (map[string]string, error) {
	if sp == nil {
		return nil, fmt.Errorf("switch profile spec is nil") //nolint:err113
	}

	ports := map[string]string{}
	for apiName, port := range sp.Ports {
		if port.Profile == "" {
			continue
		}

		profile, exists := sp.PortProfiles[port.Profile]
		if !exists {
			return nil, fmt.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile) //nolint:err113
		}
		if profile.Breakout == nil {
			continue
		}

		if _, exists := ports[port.NOSName]; exists {
			return nil, fmt.Errorf("NOS port %q is duplicated", port.NOSName) //nolint:err113
		}

		ports[port.NOSName] = apiName
	}

	return ports, nil
}

func (sp *SwitchProfileSpec) GetAllBreakoutNOSNames() (map[string]bool, error) {
	if sp == nil {
		return nil, fmt.Errorf("switch profile spec is nil") //nolint:err113
	}

	ports := map[string]bool{}

	for portName, port := range sp.Ports {
		if port.Profile == "" {
			continue
		}

		profile, ok := sp.PortProfiles[port.Profile]
		if !ok {
			return nil, fmt.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile) //nolint:err113
		}

		if profile.Breakout == nil {
			continue
		}

		nosNameBaseStr, cut := strings.CutPrefix(port.BaseNOSName, DataPortNOSNamePrefix)
		if !cut {
			return nil, fmt.Errorf("port %q base NOS name %q is invalid (no expected prefix)", portName, port.NOSName) //nolint:err113
		}
		nosNameBase, err := strconv.Atoi(nosNameBaseStr)
		if err != nil {
			return nil, fmt.Errorf("port %q base NOS name %q is invalid (suffix isn't a number)", portName, port.NOSName) //nolint:err113
		}

		for _, breakoutMode := range profile.Breakout.Supported {
			for _, offsetStr := range breakoutMode.Offsets {
				offset, err := strconv.Atoi(offsetStr)
				if err != nil {
					return nil, fmt.Errorf("port %q NOS name %q breakout mode %q offset %q is invalid (not a number)", portName, port.NOSName, breakoutMode, offsetStr) //nolint:err113
				}

				nosName := fmt.Sprintf("%s%d", DataPortNOSNamePrefix, nosNameBase+offset)
				ports[nosName] = true
			}
		}
	}

	return ports, nil
}

func (sp *SwitchProfileSpec) NormalizePortName(portName string) (string, error) {
	if sp == nil {
		return "", fmt.Errorf("switch profile spec is nil") //nolint:err113
	}

	if strings.Count(portName, "/") == 2 {
		return portName, nil
	}

	port, exists := sp.Ports[portName]
	if !exists {
		return "", fmt.Errorf("port %q does not exist in the switch profile", portName) //nolint:err113
	}

	if port.Profile == "" {
		return portName, nil
	}

	profile, exists := sp.PortProfiles[port.Profile]
	if !exists {
		return "", fmt.Errorf("port %q references non-existent port profile %q", portName, port.Profile) //nolint:err113
	}

	if profile.Breakout == nil {
		return portName, nil
	}

	return portName + "/1", nil
}

type SwitchProfilePortSummary struct {
	Name      string
	Label     string
	Type      string
	Group     string
	Default   string
	Supported string
}

func (sp *SwitchProfileSpec) GetPortsSummary() ([]SwitchProfilePortSummary, error) {
	if sp == nil {
		return nil, fmt.Errorf("switch profile spec is nil") //nolint:err113
	}

	res := []SwitchProfilePortSummary{}

	portNames := lo.Keys(sp.Ports)
	SortPortNames(portNames)

	for _, portName := range portNames {
		port := sp.Ports[portName]

		t := ""
		def := ""
		supported := ""

		if port.Management { //nolint:gocritic
			t = "Management"
		} else if port.Group != "" {
			t = "Port Group"
			gr, exists := sp.PortGroups[port.Group]
			if !exists {
				return nil, fmt.Errorf("port %q references non-existent group %q", portName, port.Group) //nolint:err113
			}

			profile, exists := sp.PortProfiles[gr.Profile]
			if !exists {
				return nil, fmt.Errorf("group %q references non-existent profile %q", port.Group, gr.Profile) //nolint:err113
			}

			if profile.Speed != nil {
				def = profile.Speed.Default
				supported = strings.Join(profile.Speed.Supported, ", ")
			}
		} else if port.Profile != "" {
			profile := sp.PortProfiles[port.Profile]
			if profile.Speed != nil {
				t = "Direct"

				if profile.Speed != nil {
					def = profile.Speed.Default
					supported = strings.Join(profile.Speed.Supported, ", ")
				}

				if profile.AutoNegAllowed {
					supported += fmt.Sprintf(", AutoNeg supported (default: %v)", profile.AutoNegDefault)
				}
			} else if profile.Breakout != nil {
				t = "Breakout"

				if profile.Breakout != nil {
					def = profile.Breakout.Default
					modes := lo.Keys(profile.Breakout.Supported)
					slices.Sort(modes)
					supported = strings.Join(modes, ", ")
				}
			}
		}

		summary := SwitchProfilePortSummary{
			Name:      portName,
			Label:     port.Label,
			Type:      t,
			Group:     port.Group,
			Default:   def,
			Supported: supported,
		}

		res = append(res, summary)
	}

	return res, nil
}

func SortPortNames(portNames []string) {
	slices.SortFunc(portNames, ComparePortNames)
}

func ComparePortNames(a, b string) int {
	if a[0] != b[0] {
		if strings.HasPrefix(a, ManagementPortPrefix) {
			return -1
		}
		if strings.HasPrefix(b, ManagementPortPrefix) {
			return 1
		}
	}

	if a == b {
		return 0
	} else if natural.Less(a, b) {
		return -1
	}

	return 1
}

func (sp *SwitchProfileSpec) GetAutoNegsDefaultsFor(sw *SwitchSpec) (map[string]bool, map[string]bool, error) {
	if sp == nil {
		return nil, nil, fmt.Errorf("switch profile spec is nil") //nolint:err113
	}
	if sw == nil {
		return nil, nil, fmt.Errorf("switch spec is nil") //nolint:err113
	}

	allowed, def := map[string]bool{}, map[string]bool{}

	for portName, port := range sp.Ports {
		if port.Management {
			continue
		}

		if port.Profile != "" { //nolint:gocritic
			profile, exists := sp.PortProfiles[port.Profile]
			if !exists {
				return nil, nil, fmt.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile) //nolint:err113
			}

			if profile.Speed != nil { //nolint:gocritic
				allowed[portName] = profile.AutoNegAllowed
				def[portName] = profile.AutoNegDefault
			} else if profile.Breakout != nil {
				breakout := profile.Breakout.Default
				if swBreakout, ok := sw.PortBreakouts[portName]; ok {
					breakout = swBreakout
				}

				breakoutProfile, ok := profile.Breakout.Supported[breakout]
				if !ok {
					return nil, nil, fmt.Errorf("port %q has a breakout %q not supported by profile %q", portName, breakout, port.Profile) //nolint:err113
				}

				for idx := range breakoutProfile.Offsets {
					allowed[fmt.Sprintf("%s/%d", portName, idx+1)] = profile.AutoNegAllowed
					def[fmt.Sprintf("%s/%d", portName, idx+1)] = profile.AutoNegDefault
				}
			} else {
				return nil, nil, fmt.Errorf("port %q profile %q has no speed or breakout", portName, port.Profile) //nolint:err113
			}
		} else if port.Group != "" {
			// autoneg is not allowed for groups
			continue
		} else {
			return nil, nil, fmt.Errorf("port %q must have a profile or group", portName) //nolint:err113
		}
	}

	return allowed, def, nil
}

func (sp *SwitchProfileSpec) GetBreakoutDefaults(sw *SwitchSpec) (map[string]string, error) {
	if sp == nil {
		return nil, fmt.Errorf("switch profile spec is nil") //nolint:err113
	}
	if sw == nil {
		return nil, fmt.Errorf("switch spec is nil") //nolint:err113
	}

	def := map[string]string{}

	for portName, port := range sp.Ports {
		if port.Management || port.Profile == "" {
			continue
		}

		profile, exists := sp.PortProfiles[port.Profile]
		if !exists {
			return nil, fmt.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile) //nolint:err113
		}

		if profile.Breakout == nil {
			continue
		}

		breakout := profile.Breakout.Default
		if swBreakout, ok := sw.PortBreakouts[portName]; ok {
			breakout = swBreakout
		}

		def[portName] = breakout
	}

	return def, nil
}

func (sp *SwitchProfileSpec) GetAvailableAPIPorts(sw *SwitchSpec) (map[string]bool, error) {
	if sp == nil {
		return nil, fmt.Errorf("switch profile spec is nil") //nolint:err113
	}
	if sw == nil {
		sw = &SwitchSpec{}
	}

	ports := map[string]bool{}

	for portName, port := range sp.Ports {
		if port.Management {
			continue
		}

		if port.Profile != "" { //nolint:gocritic
			profile, exists := sp.PortProfiles[port.Profile]
			if !exists {
				return nil, fmt.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile) //nolint:err113
			}

			if profile.Breakout != nil { //nolint:gocritic
				b := profile.Breakout.Default
				if swB, ok := sw.PortBreakouts[portName]; ok {
					b = swB
				}

				if mode, ok := profile.Breakout.Supported[b]; ok {
					for idx := range len(mode.Offsets) {
						ports[fmt.Sprintf("%s/%d", portName, idx+1)] = true
					}
				} else {
					return nil, fmt.Errorf("port %q has a breakout %q not supported by profile %q", portName, b, port.Profile) //nolint:err113
				}
			} else if profile.Speed != nil {
				ports[portName] = true
			} else {
				return nil, fmt.Errorf("port %q references profile %q without speed or breakout", port.NOSName, port.Profile) //nolint:err113
			}
		} else if port.Group != "" {
			ports[portName] = true
		} else {
			return nil, fmt.Errorf("port %q must have a profile or group", port.NOSName) //nolint:err113
		}
	}

	return ports, nil
}

// GetFECConfigurablePorts returns the set of API port names on which FEC may be configured.
// It is GetAvailableAPIPorts (speed/group base names and breakout sub-port names) plus the
// base name of any breakout port that is NOT broken out (its configured mode has a single
// lane, e.g. "E1/53" at 1x100G), which may be addressed directly. A broken-out (multi-lane)
// port keeps only its sub-port names, so its base name is rejected and the explicit sub-port
// name (e.g. "E1/53/1") must be used.
func (sp *SwitchProfileSpec) GetFECConfigurablePorts(sw *SwitchSpec) (map[string]bool, error) {
	if sw == nil {
		sw = &SwitchSpec{}
	}

	ports, err := sp.GetAvailableAPIPorts(sw)
	if err != nil {
		return nil, err
	}

	for portName, port := range sp.Ports {
		if port.Management || port.Profile == "" {
			continue
		}

		profile, ok := sp.PortProfiles[port.Profile]
		if !ok || profile.Breakout == nil {
			continue
		}

		mode := profile.Breakout.Default
		if b, ok := sw.PortBreakouts[portName]; ok {
			mode = b
		}

		if bm, ok := profile.Breakout.Supported[mode]; ok && len(bm.Offsets) == 1 {
			ports[portName] = true // not broken out: base name is directly addressable
		}
	}

	return ports, nil
}

func (sp *SwitchProfileSpec) GetPortsShortSummary() (string, error) {
	portCount := map[string]int{}
	for _, port := range sp.Ports {
		if port.Management {
			continue
		}

		profile := ""
		if port.Profile != "" {
			profile = port.Profile
		}

		if port.Group != "" {
			group, exists := sp.PortGroups[port.Group]
			if !exists {
				return "", fmt.Errorf("port %q references non-existent group %q", port.NOSName, port.Group) //nolint:err113
			}

			profile = group.Profile
		}

		if profile == "" {
			return "", fmt.Errorf("port %q must have a profile or group", port.NOSName) //nolint:err113
		}

		portCount[strings.TrimSuffix(profile, NonBreakoutPortExceptionSuffix)]++
	}

	portTypes := lo.Keys(portCount)
	slices.SortFunc(portTypes, func(a, b string) int {
		return portCount[b] - portCount[a]
	})

	portsStr := ""
	for _, profile := range portTypes {
		count := portCount[profile]
		if portsStr != "" {
			portsStr += ", "
		}

		portsStr += fmt.Sprintf("%dx%s", count, profile)
	}

	return portsStr, nil
}

func (sp *SwitchProfileSpec) GetNameSummary() string {
	summary := sp.DisplayName
	otherNames := []string{}

	nameParts := strings.Split(sp.DisplayName, " ")
	if len(sp.OtherNames) > 0 {
		for _, otherName := range sp.OtherNames {
			for _, part := range nameParts {
				otherName = strings.TrimPrefix(otherName, part+"")
			}

			otherName = strings.TrimSpace(otherName)
			if otherName != "" {
				otherNames = append(otherNames, otherName)
			}
		}
	}

	if len(otherNames) > 0 {
		summary += " (" + strings.Join(otherNames, ", ") + ")"
	}

	return summary
}
