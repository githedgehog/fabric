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

var CelesticaDS4101 = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: "celestica-ds4101",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Celestica DS4101",
		OtherNames:    []string{"Celestica Greystone"},
		SwitchSilicon: SiliconBroadcomTH4G,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: false,
			VXLAN:         false,
			ACLs:          true,
		},
		NOSType:  meta.NOSTypeSONiCBCMBase,
		Platform: "x86_64-cel_ds4101-r0",
		Config:   wiringapi.SwitchProfileConfig{},
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "Management0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "1/1", BaseNOSName: "Ethernet0", Label: "1", Profile: "OSFP-2x400G"},
			"E1/2":  {NOSName: "1/2", BaseNOSName: "Ethernet8", Label: "2", Profile: "OSFP-2x400G"},
			"E1/3":  {NOSName: "1/3", BaseNOSName: "Ethernet16", Label: "3", Profile: "OSFP-2x400G"},
			"E1/4":  {NOSName: "1/4", BaseNOSName: "Ethernet24", Label: "4", Profile: "OSFP-2x400G"},
			"E1/5":  {NOSName: "1/5", BaseNOSName: "Ethernet32", Label: "5", Profile: "OSFP-2x400G"},
			"E1/6":  {NOSName: "1/6", BaseNOSName: "Ethernet40", Label: "6", Profile: "OSFP-2x400G"},
			"E1/7":  {NOSName: "1/7", BaseNOSName: "Ethernet48", Label: "7", Profile: "OSFP-2x400G"},
			"E1/8":  {NOSName: "1/8", BaseNOSName: "Ethernet56", Label: "8", Profile: "OSFP-2x400G"},
			"E1/9":  {NOSName: "1/9", BaseNOSName: "Ethernet64", Label: "9", Profile: "OSFP-2x400G"},
			"E1/10": {NOSName: "1/10", BaseNOSName: "Ethernet72", Label: "10", Profile: "OSFP-2x400G"},
			"E1/11": {NOSName: "1/11", BaseNOSName: "Ethernet80", Label: "11", Profile: "OSFP-2x400G"},
			"E1/12": {NOSName: "1/12", BaseNOSName: "Ethernet88", Label: "12", Profile: "OSFP-2x400G"},
			"E1/13": {NOSName: "1/13", BaseNOSName: "Ethernet96", Label: "13", Profile: "OSFP-2x400G"},
			"E1/14": {NOSName: "1/14", BaseNOSName: "Ethernet104", Label: "14", Profile: "OSFP-2x400G"},
			"E1/15": {NOSName: "1/15", BaseNOSName: "Ethernet112", Label: "15", Profile: "OSFP-2x400G"},
			"E1/16": {NOSName: "1/16", BaseNOSName: "Ethernet120", Label: "16", Profile: "OSFP-2x400G"},
			"E1/17": {NOSName: "1/17", BaseNOSName: "Ethernet128", Label: "17", Profile: "OSFP-2x400G"},
			"E1/18": {NOSName: "1/18", BaseNOSName: "Ethernet136", Label: "18", Profile: "OSFP-2x400G"},
			"E1/19": {NOSName: "1/19", BaseNOSName: "Ethernet144", Label: "19", Profile: "OSFP-2x400G"},
			"E1/20": {NOSName: "1/20", BaseNOSName: "Ethernet152", Label: "20", Profile: "OSFP-2x400G"},
			"E1/21": {NOSName: "1/21", BaseNOSName: "Ethernet160", Label: "21", Profile: "OSFP-2x400G"},
			"E1/22": {NOSName: "1/22", BaseNOSName: "Ethernet168", Label: "22", Profile: "OSFP-2x400G"},
			"E1/23": {NOSName: "1/23", BaseNOSName: "Ethernet176", Label: "23", Profile: "OSFP-2x400G"},
			"E1/24": {NOSName: "1/24", BaseNOSName: "Ethernet184", Label: "24", Profile: "OSFP-2x400G"},
			"E1/25": {NOSName: "1/25", BaseNOSName: "Ethernet192", Label: "25", Profile: "OSFP-2x400G"},
			"E1/26": {NOSName: "1/26", BaseNOSName: "Ethernet200", Label: "26", Profile: "OSFP-2x400G"},
			"E1/27": {NOSName: "1/27", BaseNOSName: "Ethernet208", Label: "27", Profile: "OSFP-2x400G"},
			"E1/28": {NOSName: "1/28", BaseNOSName: "Ethernet216", Label: "28", Profile: "OSFP-2x400G"},
			"E1/29": {NOSName: "1/29", BaseNOSName: "Ethernet224", Label: "29", Profile: "OSFP-2x400G"},
			"E1/30": {NOSName: "1/30", BaseNOSName: "Ethernet232", Label: "30", Profile: "OSFP-2x400G"},
			"E1/31": {NOSName: "1/31", BaseNOSName: "Ethernet240", Label: "31", Profile: "OSFP-2x400G"},
			"E1/32": {NOSName: "1/32", BaseNOSName: "Ethernet248", Label: "32", Profile: "OSFP-2x400G"}, // 32x OSFP-2x400G
			"E1/33": {NOSName: "Ethernet256", Label: "M1", Profile: "SFP28-10G"},                        // 1x SFP28-10G
			"E1/34": {NOSName: "Ethernet257", Label: "M2", Profile: "SFP28-10G"},                        // 1x SFP28-10G
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP28-10G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "10G",
					Supported: []string{"1G", "10G"},
				},
			},
			"OSFP-2x400G": {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "2x400G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x100G": {Offsets: []string{"0"}},
						"1x200G": {Offsets: []string{"0"}},
						"1x400G": {Offsets: []string{"0"}},
						"2x40G":  {Offsets: []string{"0", "4"}},
						"2x100G": {Offsets: []string{"0", "4"}},
						"2x200G": {Offsets: []string{"0", "4"}},
						"2x400G": {Offsets: []string{"0", "4"}},
						"4x50G":  {Offsets: []string{"0", "2", "4", "6"}},
						"4x100G": {Offsets: []string{"0", "2", "4", "6"}},
						"4x200G": {Offsets: []string{"0", "2", "4", "6"}},
						"8x10G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}},
						"8x25G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}},
						"8x50G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}},
						"8x100G": {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}},
					},
				},
			},
		},
	},
}
