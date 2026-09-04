// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var CmlsNvidiaSN2201 = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: "nvidia-sn2201",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "NVIDIA SN2201",
		SwitchSilicon: SiliconSpectrum,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          true,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          false,
			MCLAG:         false,
			ESLAG:         true,
			ECMPRoCEQPN:   false,
		},
		NOSType:  meta.NOSTypeCumulusMlx,
		Platform: "x86-64-nv-sn2201",
		Config:   wiringapi.SwitchProfileConfig{},
		MaxPorts: 64,
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "eth0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "1", BaseNOSName: "swp", Label: "1", Profile: "RJ45-1G"},
			"E1/2":  {NOSName: "2", BaseNOSName: "swp", Label: "2", Profile: "RJ45-1G"},
			"E1/3":  {NOSName: "3", BaseNOSName: "swp", Label: "3", Profile: "RJ45-1G"},
			"E1/4":  {NOSName: "4", BaseNOSName: "swp", Label: "4", Profile: "RJ45-1G"},
			"E1/5":  {NOSName: "5", BaseNOSName: "swp", Label: "5", Profile: "RJ45-1G"},
			"E1/6":  {NOSName: "6", BaseNOSName: "swp", Label: "6", Profile: "RJ45-1G"},
			"E1/7":  {NOSName: "7", BaseNOSName: "swp", Label: "7", Profile: "RJ45-1G"},
			"E1/8":  {NOSName: "8", BaseNOSName: "swp", Label: "8", Profile: "RJ45-1G"},
			"E1/9":  {NOSName: "9", BaseNOSName: "swp", Label: "9", Profile: "RJ45-1G"},
			"E1/10": {NOSName: "10", BaseNOSName: "swp", Label: "10", Profile: "RJ45-1G"},
			"E1/11": {NOSName: "11", BaseNOSName: "swp", Label: "11", Profile: "RJ45-1G"},
			"E1/12": {NOSName: "12", BaseNOSName: "swp", Label: "12", Profile: "RJ45-1G"},
			"E1/13": {NOSName: "13", BaseNOSName: "swp", Label: "13", Profile: "RJ45-1G"},
			"E1/14": {NOSName: "14", BaseNOSName: "swp", Label: "14", Profile: "RJ45-1G"},
			"E1/15": {NOSName: "15", BaseNOSName: "swp", Label: "15", Profile: "RJ45-1G"},
			"E1/16": {NOSName: "16", BaseNOSName: "swp", Label: "16", Profile: "RJ45-1G"},
			"E1/17": {NOSName: "17", BaseNOSName: "swp", Label: "17", Profile: "RJ45-1G"},
			"E1/18": {NOSName: "18", BaseNOSName: "swp", Label: "18", Profile: "RJ45-1G"},
			"E1/19": {NOSName: "19", BaseNOSName: "swp", Label: "19", Profile: "RJ45-1G"},
			"E1/20": {NOSName: "20", BaseNOSName: "swp", Label: "20", Profile: "RJ45-1G"},
			"E1/21": {NOSName: "21", BaseNOSName: "swp", Label: "21", Profile: "RJ45-1G"},
			"E1/22": {NOSName: "22", BaseNOSName: "swp", Label: "22", Profile: "RJ45-1G"},
			"E1/23": {NOSName: "23", BaseNOSName: "swp", Label: "23", Profile: "RJ45-1G"},
			"E1/24": {NOSName: "24", BaseNOSName: "swp", Label: "24", Profile: "RJ45-1G"},
			"E1/25": {NOSName: "25", BaseNOSName: "swp", Label: "25", Profile: "RJ45-1G"},
			"E1/26": {NOSName: "26", BaseNOSName: "swp", Label: "26", Profile: "RJ45-1G"},
			"E1/27": {NOSName: "27", BaseNOSName: "swp", Label: "27", Profile: "RJ45-1G"},
			"E1/28": {NOSName: "28", BaseNOSName: "swp", Label: "28", Profile: "RJ45-1G"},
			"E1/29": {NOSName: "29", BaseNOSName: "swp", Label: "29", Profile: "RJ45-1G"},
			"E1/30": {NOSName: "30", BaseNOSName: "swp", Label: "30", Profile: "RJ45-1G"},
			"E1/31": {NOSName: "31", BaseNOSName: "swp", Label: "31", Profile: "RJ45-1G"},
			"E1/32": {NOSName: "32", BaseNOSName: "swp", Label: "32", Profile: "RJ45-1G"},
			"E1/33": {NOSName: "33", BaseNOSName: "swp", Label: "33", Profile: "RJ45-1G"},
			"E1/34": {NOSName: "34", BaseNOSName: "swp", Label: "34", Profile: "RJ45-1G"},
			"E1/35": {NOSName: "35", BaseNOSName: "swp", Label: "35", Profile: "RJ45-1G"},
			"E1/36": {NOSName: "36", BaseNOSName: "swp", Label: "36", Profile: "RJ45-1G"},
			"E1/37": {NOSName: "37", BaseNOSName: "swp", Label: "37", Profile: "RJ45-1G"},
			"E1/38": {NOSName: "38", BaseNOSName: "swp", Label: "38", Profile: "RJ45-1G"},
			"E1/39": {NOSName: "39", BaseNOSName: "swp", Label: "39", Profile: "RJ45-1G"},
			"E1/40": {NOSName: "40", BaseNOSName: "swp", Label: "40", Profile: "RJ45-1G"},
			"E1/41": {NOSName: "41", BaseNOSName: "swp", Label: "41", Profile: "RJ45-1G"},
			"E1/42": {NOSName: "42", BaseNOSName: "swp", Label: "42", Profile: "RJ45-1G"},
			"E1/43": {NOSName: "43", BaseNOSName: "swp", Label: "43", Profile: "RJ45-1G"},
			"E1/44": {NOSName: "44", BaseNOSName: "swp", Label: "44", Profile: "RJ45-1G"},
			"E1/45": {NOSName: "45", BaseNOSName: "swp", Label: "45", Profile: "RJ45-1G"},
			"E1/46": {NOSName: "46", BaseNOSName: "swp", Label: "46", Profile: "RJ45-1G"},
			"E1/47": {NOSName: "47", BaseNOSName: "swp", Label: "47", Profile: "RJ45-1G"},
			"E1/48": {NOSName: "48", BaseNOSName: "swp", Label: "48", Profile: "RJ45-1G"},
			"E1/49": {NOSName: "49", BaseNOSName: "swp", Label: "49", Profile: "QSFP28-100G", Pipeline: "1"},
			"E1/50": {NOSName: "50", BaseNOSName: "swp", Label: "50", Profile: "QSFP28-100G", Pipeline: "2"},
			"E1/51": {NOSName: "51", BaseNOSName: "swp", Label: "51", Profile: "QSFP28-100G", Pipeline: "3"},
			"E1/52": {NOSName: "52", BaseNOSName: "swp", Label: "52", Profile: "QSFP28-100G", Pipeline: "4"},
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"RJ45-1G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "1G",
					Supported: []string{"1G"},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
			"QSFP28-100G": {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x100G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x25G":  {Offsets: []string{""}},
						"1x40G":  {Offsets: []string{""}},
						"1x50G":  {Offsets: []string{""}},
						"1x100G": {Offsets: []string{""}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x40G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
		},
		Pipelines: map[string]wiringapi.SwitchProfilePipeline{
			"1": {MaxPorts: 4},
			"2": {MaxPorts: 4},
			"3": {MaxPorts: 4},
			"4": {MaxPorts: 4},
		},
	},
}
