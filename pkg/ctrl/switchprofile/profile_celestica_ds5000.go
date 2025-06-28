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

var CelesticaDS5000 = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: "celestica-ds5000",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Celestica DS5000",
		OtherNames:    []string{"Celestica Moonstone"},
		SwitchSilicon: SiliconBroadcomTH5,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          true,
			L2VNI:         false,
			L3VNI:         true,
			RoCE:          true,
			MCLAG:         false,
			ESLAG:         false,
		},
		Notes:    "Doesn't support non-L3 VPC modes due to the lack of L2VNI support.",
		NOSType:  meta.NOSTypeSONiCBCMBase,
		Platform: "x86-64-cls-ds5000-r0",
		Config:   wiringapi.SwitchProfileConfig{},
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "Management0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "1/1", BaseNOSName: "Ethernet0", Label: "1", Profile: "OSFP-800G"},
			"E1/2":  {NOSName: "1/2", BaseNOSName: "Ethernet8", Label: "2", Profile: "OSFP-800G"},
			"E1/3":  {NOSName: "1/3", BaseNOSName: "Ethernet16", Label: "3", Profile: "OSFP-800G"},
			"E1/4":  {NOSName: "1/4", BaseNOSName: "Ethernet24", Label: "4", Profile: "OSFP-800G"},
			"E1/5":  {NOSName: "1/5", BaseNOSName: "Ethernet32", Label: "5", Profile: "OSFP-800G"},
			"E1/6":  {NOSName: "1/6", BaseNOSName: "Ethernet40", Label: "6", Profile: "OSFP-800G"},
			"E1/7":  {NOSName: "1/7", BaseNOSName: "Ethernet48", Label: "7", Profile: "OSFP-800G"},
			"E1/8":  {NOSName: "1/8", BaseNOSName: "Ethernet56", Label: "8", Profile: "OSFP-800G"},
			"E1/9":  {NOSName: "1/9", BaseNOSName: "Ethernet64", Label: "9", Profile: "OSFP-800G"},
			"E1/10": {NOSName: "1/10", BaseNOSName: "Ethernet72", Label: "10", Profile: "OSFP-800G"},
			"E1/11": {NOSName: "1/11", BaseNOSName: "Ethernet80", Label: "11", Profile: "OSFP-800G"},
			"E1/12": {NOSName: "1/12", BaseNOSName: "Ethernet88", Label: "12", Profile: "OSFP-800G"},
			"E1/13": {NOSName: "1/13", BaseNOSName: "Ethernet96", Label: "13", Profile: "OSFP-800G"},
			"E1/14": {NOSName: "1/14", BaseNOSName: "Ethernet104", Label: "14", Profile: "OSFP-800G"},
			"E1/15": {NOSName: "1/15", BaseNOSName: "Ethernet112", Label: "15", Profile: "OSFP-800G"},
			"E1/16": {NOSName: "1/16", BaseNOSName: "Ethernet120", Label: "16", Profile: "OSFP-800G"},
			"E1/17": {NOSName: "1/17", BaseNOSName: "Ethernet128", Label: "17", Profile: "OSFP-800G"},
			"E1/18": {NOSName: "1/18", BaseNOSName: "Ethernet136", Label: "18", Profile: "OSFP-800G"},
			"E1/19": {NOSName: "1/19", BaseNOSName: "Ethernet144", Label: "19", Profile: "OSFP-800G"},
			"E1/20": {NOSName: "1/20", BaseNOSName: "Ethernet152", Label: "20", Profile: "OSFP-800G"},
			"E1/21": {NOSName: "1/21", BaseNOSName: "Ethernet160", Label: "21", Profile: "OSFP-800G"},
			"E1/22": {NOSName: "1/22", BaseNOSName: "Ethernet168", Label: "22", Profile: "OSFP-800G"},
			"E1/23": {NOSName: "1/23", BaseNOSName: "Ethernet176", Label: "23", Profile: "OSFP-800G"},
			"E1/24": {NOSName: "1/24", BaseNOSName: "Ethernet184", Label: "24", Profile: "OSFP-800G"},
			"E1/25": {NOSName: "1/25", BaseNOSName: "Ethernet192", Label: "25", Profile: "OSFP-800G"},
			"E1/26": {NOSName: "1/26", BaseNOSName: "Ethernet200", Label: "26", Profile: "OSFP-800G"},
			"E1/27": {NOSName: "1/27", BaseNOSName: "Ethernet208", Label: "27", Profile: "OSFP-800G"},
			"E1/28": {NOSName: "1/28", BaseNOSName: "Ethernet216", Label: "28", Profile: "OSFP-800G"},
			"E1/29": {NOSName: "1/29", BaseNOSName: "Ethernet224", Label: "29", Profile: "OSFP-800G"},
			"E1/30": {NOSName: "1/30", BaseNOSName: "Ethernet232", Label: "30", Profile: "OSFP-800G"},
			"E1/31": {NOSName: "1/31", BaseNOSName: "Ethernet240", Label: "31", Profile: "OSFP-800G"},
			"E1/32": {NOSName: "1/32", BaseNOSName: "Ethernet248", Label: "32", Profile: "OSFP-800G"},
			"E1/33": {NOSName: "1/33", BaseNOSName: "Ethernet256", Label: "33", Profile: "OSFP-800G"},
			"E1/34": {NOSName: "1/34", BaseNOSName: "Ethernet264", Label: "34", Profile: "OSFP-800G"},
			"E1/35": {NOSName: "1/35", BaseNOSName: "Ethernet272", Label: "35", Profile: "OSFP-800G"},
			"E1/36": {NOSName: "1/36", BaseNOSName: "Ethernet280", Label: "36", Profile: "OSFP-800G"},
			"E1/37": {NOSName: "1/37", BaseNOSName: "Ethernet288", Label: "37", Profile: "OSFP-800G"},
			"E1/38": {NOSName: "1/38", BaseNOSName: "Ethernet296", Label: "38", Profile: "OSFP-800G"},
			"E1/39": {NOSName: "1/39", BaseNOSName: "Ethernet304", Label: "39", Profile: "OSFP-800G"},
			"E1/40": {NOSName: "1/40", BaseNOSName: "Ethernet312", Label: "40", Profile: "OSFP-800G"},
			"E1/41": {NOSName: "1/41", BaseNOSName: "Ethernet320", Label: "41", Profile: "OSFP-800G"},
			"E1/42": {NOSName: "1/42", BaseNOSName: "Ethernet328", Label: "42", Profile: "OSFP-800G"},
			"E1/43": {NOSName: "1/43", BaseNOSName: "Ethernet336", Label: "43", Profile: "OSFP-800G"},
			"E1/44": {NOSName: "1/44", BaseNOSName: "Ethernet344", Label: "44", Profile: "OSFP-800G"},
			"E1/45": {NOSName: "1/45", BaseNOSName: "Ethernet352", Label: "45", Profile: "OSFP-800G"},
			"E1/46": {NOSName: "1/46", BaseNOSName: "Ethernet360", Label: "46", Profile: "OSFP-800G"},
			"E1/47": {NOSName: "1/47", BaseNOSName: "Ethernet368", Label: "47", Profile: "OSFP-800G"},
			"E1/48": {NOSName: "1/48", BaseNOSName: "Ethernet376", Label: "48", Profile: "OSFP-800G"},
			"E1/49": {NOSName: "1/49", BaseNOSName: "Ethernet384", Label: "49", Profile: "OSFP-800G"},
			"E1/50": {NOSName: "1/50", BaseNOSName: "Ethernet392", Label: "50", Profile: "OSFP-800G"},
			"E1/51": {NOSName: "1/51", BaseNOSName: "Ethernet400", Label: "51", Profile: "OSFP-800G"},
			"E1/52": {NOSName: "1/52", BaseNOSName: "Ethernet408", Label: "52", Profile: "OSFP-800G"},
			"E1/53": {NOSName: "1/53", BaseNOSName: "Ethernet416", Label: "53", Profile: "OSFP-800G"},
			"E1/54": {NOSName: "1/54", BaseNOSName: "Ethernet424", Label: "54", Profile: "OSFP-800G"},
			"E1/55": {NOSName: "1/55", BaseNOSName: "Ethernet432", Label: "55", Profile: "OSFP-800G"},
			"E1/56": {NOSName: "1/56", BaseNOSName: "Ethernet440", Label: "56", Profile: "OSFP-800G"},
			"E1/57": {NOSName: "1/57", BaseNOSName: "Ethernet448", Label: "57", Profile: "OSFP-800G"},
			"E1/58": {NOSName: "1/58", BaseNOSName: "Ethernet456", Label: "58", Profile: "OSFP-800G"},
			"E1/59": {NOSName: "1/59", BaseNOSName: "Ethernet464", Label: "59", Profile: "OSFP-800G"},
			"E1/60": {NOSName: "1/60", BaseNOSName: "Ethernet472", Label: "60", Profile: "OSFP-800G"},
			"E1/61": {NOSName: "1/61", BaseNOSName: "Ethernet480", Label: "61", Profile: "OSFP-800G"},
			"E1/62": {NOSName: "1/62", BaseNOSName: "Ethernet488", Label: "62", Profile: "OSFP-800G"},
			"E1/63": {NOSName: "1/63", BaseNOSName: "Ethernet496", Label: "63", Profile: "OSFP-800G"},
			"E1/64": {NOSName: "1/64", BaseNOSName: "Ethernet504", Label: "64", Profile: "OSFP-800G"}, // 64x OSFP-800G
			"E1/65": {NOSName: "Ethernet512", Label: "65", Profile: "SFP28-25G"},                      // 1x SFP-28-25G
			"E1/66": {NOSName: "Ethernet513", Label: "66", Profile: "SFP28-25G"},                      // 1x SFP-28-25G
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP28-25G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "25G",
					Supported: []string{"1G", "10G", "25G"},
				},
			},
			"OSFP-800G": {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x800G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"1x200G": {Offsets: []string{"0"}},
						"1x400G": {Offsets: []string{"0"}},
						"1x800G": {Offsets: []string{"0"}},
						"2x50G":  {Offsets: []string{"0", "2"}},
						"2x100G": {Offsets: []string{"0", "4"}},
						"2x200G": {Offsets: []string{"0", "4"}},
						"2x400G": {Offsets: []string{"0", "4"}},
						"4x100G": {Offsets: []string{"0", "2", "4", "6"}},
						"4x200G": {Offsets: []string{"0", "2", "4", "6"}},
						"8x100G": {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: false,
			},
		},
	},
}
