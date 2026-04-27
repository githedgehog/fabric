// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var EdgecoreDCS240 = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: "edgecore-dcs240",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Edgecore DCS240",
		OtherNames:    []string{"Edgecore AS9726"},
		SwitchSilicon: SiliconBroadcomTD4,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          true,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          true,
			MCLAG:         false,
			ESLAG:         true,
			ECMPRoCEQPN:   false,
		},
		Notes:    "Upper 16 ports supply maximum of 24W, lower 16 ports supply maximum of 14W",
		NOSType:  meta.NOSTypeSONiCBCMBase,
		Platform: "x86_64-accton_as9726_32d-r0",
		Config:   wiringapi.SwitchProfileConfig{},
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "Management0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "1/1", BaseNOSName: "Ethernet0", Label: "1", Profile: "QSFP56DD-400G", Pipeline: "1"},
			"E1/2":  {NOSName: "1/2", BaseNOSName: "Ethernet8", Label: "2", Profile: "QSFP56DD-400G", Pipeline: "1"},
			"E1/3":  {NOSName: "1/3", BaseNOSName: "Ethernet16", Label: "3", Profile: "QSFP56DD-400G", Pipeline: "1"},
			"E1/4":  {NOSName: "1/4", BaseNOSName: "Ethernet24", Label: "4", Profile: "QSFP56DD-400G", Pipeline: "1"},
			"E1/5":  {NOSName: "1/5", BaseNOSName: "Ethernet32", Label: "5", Profile: "QSFP56DD-400G", Pipeline: "2"},
			"E1/6":  {NOSName: "1/6", BaseNOSName: "Ethernet40", Label: "6", Profile: "QSFP56DD-400G", Pipeline: "2"},
			"E1/7":  {NOSName: "1/7", BaseNOSName: "Ethernet48", Label: "7", Profile: "QSFP56DD-400G", Pipeline: "2"},
			"E1/8":  {NOSName: "1/8", BaseNOSName: "Ethernet56", Label: "8", Profile: "QSFP56DD-400G", Pipeline: "2"},
			"E1/9":  {NOSName: "1/9", BaseNOSName: "Ethernet64", Label: "9", Profile: "QSFP56DD-400G", Pipeline: "3"},
			"E1/10": {NOSName: "1/10", BaseNOSName: "Ethernet72", Label: "10", Profile: "QSFP56DD-400G", Pipeline: "3"},
			"E1/11": {NOSName: "1/11", BaseNOSName: "Ethernet80", Label: "11", Profile: "QSFP56DD-400G", Pipeline: "3"},
			"E1/12": {NOSName: "1/12", BaseNOSName: "Ethernet88", Label: "12", Profile: "QSFP56DD-400G", Pipeline: "3"},
			"E1/13": {NOSName: "1/13", BaseNOSName: "Ethernet96", Label: "13", Profile: "QSFP56DD-400G", Pipeline: "4"},
			"E1/14": {NOSName: "1/14", BaseNOSName: "Ethernet104", Label: "14", Profile: "QSFP56DD-400G", Pipeline: "4"},
			"E1/15": {NOSName: "1/15", BaseNOSName: "Ethernet112", Label: "15", Profile: "QSFP56DD-400G", Pipeline: "4"},
			"E1/16": {NOSName: "1/16", BaseNOSName: "Ethernet120", Label: "16", Profile: "QSFP56DD-400G", Pipeline: "4"},
			"E1/17": {NOSName: "1/17", BaseNOSName: "Ethernet128", Label: "17", Profile: "QSFP56DD-400G", Pipeline: "5"},
			"E1/18": {NOSName: "1/18", BaseNOSName: "Ethernet136", Label: "18", Profile: "QSFP56DD-400G", Pipeline: "5"},
			"E1/19": {NOSName: "1/19", BaseNOSName: "Ethernet144", Label: "19", Profile: "QSFP56DD-400G", Pipeline: "5"},
			"E1/20": {NOSName: "1/20", BaseNOSName: "Ethernet152", Label: "20", Profile: "QSFP56DD-400G", Pipeline: "5"},
			"E1/21": {NOSName: "1/21", BaseNOSName: "Ethernet160", Label: "21", Profile: "QSFP56DD-400G", Pipeline: "6"},
			"E1/22": {NOSName: "1/22", BaseNOSName: "Ethernet168", Label: "22", Profile: "QSFP56DD-400G", Pipeline: "6"},
			"E1/23": {NOSName: "1/23", BaseNOSName: "Ethernet176", Label: "23", Profile: "QSFP56DD-400G", Pipeline: "6"},
			"E1/24": {NOSName: "1/24", BaseNOSName: "Ethernet184", Label: "24", Profile: "QSFP56DD-400G", Pipeline: "6"},
			"E1/25": {NOSName: "1/25", BaseNOSName: "Ethernet192", Label: "25", Profile: "QSFP56DD-400G", Pipeline: "7"},
			"E1/26": {NOSName: "1/26", BaseNOSName: "Ethernet200", Label: "26", Profile: "QSFP56DD-400G", Pipeline: "7"},
			"E1/27": {NOSName: "1/27", BaseNOSName: "Ethernet208", Label: "27", Profile: "QSFP56DD-400G", Pipeline: "7"},
			"E1/28": {NOSName: "1/28", BaseNOSName: "Ethernet216", Label: "28", Profile: "QSFP56DD-400G", Pipeline: "7"},
			"E1/29": {NOSName: "1/29", BaseNOSName: "Ethernet224", Label: "29", Profile: "QSFP56DD-400G", Pipeline: "8"},
			"E1/30": {NOSName: "1/30", BaseNOSName: "Ethernet232", Label: "30", Profile: "QSFP56DD-400G", Pipeline: "8"},
			"E1/31": {NOSName: "1/31", BaseNOSName: "Ethernet240", Label: "31", Profile: "QSFP56DD-400G", Pipeline: "8"},
			"E1/32": {NOSName: "1/32", BaseNOSName: "Ethernet248", Label: "32", Profile: "QSFP56DD-400G", Pipeline: "8"}, // 32x QSFP56DD-400G
			"E1/33": {NOSName: "Ethernet256", Label: "33", Profile: "SFP28-10G"},
			"E1/34": {NOSName: "Ethernet257", Label: "34", Profile: "SFP28-10G"},
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP28-10G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "10G",
					Supported: []string{"1G", "10G"},
				},
				AutoNegAllowed: false,
			},
			"QSFP56DD-400G": {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x400G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x10G":  {Offsets: []string{"0"}},
						"1x25G":  {Offsets: []string{"0"}},
						"1x40G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"1x400G": {Offsets: []string{"0"}},
						"2x40G":  {Offsets: []string{"0", "4"}},
						"2x50G":  {Offsets: []string{"0", "2"}},
						"2x100G": {Offsets: []string{"0", "4"}},
						"2x200G": {Offsets: []string{"0", "4"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x100G": {Offsets: []string{"0", "2", "4", "6"}},
						"8x10G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}},
						"8x25G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}},
						"8x50G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: false,
			},
		},

		Pipelines: map[string]wiringapi.SwitchProfilePipeline{
			"1": {MaxPorts: 18},
			"2": {MaxPorts: 18},
			"3": {MaxPorts: 18},
			"4": {MaxPorts: 18},
			"5": {MaxPorts: 18},
			"6": {MaxPorts: 18},
			"7": {MaxPorts: 18},
			"8": {MaxPorts: 18},
		},
	},
}
