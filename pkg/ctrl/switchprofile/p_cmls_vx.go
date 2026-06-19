// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var CumulusVX = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: meta.SwitchProfileCmlsVX,
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Cumulus VX",
		SwitchSilicon: SiliconVS,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          false,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          true,
			MCLAG:         false,
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
			"E1/1":  {NOSName: "1", BaseNOSName: "swp", Label: "1", Profile: "SFP28-10G"},
			"E1/2":  {NOSName: "2", BaseNOSName: "swp", Label: "2", Profile: "SFP28-10G"},
			"E1/3":  {NOSName: "3", BaseNOSName: "swp", Label: "3", Profile: "SFP28-10G"},
			"E1/4":  {NOSName: "4", BaseNOSName: "swp", Label: "4", Profile: "SFP28-10G"},
			"E1/5":  {NOSName: "5", BaseNOSName: "swp", Label: "5", Profile: "SFP28-10G"},
			"E1/6":  {NOSName: "6", BaseNOSName: "swp", Label: "6", Profile: "SFP28-10G"},
			"E1/7":  {NOSName: "7", BaseNOSName: "swp", Label: "7", Profile: "SFP28-10G"},
			"E1/8":  {NOSName: "8", BaseNOSName: "swp", Label: "8", Profile: "SFP28-10G"},
			"E1/9":  {NOSName: "9", BaseNOSName: "swp", Label: "9", Profile: "SFP28-10G"},
			"E1/10": {NOSName: "10", BaseNOSName: "swp", Label: "10", Profile: "SFP28-10G"},
			"E1/11": {NOSName: "11", BaseNOSName: "swp", Label: "11", Profile: "SFP28-10G"},
			"E1/12": {NOSName: "12", BaseNOSName: "swp", Label: "12", Profile: "SFP28-10G"},
			"E1/13": {NOSName: "13", BaseNOSName: "swp", Label: "13", Profile: "SFP28-10G"},
			"E1/14": {NOSName: "14", BaseNOSName: "swp", Label: "14", Profile: "SFP28-10G"},
			"E1/15": {NOSName: "15", BaseNOSName: "swp", Label: "15", Profile: "SFP28-10G"},
			"E1/16": {NOSName: "16", BaseNOSName: "swp", Label: "16", Profile: "SFP28-10G"},
			"E1/17": {NOSName: "17", BaseNOSName: "swp", Label: "17", Profile: "SFP28-10G"},
			"E1/18": {NOSName: "18", BaseNOSName: "swp", Label: "18", Profile: "SFP28-10G"},
			"E1/19": {NOSName: "19", BaseNOSName: "swp", Label: "19", Profile: "SFP28-10G"},
			"E1/20": {NOSName: "20", BaseNOSName: "swp", Label: "20", Profile: "SFP28-10G"},
			"E1/21": {NOSName: "21", BaseNOSName: "swp", Label: "21", Profile: "SFP28-10G"},
			"E1/22": {NOSName: "22", BaseNOSName: "swp", Label: "22", Profile: "SFP28-10G"},
			"E1/23": {NOSName: "23", BaseNOSName: "swp", Label: "23", Profile: "SFP28-10G"},
			"E1/24": {NOSName: "24", BaseNOSName: "swp", Label: "24", Profile: "SFP28-10G"},
			"E1/25": {NOSName: "25", BaseNOSName: "swp", Label: "25", Profile: "SFP28-10G"},
			"E1/26": {NOSName: "26", BaseNOSName: "swp", Label: "26", Profile: "SFP28-10G"},
			"E1/27": {NOSName: "27", BaseNOSName: "swp", Label: "27", Profile: "SFP28-10G"},
			"E1/28": {NOSName: "28", BaseNOSName: "swp", Label: "28", Profile: "SFP28-10G"},
			"E1/29": {NOSName: "29", BaseNOSName: "swp", Label: "29", Profile: "SFP28-10G"},
			"E1/30": {NOSName: "30", BaseNOSName: "swp", Label: "30", Profile: "SFP28-10G"},
			"E1/31": {NOSName: "31", BaseNOSName: "swp", Label: "31", Profile: "SFP28-10G"},
			"E1/32": {NOSName: "32", BaseNOSName: "swp", Label: "32", Profile: "SFP28-10G"},
			"E1/33": {NOSName: "33", BaseNOSName: "swp", Label: "33", Profile: "SFP28-10G"},
			"E1/34": {NOSName: "34", BaseNOSName: "swp", Label: "34", Profile: "SFP28-10G"},
			"E1/35": {NOSName: "35", BaseNOSName: "swp", Label: "35", Profile: "SFP28-10G"},
			"E1/36": {NOSName: "36", BaseNOSName: "swp", Label: "36", Profile: "SFP28-10G"},
			"E1/37": {NOSName: "37", BaseNOSName: "swp", Label: "37", Profile: "SFP28-10G"},
			"E1/38": {NOSName: "38", BaseNOSName: "swp", Label: "38", Profile: "SFP28-10G"},
			"E1/39": {NOSName: "39", BaseNOSName: "swp", Label: "39", Profile: "SFP28-10G"},
			"E1/40": {NOSName: "40", BaseNOSName: "swp", Label: "40", Profile: "SFP28-10G"},
			"E1/41": {NOSName: "41", BaseNOSName: "swp", Label: "41", Profile: "SFP28-10G"},
			"E1/42": {NOSName: "42", BaseNOSName: "swp", Label: "42", Profile: "SFP28-10G"},
			"E1/43": {NOSName: "43", BaseNOSName: "swp", Label: "43", Profile: "SFP28-10G"},
			"E1/44": {NOSName: "44", BaseNOSName: "swp", Label: "44", Profile: "SFP28-10G"},
			"E1/45": {NOSName: "45", BaseNOSName: "swp", Label: "45", Profile: "SFP28-10G"},
			"E1/46": {NOSName: "46", BaseNOSName: "swp", Label: "46", Profile: "SFP28-10G"},
			"E1/47": {NOSName: "47", BaseNOSName: "swp", Label: "47", Profile: "SFP28-10G"},
			"E1/48": {NOSName: "48", BaseNOSName: "swp", Label: "48", Profile: "SFP28-10G"},
			"E1/49": {NOSName: "49", BaseNOSName: "swp", Label: "49", Profile: "SFP28-10G"},
			"E1/50": {NOSName: "50", BaseNOSName: "swp", Label: "50", Profile: "SFP28-10G"},
			"E1/51": {NOSName: "51", BaseNOSName: "swp", Label: "51", Profile: "SFP28-10G"},
			"E1/52": {NOSName: "52", BaseNOSName: "swp", Label: "52", Profile: "SFP28-10G"},
			"E1/53": {NOSName: "53", BaseNOSName: "swp", Label: "53", Profile: "SFP28-10G"},
			"E1/54": {NOSName: "54", BaseNOSName: "swp", Label: "54", Profile: "SFP28-10G"},
			"E1/55": {NOSName: "55", BaseNOSName: "swp", Label: "55", Profile: "SFP28-10G"},
			"E1/56": {NOSName: "56", BaseNOSName: "swp", Label: "56", Profile: "SFP28-10G"},
			"E1/57": {NOSName: "57", BaseNOSName: "swp", Label: "57", Profile: "SFP28-10G"},
			"E1/58": {NOSName: "58", BaseNOSName: "swp", Label: "58", Profile: "SFP28-10G"},
			"E1/59": {NOSName: "59", BaseNOSName: "swp", Label: "59", Profile: "SFP28-10G"},
			"E1/60": {NOSName: "60", BaseNOSName: "swp", Label: "60", Profile: "SFP28-10G"},
			"E1/61": {NOSName: "61", BaseNOSName: "swp", Label: "61", Profile: "SFP28-10G"},
			"E1/62": {NOSName: "62", BaseNOSName: "swp", Label: "62", Profile: "SFP28-10G"},
			"E1/63": {NOSName: "63", BaseNOSName: "swp", Label: "63", Profile: "SFP28-10G"},
			"E1/64": {NOSName: "64", BaseNOSName: "swp", Label: "64", Profile: "SFP28-10G"},
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
