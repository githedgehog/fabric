/*
Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha2

import (
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AgentSpec defines the desired state of Agent
type AgentSpec struct {
	ControlVIP     string                              `json:"controlVIP,omitempty"`
	Users          []UserCreds                         `json:"users,omitempty"`
	Switch         wiringapi.SwitchSpec                `json:"switch,omitempty"`
	Connections    map[string]wiringapi.ConnectionSpec `json:"connections,omitempty"`
	VPCs           map[string]vpcapi.VPCSpec           `json:"vpcs,omitempty"`
	VPCAttachments map[string]vpcapi.VPCAttachmentSpec `json:"vpcAttachments,omitempty"`
	VPCVLANRange   string                              `json:"vpcVLANRange,omitempty"`
}

type UserCreds struct {
	Name     string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Role     string `json:"role,omitempty"`
}

// AgentStatus defines the observed state of Agent
type AgentStatus struct {
	// TODO
	// Applied     bool        `json:"applied,omitempty"`
	// LastApplied metav1.Time `json:"lastApplied,omitempty"`

	NOSInfo NOSInfo `json:"nosInfo,omitempty"`
}

type NOSInfo struct {
	AsicVersion         string `json:"asicVersion,omitempty"`
	BuildCommit         string `json:"buildCommit,omitempty"`
	BuildDate           string `json:"buildDate,omitempty"`
	BuiltBy             string `json:"builtBy,omitempty"`
	ConfigDbVersion     string `json:"configDbVersion,omitempty"`
	DistributionVersion string `json:"distributionVersion,omitempty"`
	HardwareVersion     string `json:"hardwareVersion,omitempty"`
	HwskuVersion        string `json:"hwskuVersion,omitempty"`
	KernelVersion       string `json:"kernelVersion,omitempty"`
	MfgName             string `json:"mfgName,omitempty"`
	PlatformName        string `json:"platformName,omitempty"`
	ProductDescription  string `json:"productDescription,omitempty"`
	ProductVersion      string `json:"productVersion,omitempty"`
	SerialNumber        string `json:"serialNumber,omitempty"`
	SoftwareVersion     string `json:"softwareVersion,omitempty"`
	UpTime              string `json:"upTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Agent is the Schema for the agents API
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AgentList contains a list of Agent
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
