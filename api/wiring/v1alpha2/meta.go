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

package v1alpha2

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PortNameSeparator    = "/"
	DefaultVLANNamespace = "default"
)

var (
	// TODO should it be same as group name? or just standard prefix for all APIs?
	LabelPrefix               = "fabric.githedgehog.com/"
	LabelSwitch               = LabelName("switch")
	LabelProfile              = LabelName("profile")
	LabelServer               = LabelName("server")
	LabelServerType           = LabelName("server-type")
	LabelConnection           = LabelName("connection")
	LabelConnectionType       = LabelName("connection-type")
	LabelSwitches             = LabelName("switches")
	LabelServers              = LabelName("servers")
	LabelVPC                  = LabelName("vpc")
	ListLabelValue            = "true"
	ConnectionLabelTypeServer = "server"
	ConnectionLabelTypeSwitch = "switch"
	AnnotationPorts           = LabelName("ports")
)

func LabelName(name string) string {
	return LabelPrefix + name
}

func ListLabelPrefix(listType string) string {
	return listType + "." + LabelPrefix
}

func ListLabel(listType, val string) string {
	return ListLabelPrefix(listType) + val
}

func ListLabelServer(serverName string) string {
	return ListLabel(ConnectionLabelTypeServer, serverName)
}

func ListLabelSwitch(switchName string) string {
	return ListLabel(ConnectionLabelTypeSwitch, switchName)
}

func ListLabelVLANNamespace(vlanNamespace string) string {
	return ListLabel("vlanns", vlanNamespace)
}

func ListLabelSwitchGroup(groupName string) string {
	return ListLabel("switchgroup", groupName)
}

func MatchingLabelsForListLabelServer(serverName string) client.MatchingLabels {
	return client.MatchingLabels{
		ListLabel(ConnectionLabelTypeServer, serverName): ListLabelValue,
	}
}

func MatchingLabelsForListLabelSwitch(switchName string) client.MatchingLabels {
	return client.MatchingLabels{
		ListLabel(ConnectionLabelTypeSwitch, switchName): ListLabelValue,
	}
}

func MatchingLabelsForSwitchGroup(groupName string) client.MatchingLabels {
	return client.MatchingLabels{
		ListLabelSwitchGroup(groupName): ListLabelValue,
	}
}

type ApplyStatus struct {
	Generation int64            `json:"gen,omitempty"`
	Time       metav1.Time      `json:"time,omitempty"`
	Detailed   map[string]int64 `json:"detailed,omitempty"`
}

func CleanupFabricLabels(labels map[string]string) {
	for key := range labels {
		if strings.Contains(key, LabelPrefix) {
			delete(labels, key)
		}
	}
}
