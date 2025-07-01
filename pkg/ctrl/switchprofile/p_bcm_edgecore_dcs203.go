// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var EdgecoreDCS203 = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: "edgecore-dcs203",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Edgecore DCS203",
		OtherNames:    []string{"Edgecore AS7326-56X"},
		SwitchSilicon: SiliconBroadcomTD3_X7_2_0T,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          true,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          true,
			MCLAG:         true,
			ESLAG:         true,
			ECMPRoCEQPN:   false,
		},
		NOSType:  meta.NOSTypeSONiCBCMBase,
		Platform: "x86_64-accton_as7326_56x-r0",
		Config:   wiringapi.SwitchProfileConfig{},
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "Management0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "Ethernet0", Label: "1", Group: "1"},
			"E1/2":  {NOSName: "Ethernet1", Label: "2", Group: "1"},
			"E1/3":  {NOSName: "Ethernet2", Label: "3", Group: "1"},
			"E1/4":  {NOSName: "Ethernet3", Label: "4", Group: "1"},
			"E1/5":  {NOSName: "Ethernet4", Label: "5", Group: "1"},
			"E1/6":  {NOSName: "Ethernet5", Label: "6", Group: "1"},
			"E1/7":  {NOSName: "Ethernet6", Label: "7", Group: "1"},
			"E1/8":  {NOSName: "Ethernet7", Label: "8", Group: "1"},
			"E1/9":  {NOSName: "Ethernet8", Label: "9", Group: "1"},
			"E1/10": {NOSName: "Ethernet9", Label: "10", Group: "1"},
			"E1/11": {NOSName: "Ethernet10", Label: "11", Group: "1"},
			"E1/12": {NOSName: "Ethernet11", Label: "12", Group: "1"},
			"E1/13": {NOSName: "Ethernet12", Label: "13", Group: "2"},
			"E1/14": {NOSName: "Ethernet13", Label: "14", Group: "2"},
			"E1/15": {NOSName: "Ethernet14", Label: "15", Group: "2"},
			"E1/16": {NOSName: "Ethernet15", Label: "16", Group: "2"},
			"E1/17": {NOSName: "Ethernet16", Label: "17", Group: "2"},
			"E1/18": {NOSName: "Ethernet17", Label: "18", Group: "2"},
			"E1/19": {NOSName: "Ethernet18", Label: "19", Group: "2"},
			"E1/20": {NOSName: "Ethernet19", Label: "20", Group: "2"},
			"E1/21": {NOSName: "Ethernet20", Label: "21", Group: "2"},
			"E1/22": {NOSName: "Ethernet21", Label: "22", Group: "2"},
			"E1/23": {NOSName: "Ethernet22", Label: "23", Group: "2"},
			"E1/24": {NOSName: "Ethernet23", Label: "24", Group: "2"},
			"E1/25": {NOSName: "Ethernet24", Label: "25", Group: "3"},
			"E1/26": {NOSName: "Ethernet25", Label: "26", Group: "3"},
			"E1/27": {NOSName: "Ethernet26", Label: "27", Group: "3"},
			"E1/28": {NOSName: "Ethernet27", Label: "28", Group: "3"},
			"E1/29": {NOSName: "Ethernet28", Label: "29", Group: "3"},
			"E1/30": {NOSName: "Ethernet29", Label: "30", Group: "3"},
			"E1/31": {NOSName: "Ethernet30", Label: "31", Group: "3"},
			"E1/32": {NOSName: "Ethernet31", Label: "32", Group: "3"},
			"E1/33": {NOSName: "Ethernet32", Label: "33", Group: "3"},
			"E1/34": {NOSName: "Ethernet33", Label: "34", Group: "3"},
			"E1/35": {NOSName: "Ethernet34", Label: "35", Group: "3"},
			"E1/36": {NOSName: "Ethernet35", Label: "36", Group: "3"},
			"E1/37": {NOSName: "Ethernet36", Label: "37", Group: "4"},
			"E1/38": {NOSName: "Ethernet37", Label: "38", Group: "4"},
			"E1/39": {NOSName: "Ethernet38", Label: "39", Group: "4"},
			"E1/40": {NOSName: "Ethernet39", Label: "40", Group: "4"},
			"E1/41": {NOSName: "Ethernet40", Label: "41", Group: "4"},
			"E1/42": {NOSName: "Ethernet41", Label: "42", Group: "4"},
			"E1/43": {NOSName: "Ethernet42", Label: "43", Group: "4"},
			"E1/44": {NOSName: "Ethernet43", Label: "44", Group: "4"},
			"E1/45": {NOSName: "Ethernet44", Label: "45", Group: "4"},
			"E1/46": {NOSName: "Ethernet45", Label: "46", Group: "4"},
			"E1/47": {NOSName: "Ethernet46", Label: "47", Group: "4"},
			"E1/48": {NOSName: "Ethernet47", Label: "48", Group: "4"}, // 48 x SFP28-25G, 4 Groups of 12 ports
			"E1/49": {NOSName: "1/49", BaseNOSName: "Ethernet48", Label: "49", Profile: "QSFP28-100G"},
			"E1/50": {NOSName: "1/53", BaseNOSName: "Ethernet52", Label: "50", Profile: "QSFP28-100G"},
			"E1/51": {NOSName: "1/57", BaseNOSName: "Ethernet56", Label: "51", Profile: "QSFP28-100G"},
			"E1/52": {NOSName: "1/61", BaseNOSName: "Ethernet60", Label: "52", Profile: "QSFP28-100G"},
			"E1/53": {NOSName: "1/65", BaseNOSName: "Ethernet64", Label: "53", Profile: "QSFP28-100G"},
			"E1/54": {NOSName: "1/69", BaseNOSName: "Ethernet68", Label: "54", Profile: "QSFP28-100G"},
			"E1/55": {NOSName: "1/73", BaseNOSName: "Ethernet72", Label: "55", Profile: "QSFP28-100G"},
			"E1/56": {NOSName: "Ethernet76", Label: "56", Profile: "QSFP28-100G" + wiringapi.NonBreakoutPortExceptionSuffix}, // 8x QSFP28-100G, with last one not breakable
			"E1/57": {NOSName: "Ethernet80", Label: "57", Profile: "SFP28-10G"},
			"E1/58": {NOSName: "Ethernet81", Label: "58", Profile: "SFP28-10G"}, // 2x SFP28-10G
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
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP28-25G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "25G",
					Supported: []string{"10G", "25G"},
				},
			},
			"SFP28-10G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "10G",
					Supported: []string{"1G", "10G"},
				},
			},
			"QSFP28-100G" + wiringapi.NonBreakoutPortExceptionSuffix: {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "100G",
					Supported: []string{"40G", "100G"},
				},
			},
			"QSFP28-100G": {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x100G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x100G": {Offsets: []string{"0"}},
						"1x40G":  {Offsets: []string{"0"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
					},
				},
			},
		},
	},
}
