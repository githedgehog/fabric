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

const DefaultIPv4Namespace = "default"

var (
	LabelPrefix          = "fabric.githedgehog.com/"
	LabelVPC             = LabelName("vpc")
	LabelVPC1            = LabelName("vpc1")
	LabelVPC2            = LabelName("vpc2")
	LabelSubnet          = LabelName("subnet")
	LabelIPv4NS          = LabelName("ipv4ns")
	LabelVLANNS          = LabelName("vlanns")
	LabelExternal        = LabelName("external")
	LabelNativeVLAN      = LabelName("nativevlan")
	LabelNativeVLANValue = "true"
	ListLabelValue       = "true"
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

func ListLabelVPC(vpcName string) string {
	return ListLabel("vpc", vpcName)
}
