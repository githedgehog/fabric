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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var EdgecoreDCS501 = wiringapi.SwitchProfile{
	ObjectMeta: metav1.ObjectMeta{
		Name: "edgecore-dcs501",
	},
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName: "Edgecore DCS501",
		OtherNames:  []string{"Edgecore AS7712-32X"},
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: false,
			VXLAN:         false,
			ACLs:          true,
		},
		NOSType:  meta.NOSTypeSONiCBCMBase,
		Platform: "x86_64-accton_as7712_32x-r0",
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
		},

		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
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
