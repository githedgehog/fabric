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

// TODO do we need to create user inputable AgentAction CRD with version override, reinstall and reboot requests?

// AgentSpec defines the desired state of Agent
type AgentSpec struct {
	Version       AgentVersion            `json:"version,omitempty"`
	ControlVIP    string                  `json:"controlVIP,omitempty"`
	Users         []UserCreds             `json:"users,omitempty"`
	Switch        wiringapi.SwitchSpec    `json:"switch,omitempty"`
	Connections   []ConnectionInfo        `json:"connections,omitempty"`
	VPCs          []vpcapi.VPCSummarySpec `json:"vpcs,omitempty"`
	VPCVLANRange  string                  `json:"vpcVLANRange,omitempty"`
	PortChannels  map[string]uint16       `json:"portChannels,omitempty"`
	Reinstall     string                  `json:"reinstall,omitempty"` // set to InstallID to reinstall NOS
	Reboot        string                  `json:"reboot,omitempty"`    // set to RunID to reboot
	StatusUpdates []ApplyStatusUpdate     `json:"statusUpdates,omitempty"`
}

type AgentVersion struct {
	Default  string `json:"default,omitempty"`
	Override string `json:"override,omitempty"`
	Repo     string `json:"repo,omitempty"`
	CA       string `json:"ca,omitempty"`
}

type UserCreds struct {
	Name     string   `json:"name,omitempty"`
	Password string   `json:"password,omitempty"`
	Role     string   `json:"role,omitempty"`
	SSHKeys  []string `json:"sshKeys,omitempty"`
}

type ConnectionInfo struct {
	Name string                   `json:"name,omitempty"`
	Spec wiringapi.ConnectionSpec `json:"spec,omitempty"`
}

type ApplyStatusUpdate struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Generation int64  `json:"generation,omitempty"`
}

// TODO replace flat attempl/apply in agent status with ApplyStatus

// AgentStatus defines the observed state of Agent
type AgentStatus struct {
	Version         string              `json:"version,omitempty"`
	InstallID       string              `json:"installID,omitempty"`
	RunID           string              `json:"runID,omitempty"`
	LastHeartbeat   metav1.Time         `json:"lastHeartbeat,omitempty"`
	LastAttemptTime metav1.Time         `json:"lastAttemptTime,omitempty"`
	LastAttemptGen  int64               `json:"lastAttemptGen,omitempty"`
	LastAppliedTime metav1.Time         `json:"lastAppliedTime,omitempty"`
	LastAppliedGen  int64               `json:"lastAppliedGen,omitempty"`
	NOSInfo         NOSInfo             `json:"nosInfo,omitempty"`
	StatusUpdates   []ApplyStatusUpdate `json:"statusUpdates,omitempty"`
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric,shortName=ag
// +kubebuilder:printcolumn:name="HWSKU",type=string,JSONPath=`.status.nosInfo.hwskuVersion`,priority=0
// +kubebuilder:printcolumn:name="ASIC",type=string,JSONPath=`.status.nosInfo.asicVersion`,priority=0
// +kubebuilder:printcolumn:name="Heartbeat",type=date,JSONPath=`.status.lastHeartbeat`,priority=1
// +kubebuilder:printcolumn:name="Applied",type=date,JSONPath=`.status.lastAppliedTime`,priority=0
// +kubebuilder:printcolumn:name="AppliedG",type=string,JSONPath=`.status.lastAppliedGen`,priority=0
// +kubebuilder:printcolumn:name="CurrentG",type=string,JSONPath=`.metadata.generation`,priority=0
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,priority=0
// +kubebuilder:printcolumn:name="Software",type=string,JSONPath=`.status.nosInfo.softwareVersion`,priority=1
// +kubebuilder:printcolumn:name="Attempt",type=date,JSONPath=`.status.lastAttemptTime`,priority=2
// +kubebuilder:printcolumn:name="AttemptG",type=string,JSONPath=`.status.lastAttemptGen`,priority=2
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
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
