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

const (
	VirtualEdgeConfigAnnotation = "virtual-edge.hhfab.fabric.githedgehog.com/external-cfg"
)

// +kubebuilder:skip
type VirtualEdgeConfig struct {
	ASN          string `json:"ASN"`
	VRF          string `json:"VRF"`
	CommunityIn  string `json:"CommunityIn"`
	CommunityOut string `json:"CommunityOut"`
	NeighborIP   string `json:"NeighborIP"`
	IfName       string `json:"ifName"`
	IfVlan       string `json:"ifVlan"`
	IfIP         string `json:"ifIP"`
}
