// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var CelesticaDS3000 = wiringapi.SwitchProfile{
	ObjectMeta: kmetav1.ObjectMeta{
		Name: "celestica-ds3000",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "Celestica DS3000",
		OtherNames:    []string{"Celestica Seastone2"},
		SwitchSilicon: SiliconBroadcomTD3_X7_3_2T,
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
		Platform: "x86_64-cel_seastone_2-r0",
		Config:   wiringapi.SwitchProfileConfig{},
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "Management0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "1/1", BaseNOSName: "Ethernet0", Label: "1", Profile: "QSFP28-100G"},
			"E1/2":  {NOSName: "1/2", BaseNOSName: "Ethernet4", Label: "2", Profile: "QSFP28-100G"},
			"E1/3":  {NOSName: "1/3", BaseNOSName: "Ethernet8", Label: "3", Profile: "QSFP28-100G"},
			"E1/4":  {NOSName: "1/4", BaseNOSName: "Ethernet12", Label: "4", Profile: "QSFP28-100G"},
			"E1/5":  {NOSName: "1/5", BaseNOSName: "Ethernet16", Label: "5", Profile: "QSFP28-100G"},
			"E1/6":  {NOSName: "1/6", BaseNOSName: "Ethernet20", Label: "6", Profile: "QSFP28-100G"},
			"E1/7":  {NOSName: "1/7", BaseNOSName: "Ethernet24", Label: "7", Profile: "QSFP28-100G"},
			"E1/8":  {NOSName: "1/8", BaseNOSName: "Ethernet28", Label: "8", Profile: "QSFP28-100G"},
			"E1/9":  {NOSName: "1/9", BaseNOSName: "Ethernet32", Label: "9", Profile: "QSFP28-100G"},
			"E1/10": {NOSName: "1/10", BaseNOSName: "Ethernet36", Label: "10", Profile: "QSFP28-100G"},
			"E1/11": {NOSName: "1/11", BaseNOSName: "Ethernet40", Label: "11", Profile: "QSFP28-100G"},
			"E1/12": {NOSName: "1/12", BaseNOSName: "Ethernet44", Label: "12", Profile: "QSFP28-100G"},
			"E1/13": {NOSName: "1/13", BaseNOSName: "Ethernet48", Label: "13", Profile: "QSFP28-100G"},
			"E1/14": {NOSName: "1/14", BaseNOSName: "Ethernet52", Label: "14", Profile: "QSFP28-100G"},
			"E1/15": {NOSName: "1/15", BaseNOSName: "Ethernet56", Label: "15", Profile: "QSFP28-100G"},
			"E1/16": {NOSName: "1/16", BaseNOSName: "Ethernet60", Label: "16", Profile: "QSFP28-100G"},
			"E1/17": {NOSName: "1/17", BaseNOSName: "Ethernet64", Label: "17", Profile: "QSFP28-100G"},
			"E1/18": {NOSName: "1/18", BaseNOSName: "Ethernet68", Label: "18", Profile: "QSFP28-100G"},
			"E1/19": {NOSName: "1/19", BaseNOSName: "Ethernet72", Label: "19", Profile: "QSFP28-100G"},
			"E1/20": {NOSName: "1/20", BaseNOSName: "Ethernet76", Label: "20", Profile: "QSFP28-100G"},
			"E1/21": {NOSName: "1/21", BaseNOSName: "Ethernet80", Label: "21", Profile: "QSFP28-100G"},
			"E1/22": {NOSName: "1/22", BaseNOSName: "Ethernet84", Label: "22", Profile: "QSFP28-100G"},
			"E1/23": {NOSName: "1/23", BaseNOSName: "Ethernet88", Label: "23", Profile: "QSFP28-100G"},
			"E1/24": {NOSName: "1/24", BaseNOSName: "Ethernet92", Label: "24", Profile: "QSFP28-100G"},
			"E1/25": {NOSName: "1/25", BaseNOSName: "Ethernet96", Label: "25", Profile: "QSFP28-100G"},
			"E1/26": {NOSName: "1/26", BaseNOSName: "Ethernet100", Label: "26", Profile: "QSFP28-100G"},
			"E1/27": {NOSName: "1/27", BaseNOSName: "Ethernet104", Label: "27", Profile: "QSFP28-100G"},
			"E1/28": {NOSName: "1/28", BaseNOSName: "Ethernet108", Label: "28", Profile: "QSFP28-100G"},
			"E1/29": {NOSName: "1/29", BaseNOSName: "Ethernet112", Label: "29", Profile: "QSFP28-100G"},
			"E1/30": {NOSName: "1/30", BaseNOSName: "Ethernet116", Label: "30", Profile: "QSFP28-100G"},
			"E1/31": {NOSName: "1/31", BaseNOSName: "Ethernet120", Label: "31", Profile: "QSFP28-100G"},
			"E1/32": {NOSName: "1/32", BaseNOSName: "Ethernet124", Label: "32", Profile: "QSFP28-100G"}, // 32x QSFP28-100G
			"E1/33": {NOSName: "Ethernet128", Label: "33", Profile: "SFP28-10G"},                        // 1x SFP28-10G
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP28-10G": {
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
						"1x50G":  {Offsets: []string{"0"}},
						"2x50G":  {Offsets: []string{"0", "2"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
					},
				},
			},
		},
	},
}
