// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

var CumulusVX = wiringapi.SwitchProfile{
	Name: meta.SwitchProfileCmlsVX,
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Cumulus VX",
		SwitchSilicon: SiliconVS,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          false,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          true,
			ESLAG:         true,
			ECMPRoCEQPN:   false,
		},
		NOSType:  meta.NOSTypeCumulusVX,
		Platform: "x86_64-kvm_x86_64-r0",
		Config:   wiringapi.SwitchProfileConfig{
			// TODO check if some limitations
			// MaxPathsEBGP: 16,
		},
		// TODO update with actual values, it's obviously incorrect right now but it doesn't matter for the initial research
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "eth0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "swp1", Label: "1", Profile: "SFP28-10G"},
			"E1/2":  {NOSName: "swp2", Label: "2", Profile: "SFP28-10G"},
			"E1/3":  {NOSName: "swp3", Label: "3", Profile: "SFP28-10G"},
			"E1/4":  {NOSName: "swp4", Label: "4", Profile: "SFP28-10G"},
			"E1/5":  {NOSName: "swp5", Label: "5", Profile: "SFP28-10G"},
			"E1/6":  {NOSName: "swp6", Label: "6", Profile: "SFP28-10G"},
			"E1/7":  {NOSName: "swp7", Label: "7", Profile: "SFP28-10G"},
			"E1/8":  {NOSName: "swp8", Label: "8", Profile: "SFP28-10G"},
			"E1/9":  {NOSName: "swp9", Label: "9", Profile: "SFP28-10G"},
			"E1/10": {NOSName: "swp10", Label: "10", Profile: "SFP28-10G"},
			"E1/11": {NOSName: "swp11", Label: "11", Profile: "SFP28-10G"},
			"E1/12": {NOSName: "swp12", Label: "12", Profile: "SFP28-10G"},
			"E1/13": {NOSName: "swp13", Label: "13", Profile: "SFP28-10G"},
			"E1/14": {NOSName: "swp14", Label: "14", Profile: "SFP28-10G"},
			"E1/15": {NOSName: "swp15", Label: "15", Profile: "SFP28-10G"},
			"E1/16": {NOSName: "swp16", Label: "16", Profile: "SFP28-10G"},
			"E1/17": {NOSName: "swp17", Label: "17", Profile: "SFP28-10G"},
			"E1/18": {NOSName: "swp18", Label: "18", Profile: "SFP28-10G"},
			"E1/19": {NOSName: "swp19", Label: "19", Profile: "SFP28-10G"},
			"E1/20": {NOSName: "swp20", Label: "20", Profile: "SFP28-10G"},
			"E1/21": {NOSName: "swp21", Label: "21", Profile: "SFP28-10G"},
			"E1/22": {NOSName: "swp22", Label: "22", Profile: "SFP28-10G"},
			"E1/23": {NOSName: "swp23", Label: "23", Profile: "SFP28-10G"},
			"E1/24": {NOSName: "swp24", Label: "24", Profile: "SFP28-10G"},
			"E1/25": {NOSName: "swp25", Label: "25", Profile: "SFP28-10G"},
			"E1/26": {NOSName: "swp26", Label: "26", Profile: "SFP28-10G"},
			"E1/27": {NOSName: "swp27", Label: "27", Profile: "SFP28-10G"},
			"E1/28": {NOSName: "swp28", Label: "28", Profile: "SFP28-10G"},
			"E1/29": {NOSName: "swp29", Label: "29", Profile: "SFP28-10G"},
			"E1/30": {NOSName: "swp30", Label: "30", Profile: "SFP28-10G"},
			"E1/31": {NOSName: "swp31", Label: "31", Profile: "SFP28-10G"},
			"E1/32": {NOSName: "swp32", Label: "32", Profile: "SFP28-10G"},
			"E1/33": {NOSName: "swp33", Label: "33", Profile: "SFP28-10G"},
			"E1/34": {NOSName: "swp34", Label: "34", Profile: "SFP28-10G"},
			"E1/35": {NOSName: "swp35", Label: "35", Profile: "SFP28-10G"},
			"E1/36": {NOSName: "swp36", Label: "36", Profile: "SFP28-10G"},
			"E1/37": {NOSName: "swp37", Label: "37", Profile: "SFP28-10G"},
			"E1/38": {NOSName: "swp38", Label: "38", Profile: "SFP28-10G"},
			"E1/39": {NOSName: "swp39", Label: "39", Profile: "SFP28-10G"},
			"E1/40": {NOSName: "swp40", Label: "40", Profile: "SFP28-10G"},
			"E1/41": {NOSName: "swp41", Label: "41", Profile: "SFP28-10G"},
			"E1/42": {NOSName: "swp42", Label: "42", Profile: "SFP28-10G"},
			"E1/43": {NOSName: "swp43", Label: "43", Profile: "SFP28-10G"},
			"E1/44": {NOSName: "swp44", Label: "44", Profile: "SFP28-10G"},
			"E1/45": {NOSName: "swp45", Label: "45", Profile: "SFP28-10G"},
			"E1/46": {NOSName: "swp46", Label: "46", Profile: "SFP28-10G"},
			"E1/47": {NOSName: "swp47", Label: "47", Profile: "SFP28-10G"},
			"E1/48": {NOSName: "swp48", Label: "48", Profile: "SFP28-10G"},
			"E1/49": {NOSName: "swp49", Label: "49", Profile: "SFP28-10G"},
			"E1/50": {NOSName: "swp50", Label: "50", Profile: "SFP28-10G"},
			"E1/51": {NOSName: "swp51", Label: "51", Profile: "SFP28-10G"},
			"E1/52": {NOSName: "swp52", Label: "52", Profile: "SFP28-10G"},
			"E1/53": {NOSName: "swp53", Label: "53", Profile: "SFP28-10G"},
			"E1/54": {NOSName: "swp54", Label: "54", Profile: "SFP28-10G"},
			"E1/55": {NOSName: "swp55", Label: "55", Profile: "SFP28-10G"},
			"E1/56": {NOSName: "swp56", Label: "56", Profile: "SFP28-10G"},
			"E1/57": {NOSName: "swp57", Label: "57", Profile: "SFP28-10G"},
			"E1/58": {NOSName: "swp58", Label: "58", Profile: "SFP28-10G"},
			"E1/59": {NOSName: "swp59", Label: "59", Profile: "SFP28-10G"},
			"E1/60": {NOSName: "swp60", Label: "60", Profile: "SFP28-10G"},
			"E1/61": {NOSName: "swp61", Label: "61", Profile: "SFP28-10G"},
			"E1/62": {NOSName: "swp62", Label: "62", Profile: "SFP28-10G"},
			"E1/63": {NOSName: "swp63", Label: "63", Profile: "SFP28-10G"},
			"E1/64": {NOSName: "swp64", Label: "64", Profile: "SFP28-10G"},
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP28-10G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "10G",
					Supported: []string{"1G", "10G"},
				},
			},
		},
	},
}
