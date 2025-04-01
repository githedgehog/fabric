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
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var DellS5248FON = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: "dell-s5248f-on",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName: "Dell S5248F-ON",
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			VXLAN:         true,
			ACLs:          true,
		},
		NOSType:  meta.NOSTypeSONiCBCMBase,
		Platform: "x86_64-dellemc_s5248f_c3538-r0",
		Config:   wiringapi.SwitchProfileConfig{},
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "Management0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "Ethernet0", Label: "1", Group: "1"},
			"E1/2":  {NOSName: "Ethernet1", Label: "2", Group: "1"},
			"E1/3":  {NOSName: "Ethernet2", Label: "3", Group: "1"},
			"E1/4":  {NOSName: "Ethernet3", Label: "4", Group: "1"},
			"E1/5":  {NOSName: "Ethernet4", Label: "5", Group: "2"},
			"E1/6":  {NOSName: "Ethernet5", Label: "6", Group: "2"},
			"E1/7":  {NOSName: "Ethernet6", Label: "7", Group: "2"},
			"E1/8":  {NOSName: "Ethernet7", Label: "8", Group: "2"},
			"E1/9":  {NOSName: "Ethernet8", Label: "9", Group: "3"},
			"E1/10": {NOSName: "Ethernet9", Label: "10", Group: "3"},
			"E1/11": {NOSName: "Ethernet10", Label: "11", Group: "3"},
			"E1/12": {NOSName: "Ethernet11", Label: "12", Group: "3"},
			"E1/13": {NOSName: "Ethernet12", Label: "13", Group: "4"},
			"E1/14": {NOSName: "Ethernet13", Label: "14", Group: "4"},
			"E1/15": {NOSName: "Ethernet14", Label: "15", Group: "4"},
			"E1/16": {NOSName: "Ethernet15", Label: "16", Group: "4"},
			"E1/17": {NOSName: "Ethernet16", Label: "17", Group: "5"},
			"E1/18": {NOSName: "Ethernet17", Label: "18", Group: "5"},
			"E1/19": {NOSName: "Ethernet18", Label: "19", Group: "5"},
			"E1/20": {NOSName: "Ethernet19", Label: "20", Group: "5"},
			"E1/21": {NOSName: "Ethernet20", Label: "21", Group: "6"},
			"E1/22": {NOSName: "Ethernet21", Label: "22", Group: "6"},
			"E1/23": {NOSName: "Ethernet22", Label: "23", Group: "6"},
			"E1/24": {NOSName: "Ethernet23", Label: "24", Group: "6"},
			"E1/25": {NOSName: "Ethernet24", Label: "25", Group: "7"},
			"E1/26": {NOSName: "Ethernet25", Label: "26", Group: "7"},
			"E1/27": {NOSName: "Ethernet26", Label: "27", Group: "7"},
			"E1/28": {NOSName: "Ethernet27", Label: "28", Group: "7"},
			"E1/29": {NOSName: "Ethernet28", Label: "29", Group: "8"},
			"E1/30": {NOSName: "Ethernet29", Label: "30", Group: "8"},
			"E1/31": {NOSName: "Ethernet30", Label: "31", Group: "8"},
			"E1/32": {NOSName: "Ethernet31", Label: "32", Group: "8"},
			"E1/33": {NOSName: "Ethernet32", Label: "33", Group: "9"},
			"E1/34": {NOSName: "Ethernet33", Label: "34", Group: "9"},
			"E1/35": {NOSName: "Ethernet34", Label: "35", Group: "9"},
			"E1/36": {NOSName: "Ethernet35", Label: "36", Group: "9"},
			"E1/37": {NOSName: "Ethernet36", Label: "37", Group: "10"},
			"E1/38": {NOSName: "Ethernet37", Label: "38", Group: "10"},
			"E1/39": {NOSName: "Ethernet38", Label: "39", Group: "10"},
			"E1/40": {NOSName: "Ethernet39", Label: "40", Group: "10"},
			"E1/41": {NOSName: "Ethernet40", Label: "41", Group: "11"},
			"E1/42": {NOSName: "Ethernet41", Label: "42", Group: "11"},
			"E1/43": {NOSName: "Ethernet42", Label: "43", Group: "11"},
			"E1/44": {NOSName: "Ethernet43", Label: "44", Group: "11"},
			"E1/45": {NOSName: "Ethernet44", Label: "45", Group: "12"},
			"E1/46": {NOSName: "Ethernet45", Label: "46", Group: "12"},
			"E1/47": {NOSName: "Ethernet46", Label: "47", Group: "12"},
			"E1/48": {NOSName: "Ethernet47", Label: "48", Group: "12"},
			"E1/49": {NOSName: "1/49", BaseNOSName: "Ethernet48", Label: "49", Profile: "QSFP28-100G"},
			"E1/50": {NOSName: "1/50", BaseNOSName: "Ethernet52", Label: "50", Profile: "QSFP28-100G"},
			"E1/51": {NOSName: "1/51", BaseNOSName: "Ethernet56", Label: "51", Profile: "QSFP28-100G"},
			"E1/52": {NOSName: "1/52", BaseNOSName: "Ethernet60", Label: "52", Profile: "QSFP28-100G"},
			"E1/53": {NOSName: "1/53", BaseNOSName: "Ethernet64", Label: "53", Profile: "QSFP28-100G"},
			"E1/54": {NOSName: "1/54", BaseNOSName: "Ethernet68", Label: "54", Profile: "QSFP28-100G"},
			"E1/55": {NOSName: "1/55", BaseNOSName: "Ethernet72", Label: "55", Profile: "QSFP28-100G"},
			"E1/56": {NOSName: "1/56", BaseNOSName: "Ethernet76", Label: "56", Profile: "QSFP28-100G"},
		},
		PortGroups: map[string]wiringapi.SwitchProfilePortGroup{
			"1": {
				NOSName: "1",
				Profile: "SFP28-25G",
			},
			"2": {
				NOSName: "2",
				Profile: "SFP28-25G",
			},
			"3": {
				NOSName: "3",
				Profile: "SFP28-25G",
			},
			"4": {
				NOSName: "4",
				Profile: "SFP28-25G",
			},
			"5": {
				NOSName: "5",
				Profile: "SFP28-25G",
			},
			"6": {
				NOSName: "6",
				Profile: "SFP28-25G",
			},
			"7": {
				NOSName: "7",
				Profile: "SFP28-25G",
			},
			"8": {
				NOSName: "8",
				Profile: "SFP28-25G",
			},
			"9": {
				NOSName: "9",
				Profile: "SFP28-25G",
			},
			"10": {
				NOSName: "10",
				Profile: "SFP28-25G",
			},
			"11": {
				NOSName: "11",
				Profile: "SFP28-25G",
			},
			"12": {
				NOSName: "12",
				Profile: "SFP28-25G",
			},
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP28-25G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "25G",
					Supported: []string{"10G", "25G"},
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
