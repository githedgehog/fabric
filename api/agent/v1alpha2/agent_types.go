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

// AgentSpec defines the desired state of the Agent and includes all relevant information required to fully configure
// the switch and manage its lifecycle. It is not intended to be used directly by users.
type AgentSpec struct {
	Role                     wiringapi.SwitchRole                     `json:"role,omitempty"`
	Description              string                                   `json:"description,omitempty"`
	Config                   AgentSpecConfig                          `json:"config,omitempty"`
	Version                  AgentVersion                             `json:"version,omitempty"`
	Users                    []UserCreds                              `json:"users,omitempty"`
	Switch                   wiringapi.SwitchSpec                     `json:"switch,omitempty"`
	Switches                 map[string]wiringapi.SwitchSpec          `json:"switches,omitempty"`
	Connections              map[string]wiringapi.ConnectionSpec      `json:"connections,omitempty"`
	VPCs                     map[string]vpcapi.VPCSpec                `json:"vpcs,omitempty"`
	VPCAttachments           map[string]vpcapi.VPCAttachmentSpec      `json:"vpcAttachments,omitempty"`
	VPCPeerings              map[string]vpcapi.VPCPeeringSpec         `json:"vpcPeers,omitempty"`
	VPCLoopbackLinks         map[string]string                        `json:"vpcLoopbackLinks,omitempty"`
	VPCLoopbackVLANs         map[string]uint16                        `json:"vpcLoopbackVLANs,omitempty"`
	IPv4Namespaces           map[string]vpcapi.IPv4NamespaceSpec      `json:"ipv4Namespaces,omitempty"`
	VLANNamespaces           map[string]wiringapi.VLANNamespaceSpec   `json:"vlanNamespaces,omitempty"`
	Externals                map[string]vpcapi.ExternalSpec           `json:"externals,omitempty"`
	ExternalAttachments      map[string]vpcapi.ExternalAttachmentSpec `json:"externalAttachments,omitempty"`
	ExternalPeerings         map[string]vpcapi.ExternalPeeringSpec    `json:"externalPeerings,omitempty"`
	ConfiguredVPCSubnets     map[string]bool                          `json:"configuredVPCSubnets,omitempty"`
	MCLAGAttachedVPCs        map[string]bool                          `json:"mclagAttachedVPCs,omitempty"`
	VNIs                     map[string]uint32                        `json:"vnis,omitempty"`
	IRBVLANs                 map[string]uint16                        `json:"irbVLANs,omitempty"`
	ConnSystemIDs            map[string]uint32                        `json:"connSystemIDs,omitempty"`
	ExternalPeeringPrefixIDs map[string]uint32                        `json:"externalPeeringPrefixIDs,omitempty"`
	ExternalSeqs             map[string]uint16                        `json:"externalSeqs,omitempty"`
	PortChannels             map[string]uint16                        `json:"portChannels,omitempty"`
	Reinstall                string                                   `json:"reinstall,omitempty"`  // set to InstallID to reinstall NOS
	Reboot                   string                                   `json:"reboot,omitempty"`     // set to RunID to reboot
	PowerReset               string                                   `json:"powerReset,omitempty"` // set to RunID to power reset

	// TODO impl
	StatusUpdates []ApplyStatusUpdate `json:"statusUpdates,omitempty"`
}

type AgentSpecConfig struct {
	ControlVIP            string                        `json:"controlVIP,omitempty"`
	VPCPeeringDisabled    bool                          `json:"vpcPeeringDisabled,omitempty"`
	CollapsedCore         *AgentSpecConfigCollapsedCore `json:"collapsedCore,omitempty"`
	SpineLeaf             *AgentSpecConfigSpineLeaf     `json:"spineLeaf,omitempty"`
	BaseVPCCommunity      string                        `json:"baseVPCCommunity,omitempty"`
	VPCLoopbackSubnet     string                        `json:"vpcLoopbackSubnet,omitempty"`
	FabricMTU             uint16                        `json:"fabricMTU,omitempty"`
	ServerFacingMTUOffset uint16                        `json:"serverFacingMTUOffset,omitempty"`
}

type AgentSpecConfigCollapsedCore struct{}

type AgentSpecConfigSpineLeaf struct{}

func (a *Agent) IsCollapsedCore() bool {
	return a != nil && a.Spec.Config.CollapsedCore != nil
}

func (a *Agent) IsSpineLeaf() bool {
	return a != nil && a.Spec.Config.SpineLeaf != nil
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

type ApplyStatusUpdate struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Generation int64  `json:"generation,omitempty"`
}

// TODO replace flat attempl/apply in agent status with ApplyStatus

// AgentStatus defines the observed state of the agent running on a specific switch and includes information about the
// switch itself as well as the state of the agent and applied configuration.
type AgentStatus struct {
	// Current running agent version
	Version string `json:"version,omitempty"`
	// ID of the agent installation, used to track NOS re-installs
	InstallID string `json:"installID,omitempty"`
	// ID of the agent run, used to track NOS reboots
	RunID string `json:"runID,omitempty"`
	// Time of the last heartbeat from the agent
	LastHeartbeat metav1.Time `json:"lastHeartbeat,omitempty"`
	// Time of the last attempt to apply configuration
	LastAttemptTime metav1.Time `json:"lastAttemptTime,omitempty"`
	// Generation of the last attempt to apply configuration
	LastAttemptGen int64 `json:"lastAttemptGen,omitempty"`
	// Time of the last successful configuration application
	LastAppliedTime metav1.Time `json:"lastAppliedTime,omitempty"`
	// Generation of the last successful configuration application
	LastAppliedGen int64 `json:"lastAppliedGen,omitempty"`
	// Information about the switch and NOS
	NOSInfo NOSInfo `json:"nosInfo,omitempty"`
	// Status updates from the agent
	StatusUpdates []ApplyStatusUpdate `json:"statusUpdates,omitempty"`
	// Conditions of the agent, includes readiness marker for use with kubectl wait
	Conditions []metav1.Condition `json:"conditions"`
}

// NOSInfo contains information about the switch and NOS received from the switch itself by the agent
type NOSInfo struct {
	// ASIC name, such as "broadcom" or "vs"
	AsicVersion string `json:"asicVersion,omitempty"`
	// NOS build commit
	BuildCommit string `json:"buildCommit,omitempty"`
	// NOS build date
	BuildDate string `json:"buildDate,omitempty"`
	// NOS build user
	BuiltBy string `json:"builtBy,omitempty"`
	// NOS config DB version, such as "version_4_2_1"
	ConfigDbVersion string `json:"configDbVersion,omitempty"`
	// Distribution version, such as "Debian 10.13"
	DistributionVersion string `json:"distributionVersion,omitempty"`
	// Hardware version, such as "X01"
	HardwareVersion string `json:"hardwareVersion,omitempty"`
	// Hwsku version, such as "DellEMC-S5248f-P-25G-DPB"
	HwskuVersion string `json:"hwskuVersion,omitempty"`
	// Kernel version, such as "5.10.0-21-amd64"
	KernelVersion string `json:"kernelVersion,omitempty"`
	// Manufacturer name, such as "Dell EMC"
	MfgName string `json:"mfgName,omitempty"`
	// Platform name, such as "x86_64-dellemc_s5248f_c3538-r0"
	PlatformName string `json:"platformName,omitempty"`
	// NOS product description, such as "Enterprise SONiC Distribution by Broadcom - Enterprise Base package"
	ProductDescription string `json:"productDescription,omitempty"`
	// NOS product version, empty for Broadcom SONiC
	ProductVersion string `json:"productVersion,omitempty"`
	// Switch serial number
	SerialNumber string `json:"serialNumber,omitempty"`
	// NOS software version, such as "4.2.0-Enterprise_Base"
	SoftwareVersion string `json:"softwareVersion,omitempty"`
	// Switch uptime, such as "21:21:27 up 1 day, 23:26, 0 users, load average: 1.92, 1.99, 2.00 "
	UpTime string `json:"upTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric,shortName=ag
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`,priority=0
// +kubebuilder:printcolumn:name="Descr",type=string,JSONPath=`.spec.description`,priority=0
// +kubebuilder:printcolumn:name="HWSKU",type=string,JSONPath=`.status.nosInfo.hwskuVersion`,priority=1
// +kubebuilder:printcolumn:name="ASIC",type=string,JSONPath=`.status.nosInfo.asicVersion`,priority=1
// +kubebuilder:printcolumn:name="Heartbeat",type=date,JSONPath=`.status.lastHeartbeat`,priority=1
// +kubebuilder:printcolumn:name="Applied",type=date,JSONPath=`.status.lastAppliedTime`,priority=0
// +kubebuilder:printcolumn:name="AppliedG",type=string,JSONPath=`.status.lastAppliedGen`,priority=0
// +kubebuilder:printcolumn:name="CurrentG",type=string,JSONPath=`.metadata.generation`,priority=0
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,priority=0
// +kubebuilder:printcolumn:name="Software",type=string,JSONPath=`.status.nosInfo.softwareVersion`,priority=1
// +kubebuilder:printcolumn:name="Attempt",type=date,JSONPath=`.status.lastAttemptTime`,priority=2
// +kubebuilder:printcolumn:name="AttemptG",type=string,JSONPath=`.status.lastAttemptGen`,priority=2
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
// Agent is an internal API object used by the controller to pass all relevant information to the agent running on a
// specific switch in order to fully configure it and manage its lifecycle. It is not intended to be used directly by
// users. Spec of the object isn't user-editable, it is managed by the controller. Status of the object is updated by
// the agent and is used by the controller to track the state of the agent and the switch it is running on. Name of the
// Agent object is the same as the name of the switch it is running on and it's created in the same namespace as the
// Switch object.
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the Agent
	Spec AgentSpec `json:"spec,omitempty"`
	// Status is the observed state of the Agent
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

// TODO replace with real profile, temp hack
func (s *AgentSpec) IsVS() bool {
	return s.Switch.Profile == "vs"
}
