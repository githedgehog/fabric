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

package switchprofile

import (
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var profileDellS5232FON = wiringapi.SwitchProfile{
	ObjectMeta: metav1.ObjectMeta{
		Name: "dell-s5232f-on",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName: "Dell S5232F-ON",
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			VXLAN:         true,
			ACLs:          true,
		},
		Config: wiringapi.SwitchProfileConfig{},
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "Management0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "Ethernet0", Label: "1", Profile: "QSFP28-100G"},
			"E1/2":  {NOSName: "Ethernet4", Label: "2", Profile: "QSFP28-100G"},
			"E1/3":  {NOSName: "Ethernet8", Label: "3", Profile: "QSFP28-100G"},
			"E1/4":  {NOSName: "Ethernet12", Label: "4", Profile: "QSFP28-100G"},
			"E1/5":  {NOSName: "Ethernet16", Label: "5", Profile: "QSFP28-100G"},
			"E1/6":  {NOSName: "Ethernet20", Label: "6", Profile: "QSFP28-100G"},
			"E1/7":  {NOSName: "Ethernet24", Label: "7", Profile: "QSFP28-100G"},
			"E1/8":  {NOSName: "Ethernet28", Label: "8", Profile: "QSFP28-100G"},
			"E1/9":  {NOSName: "Ethernet32", Label: "9", Profile: "QSFP28-100G"},
			"E1/10": {NOSName: "Ethernet36", Label: "10", Profile: "QSFP28-100G"},
			"E1/11": {NOSName: "Ethernet40", Label: "11", Profile: "QSFP28-100G"},
			"E1/12": {NOSName: "Ethernet44", Label: "12", Profile: "QSFP28-100G"},
			"E1/13": {NOSName: "Ethernet48", Label: "13", Profile: "QSFP28-100G"},
			"E1/14": {NOSName: "Ethernet52", Label: "14", Profile: "QSFP28-100G"},
			"E1/15": {NOSName: "Ethernet56", Label: "15", Profile: "QSFP28-100G"},
			"E1/16": {NOSName: "Ethernet60", Label: "16", Profile: "QSFP28-100G"},
			"E1/17": {NOSName: "Ethernet64", Label: "17", Profile: "QSFP28-100G"},
			"E1/18": {NOSName: "Ethernet68", Label: "18", Profile: "QSFP28-100G"},
			"E1/19": {NOSName: "Ethernet72", Label: "19", Profile: "QSFP28-100G"},
			"E1/20": {NOSName: "Ethernet76", Label: "20", Profile: "QSFP28-100G"},
			"E1/21": {NOSName: "Ethernet80", Label: "21", Profile: "QSFP28-100G"},
			"E1/22": {NOSName: "Ethernet84", Label: "22", Profile: "QSFP28-100G"},
			"E1/23": {NOSName: "Ethernet88", Label: "23", Profile: "QSFP28-100G"},
			"E1/24": {NOSName: "Ethernet92", Label: "24", Profile: "QSFP28-100G"},
			"E1/25": {NOSName: "Ethernet96", Label: "25", Profile: "QSFP28-100G"},
			"E1/26": {NOSName: "Ethernet100", Label: "26", Profile: "QSFP28-100G"},
			"E1/27": {NOSName: "Ethernet104", Label: "27", Profile: "QSFP28-100G"},
			"E1/28": {NOSName: "Ethernet108", Label: "28", Profile: "QSFP28-100G"},
			"E1/29": {NOSName: "Ethernet112", Label: "29", Profile: "QSFP28-100G"},
			"E1/30": {NOSName: "Ethernet116", Label: "30", Profile: "QSFP28-100G"},
			"E1/31": {NOSName: "Ethernet120", Label: "31", Profile: "QSFP28-100G"},
			"E1/32": {NOSName: "Ethernet124", Label: "32", Profile: "QSFP28-100G"},
			"E1/33": {NOSName: "Ethernet128", Label: "33", Profile: "SFP-10G"},
			"E1/34": {NOSName: "Ethernet129", Label: "34", Profile: "SFP-10G"},
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP-10G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "10G",
					Supported: []string{"1G", "10G"},
				},
			},
			"QSFP28-100G": {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x100G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x100G": {Offsets: []string{"0"}},
						"1x40G":  {Offsets: []string{"0"}},
						"2x50G":  {Offsets: []string{"0", "2"}},
						"1x50G":  {Offsets: []string{"0"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
						"1x25G":  {Offsets: []string{"0"}},
						"1x10G":  {Offsets: []string{"0"}},
					},
				},
			},
		},
	},
}
