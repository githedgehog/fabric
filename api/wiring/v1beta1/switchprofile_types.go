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
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"go.githedgehog.com/fabric/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// VXLAN defines if switch supports VXLANs
	VXLAN bool `json:"vxlan,omitempty"`
	// ACLs defines if switch supports ACLs
	ACLs bool `json:"acls,omitempty"`
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
}

// SwitchProfileStatus defines the observed state of SwitchProfile
type SwitchProfileStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=sp
// +kubebuilder:printcolumn:name="DisplayName",type=string,JSONPath=`.spec.displayName`,priority=0
// +kubebuilder:printcolumn:name="OtherNames",type=string,JSONPath=`.spec.otherNames`,priority=0
// +kubebuilder:printcolumn:name="Ports",type=string,JSONPath=`.metadata.annotations.fabric\.githedgehog\.com/ports`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// SwitchProfile represents switch capabilities and configuration
type SwitchProfile struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SwitchProfileSpec   `json:"spec,omitempty"`
	Status SwitchProfileStatus `json:"status,omitempty"`
}

const KindSwitchProfile = "SwitchProfile"

//+kubebuilder:object:root=true

// SwitchProfileList contains a list of SwitchProfile
type SwitchProfileList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []SwitchProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SwitchProfile{}, &SwitchProfileList{})
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
		return nil, errors.Wrapf(err, "failed to validate metadata")
	}

	if sp.Spec.DisplayName == "" {
		return nil, errors.Errorf("displayName is required")
	}

	if len(sp.Spec.OtherNames) > 5 {
		return nil, errors.Errorf("otherNames must not exceed 5 items")
	}
	for curr, name := range sp.Spec.OtherNames {
		if name == "" {
			return nil, errors.Errorf("otherNames must not contain empty strings")
		}

		if idx := slices.Index(sp.Spec.OtherNames, name); idx != curr {
			return nil, errors.Errorf("otherNames must not contain duplicates")
		}
	}

	if sp.Spec.SwitchSilicon == "" {
		return nil, errors.Errorf("switchSilicon is required")
	}

	if sp.Spec.NOSType == "" {
		return nil, errors.Errorf("nosType is required")
	}
	if !slices.Contains(meta.NOSTypes, sp.Spec.NOSType) {
		return nil, errors.Errorf("nosType %q is invalid", sp.Spec.NOSType)
	}

	if sp.Spec.Platform == "" {
		return nil, errors.Errorf("platform is required")
	}

	profiles := map[string]bool{}
	groups := map[string]bool{}
	nosPortNames := map[string]bool{}
	baseNOSPortNames := map[string]bool{}
	labels := map[string]bool{}

	for name, port := range sp.Spec.Ports {
		if port.NOSName == "" {
			return nil, errors.Errorf("port %q must have a NOS name", name)
		}

		if len(name) < 2 {
			return nil, errors.Errorf("port %q name must have with at least two characters", name)
		}

		if _, ok := nosPortNames[port.NOSName]; ok {
			return nil, errors.Errorf("port %q NOS name %q is duplicated", name, port.NOSName)
		}
		nosPortNames[port.NOSName] = true

		if port.BaseNOSName != "" {
			if _, ok := baseNOSPortNames[port.BaseNOSName]; ok {
				return nil, errors.Errorf("port %q base NOS name %q is duplicated", name, port.BaseNOSName)
			}
			baseNOSPortNames[port.BaseNOSName] = true
		}

		if port.Management {
			if !strings.HasPrefix(name, ManagementPortPrefix) {
				return nil, errors.Errorf("management port %q name must start with M", name)
			}

			if port, err := strconv.Atoi(name[1:]); err != nil || port <= 0 {
				return nil, errors.Errorf("management port %q name must end with a positive integer", name)
			}

			if !strings.HasPrefix(port.NOSName, ManagementPortNOSNamePrefix) {
				return nil, errors.Errorf("management port %q NOS name must start with Management", name)
			}

			if port, err := strconv.Atoi(port.NOSName[len(ManagementPortNOSNamePrefix):]); err != nil || port < 0 {
				return nil, errors.Errorf("management port %q NOS name must end with a positive integer", name)
			}

			if port.OniePortName == "" {
				return nil, errors.Errorf("management port %q must have an ONIE port name", name)
			}

			if !strings.HasPrefix(port.OniePortName, ONIEPortNamePrefix) {
				return nil, errors.Errorf("management port %q ONIE port name must start with eth", name)
			}

			if port, err := strconv.Atoi(port.OniePortName[len(ONIEPortNamePrefix):]); err != nil || port < 0 {
				return nil, errors.Errorf("management port %q ONIE port name must end with a zero or positive integer", name)
			}

			if port.Group != "" {
				return nil, errors.Errorf("management port %q must not have a group", name)
			}

			if port.Profile != "" {
				return nil, errors.Errorf("management port %q must not have a profile", name)
			}

			if port.BaseNOSName != "" {
				return nil, errors.Errorf("management port %q must not have a base NOS name", name)
			}

			continue
		}

		if !strings.HasPrefix(name, DataPortPrefix) {
			return nil, errors.Errorf("data port %q must start with E", name)
		}

		portParts := strings.Split(name[len(DataPortPrefix):], "/")
		if len(portParts) != 2 {
			return nil, errors.Errorf("data port %q name must have two segments separated by a slash (e.g. asic/port)", name)
		}
		if asic, err := strconv.Atoi(portParts[0]); err != nil || portParts[0] == "" || asic <= 0 {
			return nil, errors.Errorf("data port %q name must contain a positive integer as the first segment (e.g. asic)", name)
		}
		if port, err := strconv.Atoi(portParts[1]); err != nil || portParts[1] == "" || port <= 0 {
			return nil, errors.Errorf("data port %q name must contain a positive integer as the second segment (port)", name)
		}

		if port.Label == "" {
			return nil, errors.Errorf("port %q must have a label", name)
		}

		if _, ok := labels[port.Label]; ok {
			return nil, errors.Errorf("port %q label %q is duplicated", name, port.Label)
		}
		labels[port.Label] = true

		if port.Profile == "" && port.Group == "" {
			return nil, errors.Errorf("port %q must have a profile or group", name)
		}

		if port.Profile != "" && port.Group != "" {
			return nil, errors.Errorf("port %q must have either a profile or group, not both", name)
		}

		isBreakout := false
		if port.Profile != "" {
			profile, exists := sp.Spec.PortProfiles[port.Profile]
			if !exists {
				return nil, errors.Errorf("port %q references non-existent profile %q", name, port.Profile)
			}

			profiles[port.Profile] = true

			if profile.Breakout != nil {
				isBreakout = true

				if port.NOSName == "" {
					return nil, errors.Errorf("breakout port %q must have a NOS name", name)
				}

				if !strings.HasPrefix(port.NOSName, BreakoutNOSNamePrefix) {
					return nil, errors.Errorf("breakout port %q NOS name must start with %s", name, BreakoutNOSNamePrefix)
				}

				if _, err := strconv.Atoi(port.NOSName[len(BreakoutNOSNamePrefix):]); err != nil {
					return nil, errors.Errorf("breakout port %q NOS name must end with a positive integer", name)
				}

				if port.BaseNOSName == "" {
					return nil, errors.Errorf("breakout port %q must have a base NOS name", name)
				}

				if !strings.HasPrefix(port.BaseNOSName, DataPortNOSNamePrefix) {
					return nil, errors.Errorf("breakout port %q base NOS name must start with %s", name, DataPortNOSNamePrefix)
				}

				if port, err := strconv.Atoi(port.BaseNOSName[len(DataPortNOSNamePrefix):]); err != nil || port < 0 {
					return nil, errors.Errorf("breakout port %q base NOS name must end with a zero or positive integer", name)
				}
			}
		}

		if !isBreakout {
			if !strings.HasPrefix(port.NOSName, DataPortNOSNamePrefix) {
				return nil, errors.Errorf("data port %q NOS name must start with %s", name, DataPortNOSNamePrefix)
			}

			if port, err := strconv.Atoi(port.NOSName[len(DataPortNOSNamePrefix):]); err != nil || port < 0 {
				return nil, errors.Errorf("data port %q NOS name must end with a zero or positive integer", name)
			}
		}

		if port.Group != "" {
			if _, ok := sp.Spec.PortGroups[port.Group]; !ok {
				return nil, errors.Errorf("port %q references non-existent group %q", name, port.Group)
			}

			groups[port.Group] = true
		}
	}

	for name, group := range sp.Spec.PortGroups {
		if _, ok := groups[name]; !ok {
			return nil, errors.Errorf("group %q is not referenced by any port", name)
		}

		if group.NOSName == "" {
			return nil, errors.Errorf("group %q must have a NOS name", name)
		}

		if nosName, err := strconv.Atoi(group.NOSName); err != nil || nosName <= 0 {
			return nil, errors.Errorf("group %q NOS name must be a positive integer", name)
		}

		if group.Profile == "" {
			return nil, errors.Errorf("group %q must have a profile", name)
		}

		if _, ok := sp.Spec.PortProfiles[group.Profile]; !ok {
			return nil, errors.Errorf("group %q references non-existent profile %q", name, group.Profile)
		}

		profiles[group.Profile] = true

		if sp.Spec.PortProfiles[group.Profile].Speed == nil {
			return nil, errors.Errorf("group %q references non-speed profile %q", name, group.Profile)
		}

		if profile := sp.Spec.PortProfiles[group.Profile]; profile.AutoNegAllowed {
			return nil, errors.Errorf("group %q profile %q must not have auto-negotiation allowed", name, group.Profile)
		}
	}

	for name, profile := range sp.Spec.PortProfiles {
		if _, ok := profiles[name]; !ok {
			return nil, errors.Errorf("profile %q is not referenced by any port or group", name)
		}

		if profile.Speed == nil && profile.Breakout == nil {
			return nil, errors.Errorf("profile %q must have a speed or breakout", name)
		}

		if profile.Speed != nil && profile.Breakout != nil {
			return nil, errors.Errorf("profile %q must have either a speed or breakout, not both", name)
		}

		if profile.AutoNegDefault && !profile.AutoNegAllowed {
			return nil, errors.Errorf("profile %q must not have default auto-negotiation enabled if it's not allowed", name)
		}

		if profile.Speed != nil {
			if profile.Speed.Default == "" {
				return nil, errors.Errorf("profile %q must have a default speed", name)
			}

			if len(profile.Speed.Supported) == 0 {
				return nil, errors.Errorf("profile %q must have supported speeds", name)
			}

			if !slices.Contains(profile.Speed.Supported, profile.Speed.Default) {
				return nil, errors.Errorf("profile %q must have default speed in supported speeds", name)
			}

			for _, speed := range profile.Speed.Supported {
				if speed == "" {
					return nil, errors.Errorf("profile %q must have non-empty speeds", name)
				}

				if err := ValidatePortSpeed(speed); err != nil {
					return nil, errors.Wrapf(err, "profile %q speed %q is invalid", name, speed)
				}
			}
		}

		if profile.Breakout != nil {
			if profile.Breakout.Default == "" {
				return nil, errors.Errorf("profile %q must have a default breakout", name)
			}

			if len(profile.Breakout.Supported) == 0 {
				return nil, errors.Errorf("profile %q must have supported breakouts", name)
			}

			if _, ok := profile.Breakout.Supported[profile.Breakout.Default]; !ok {
				return nil, errors.Errorf("profile %q must have default breakout in supported breakouts", name)
			}

			for mode, offsets := range profile.Breakout.Supported {
				if len(offsets.Offsets) == 0 {
					return nil, errors.Errorf("profile %q must have non-empty offsets for mode %q", name, mode)
				}

				if mode == "" {
					return nil, errors.Errorf("profile %q must have non-empty modes", name)
				}

				if err := ValidatePortBreakoutMode(mode); err != nil {
					return nil, errors.Wrapf(err, "profile %q breakout %q is invalid", name, mode)
				}
			}
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
		return errors.Errorf("speed %q must have a G suffix", speed)
	}

	if !allowedPortSpeeds[speed] {
		return errors.Errorf("speed %q is not allowed", speed)
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
		return errors.Errorf("mode %q must have axactly one 'x' as a separator", mode)
	}

	number := parts[0]
	speed := parts[1]

	if number == "" {
		return errors.Errorf("mode %q must have a number before 'x'", mode)
	}

	if speed == "" {
		return errors.Errorf("mode %q must have a speed after 'x'", mode)
	}

	if !allowedPortBreakoutNumbers[number] {
		return errors.Errorf("mode %q must have a valid number", mode)
	}

	return ValidatePortSpeed(speed)
}

func (sp *SwitchProfileSpec) GetAPI2NOSPortsFor(sw *SwitchSpec) (map[string]string, error) {
	if sp == nil {
		return nil, errors.Errorf("switch profile spec is nil")
	}
	if sw == nil {
		return nil, errors.Errorf("switch spec is nil")
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
					return nil, errors.Errorf("port %q base NOS name %q is invalid (no expected prefix)", portName, port.NOSName)
				}
				nosNameBase, err := strconv.Atoi(nosNameBaseStr)
				if err != nil {
					return nil, errors.Errorf("port %q base NOS name %q is invalid (suffix isn't a number)", portName, port.NOSName)
				}

				for breakoutIdx, offsetStr := range breakoutMode.Offsets {
					offset, err := strconv.Atoi(offsetStr)
					if err != nil {
						return nil, errors.Errorf("port %q NOS name %q breakout mode %q offset %q is invalid (not a number)", portName, port.NOSName, swBreakout, offsetStr)
					}

					nosName := fmt.Sprintf("%s%d", DataPortNOSNamePrefix, nosNameBase+offset)

					if breakoutIdx == 0 {
						ports[portName] = nosName
					}

					ports[fmt.Sprintf("%s/%d", portName, breakoutIdx+1)] = nosName
				}
			} else {
				return nil, errors.Errorf("port %q has a breakout %q not supported by profile %q", portName, swBreakout, port.Profile)
			}
		} else {
			ports[portName] = port.NOSName
		}
	}

	return ports, nil
}

func (sp *SwitchProfileSpec) GetNOS2APIPortsFor(sw *SwitchSpec) (map[string]string, error) {
	if sp == nil {
		return nil, errors.Errorf("switch profile spec is nil")
	}
	if sw == nil {
		return nil, errors.Errorf("switch spec is nil")
	}

	api2NOS, err := sp.GetAPI2NOSPortsFor(sw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get API to NOS ports")
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

			return nil, errors.Errorf("NOS port %q is duplicated", nosPort)
		}

		ports[nosPort] = apiPort
	}

	return ports, nil
}

func (sp *SwitchProfileSpec) GetNOS2APIBreakouts() (map[string]string, error) {
	if sp == nil {
		return nil, errors.Errorf("switch profile spec is nil")
	}

	ports := map[string]string{}
	for apiName, port := range sp.Ports {
		if port.Profile == "" {
			continue
		}

		profile, exists := sp.PortProfiles[port.Profile]
		if !exists {
			return nil, errors.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile)
		}
		if profile.Breakout == nil {
			continue
		}

		if _, exists := ports[port.NOSName]; exists {
			return nil, errors.Errorf("NOS port %q is duplicated", port.NOSName)
		}

		ports[port.NOSName] = apiName
	}

	return ports, nil
}

func (sp *SwitchProfileSpec) GetAllBreakoutNOSNames() (map[string]bool, error) {
	if sp == nil {
		return nil, errors.Errorf("switch profile spec is nil")
	}

	ports := map[string]bool{}

	for portName, port := range sp.Ports {
		if port.Profile == "" {
			continue
		}

		profile, ok := sp.PortProfiles[port.Profile]
		if !ok {
			return nil, errors.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile)
		}

		if profile.Breakout == nil {
			continue
		}

		nosNameBaseStr, cut := strings.CutPrefix(port.BaseNOSName, DataPortNOSNamePrefix)
		if !cut {
			return nil, errors.Errorf("port %q base NOS name %q is invalid (no expected prefix)", portName, port.NOSName)
		}
		nosNameBase, err := strconv.Atoi(nosNameBaseStr)
		if err != nil {
			return nil, errors.Errorf("port %q base NOS name %q is invalid (suffix isn't a number)", portName, port.NOSName)
		}

		for _, breakoutMode := range profile.Breakout.Supported {
			for _, offsetStr := range breakoutMode.Offsets {
				offset, err := strconv.Atoi(offsetStr)
				if err != nil {
					return nil, errors.Errorf("port %q NOS name %q breakout mode %q offset %q is invalid (not a number)", portName, port.NOSName, breakoutMode, offsetStr)
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
		return "", errors.Errorf("switch profile spec is nil") //nolint:goerr113
	}

	if strings.Count(portName, "/") == 2 {
		return portName, nil
	}

	port, exists := sp.Ports[portName]
	if !exists {
		return "", errors.Errorf("port %q does not exist in the switch profile", portName)
	}

	if port.Profile == "" {
		return portName, nil
	}

	profile, exists := sp.PortProfiles[port.Profile]
	if !exists {
		return "", errors.Errorf("port %q references non-existent port profile %q", portName, port.Profile)
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
		return nil, errors.Errorf("switch profile spec is nil")
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
				return nil, errors.Errorf("port %q references non-existent group %q", portName, port.Group)
			}

			profile, exists := sp.PortProfiles[gr.Profile]
			if !exists {
				return nil, errors.Errorf("group %q references non-existent profile %q", port.Group, gr.Profile)
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
		return nil, nil, errors.Errorf("switch profile spec is nil")
	}
	if sw == nil {
		return nil, nil, errors.Errorf("switch spec is nil")
	}

	allowed, def := map[string]bool{}, map[string]bool{}

	for portName, port := range sp.Ports {
		if port.Management {
			continue
		}

		if port.Profile != "" { //nolint:gocritic
			profile, exists := sp.PortProfiles[port.Profile]
			if !exists {
				return nil, nil, errors.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile)
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
					return nil, nil, errors.Errorf("port %q has a breakout %q not supported by profile %q", portName, breakout, port.Profile)
				}

				for idx := range breakoutProfile.Offsets {
					allowed[fmt.Sprintf("%s/%d", portName, idx+1)] = profile.AutoNegAllowed
					def[fmt.Sprintf("%s/%d", portName, idx+1)] = profile.AutoNegDefault
				}
			} else {
				return nil, nil, errors.Errorf("port %q profile %q has no speed or breakout", portName, port.Profile)
			}
		} else if port.Group != "" {
			return nil, nil, errors.Errorf("autoneg isn't supported for port groups: %q", portName)
		} else {
			return nil, nil, errors.Errorf("port %q must have a profile or group", portName)
		}
	}

	return allowed, def, nil
}

func (sp *SwitchProfileSpec) GetBreakoutDefaults(sw *SwitchSpec) (map[string]string, error) {
	def := map[string]string{}

	for portName, port := range sp.Ports {
		if port.Management || port.Profile == "" {
			continue
		}

		profile, exists := sp.PortProfiles[port.Profile]
		if !exists {
			return nil, errors.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile)
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
		return nil, errors.Errorf("switch profile spec is nil")
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
				return nil, errors.Errorf("port %q references non-existent profile %q", port.NOSName, port.Profile)
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
					return nil, errors.Errorf("port %q has a breakout %q not supported by profile %q", portName, b, port.Profile)
				}
			} else if profile.Speed != nil {
				ports[portName] = true
			} else {
				return nil, errors.Errorf("port %q references profile %q without speed or breakout", port.NOSName, port.Profile)
			}
		} else if port.Group != "" {
			ports[portName] = true
		} else {
			return nil, errors.Errorf("port %q must have a profile or group", port.NOSName)
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
				return "", errors.Errorf("port %q references non-existent group %q", port.NOSName, port.Group)
			}

			profile = group.Profile
		}

		if profile == "" {
			return "", errors.Errorf("port %q must have a profile or group", port.NOSName)
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
