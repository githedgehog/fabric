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
			MCLAG:         true,
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
			"M1":    {NOSName: "Management0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "Ethernet0", Label: "1", Profile: "QSFP28-100G"},
			"E1/2":  {NOSName: "Ethernet1", Label: "2", Profile: "QSFP28-100G"},
			"E1/3":  {NOSName: "Ethernet2", Label: "3", Profile: "QSFP28-100G"},
			"E1/4":  {NOSName: "Ethernet3", Label: "4", Profile: "QSFP28-100G"},
			"E1/5":  {NOSName: "Ethernet4", Label: "5", Profile: "QSFP28-100G"},
			"E1/6":  {NOSName: "Ethernet5", Label: "6", Profile: "QSFP28-100G"},
			"E1/7":  {NOSName: "Ethernet6", Label: "7", Profile: "QSFP28-100G"},
			"E1/8":  {NOSName: "Ethernet7", Label: "8", Profile: "QSFP28-100G"},
			"E1/9":  {NOSName: "Ethernet8", Label: "9", Profile: "QSFP28-100G"},
			"E1/10": {NOSName: "Ethernet9", Label: "10", Profile: "QSFP28-100G"},
			"E1/11": {NOSName: "Ethernet10", Label: "11", Profile: "QSFP28-100G"},
			"E1/12": {NOSName: "Ethernet11", Label: "12", Profile: "QSFP28-100G"},
			"E1/13": {NOSName: "Ethernet12", Label: "13", Profile: "QSFP28-100G"},
			"E1/14": {NOSName: "Ethernet13", Label: "14", Profile: "QSFP28-100G"},
			"E1/15": {NOSName: "Ethernet14", Label: "15", Profile: "QSFP28-100G"},
			"E1/16": {NOSName: "Ethernet15", Label: "16", Profile: "QSFP28-100G"},
			"E1/17": {NOSName: "Ethernet16", Label: "17", Profile: "QSFP28-100G"},
			"E1/18": {NOSName: "Ethernet17", Label: "18", Profile: "QSFP28-100G"},
			"E1/19": {NOSName: "Ethernet18", Label: "19", Profile: "QSFP28-100G"},
			"E1/20": {NOSName: "Ethernet19", Label: "20", Profile: "QSFP28-100G"},
			"E1/21": {NOSName: "Ethernet20", Label: "21", Profile: "QSFP28-100G"},
			"E1/22": {NOSName: "Ethernet21", Label: "22", Profile: "QSFP28-100G"},
			"E1/23": {NOSName: "Ethernet22", Label: "23", Profile: "QSFP28-100G"},
			"E1/24": {NOSName: "Ethernet23", Label: "24", Profile: "QSFP28-100G"},
			"E1/25": {NOSName: "Ethernet24", Label: "25", Profile: "QSFP28-100G"},
			"E1/26": {NOSName: "Ethernet25", Label: "26", Profile: "QSFP28-100G"},
			"E1/27": {NOSName: "Ethernet26", Label: "27", Profile: "QSFP28-100G"},
			"E1/28": {NOSName: "Ethernet27", Label: "28", Profile: "QSFP28-100G"},
			"E1/29": {NOSName: "Ethernet28", Label: "29", Profile: "QSFP28-100G"},
			"E1/30": {NOSName: "Ethernet29", Label: "30", Profile: "QSFP28-100G"},
			"E1/31": {NOSName: "Ethernet30", Label: "31", Profile: "QSFP28-100G"},
			"E1/32": {NOSName: "Ethernet31", Label: "32", Profile: "QSFP28-100G"}, // 32x QSFP28-100G
			"E1/33": {NOSName: "Ethernet32", Label: "33", Profile: "SFP28-10G"},   // 1x SFP28-10G
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP28-10G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "10G",
					Supported: []string{"1G", "10G"},
				},
			},
			"QSFP28-100G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "100G",
					Supported: []string{"10G", "25G", "50G", "100G"},
				},
			},
		},
	},
}
