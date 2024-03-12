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
