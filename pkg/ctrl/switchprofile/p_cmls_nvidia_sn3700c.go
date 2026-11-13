// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

// Spectrum-2.
// End of Life, Cumulus 4.0.0 or later.
// 32x QSFP28 (NRZ only) at up to 100G. All ports can be broken out to 4x or 2x, there are no blocking requirements. Max
// 128 ports.
// 4x10G DAC and AOC breakout cables need /etc/cumulus/ports.conf changes and a switchd restart per the HW manual.
// Intel x86 2.2GHz CPU, 8GB RAM.
var CumulusNvidiaSN3700c = wiringapi.SwitchProfile{
	Name: "nvidia-sn3700c",
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "NVIDIA SN3700C",
		SwitchSilicon: SiliconNVIDIASpectrum2,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          true,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          true,
			ESLAG:         true,
			ECMPRoCEQPN:   false, // TODO verify
		},
		Notes:    "Ports 1, 2, 31 and 32 support up to 5W transceivers, ports 3-30 up to 3.5W.",
		NOSType:  meta.NOSTypeCumulusMlx,
		Platform: "x86-64-nv-sn3700c", // TODO verify the platform string ONIE reports, likely "x86_64-mlnx_msn3700c-r0" (SONiC sonic-buildimage device/mellanox)
		MaxPorts: 128,
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "eth0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "swp1", Label: "1", Profile: "QSFP28-100G"},
			"E1/2":  {NOSName: "swp2", Label: "2", Profile: "QSFP28-100G"},
			"E1/3":  {NOSName: "swp3", Label: "3", Profile: "QSFP28-100G"},
			"E1/4":  {NOSName: "swp4", Label: "4", Profile: "QSFP28-100G"},
			"E1/5":  {NOSName: "swp5", Label: "5", Profile: "QSFP28-100G"},
			"E1/6":  {NOSName: "swp6", Label: "6", Profile: "QSFP28-100G"},
			"E1/7":  {NOSName: "swp7", Label: "7", Profile: "QSFP28-100G"},
			"E1/8":  {NOSName: "swp8", Label: "8", Profile: "QSFP28-100G"},
			"E1/9":  {NOSName: "swp9", Label: "9", Profile: "QSFP28-100G"},
			"E1/10": {NOSName: "swp10", Label: "10", Profile: "QSFP28-100G"},
			"E1/11": {NOSName: "swp11", Label: "11", Profile: "QSFP28-100G"},
			"E1/12": {NOSName: "swp12", Label: "12", Profile: "QSFP28-100G"},
			"E1/13": {NOSName: "swp13", Label: "13", Profile: "QSFP28-100G"},
			"E1/14": {NOSName: "swp14", Label: "14", Profile: "QSFP28-100G"},
			"E1/15": {NOSName: "swp15", Label: "15", Profile: "QSFP28-100G"},
			"E1/16": {NOSName: "swp16", Label: "16", Profile: "QSFP28-100G"},
			"E1/17": {NOSName: "swp17", Label: "17", Profile: "QSFP28-100G"},
			"E1/18": {NOSName: "swp18", Label: "18", Profile: "QSFP28-100G"},
			"E1/19": {NOSName: "swp19", Label: "19", Profile: "QSFP28-100G"},
			"E1/20": {NOSName: "swp20", Label: "20", Profile: "QSFP28-100G"},
			"E1/21": {NOSName: "swp21", Label: "21", Profile: "QSFP28-100G"},
			"E1/22": {NOSName: "swp22", Label: "22", Profile: "QSFP28-100G"},
			"E1/23": {NOSName: "swp23", Label: "23", Profile: "QSFP28-100G"},
			"E1/24": {NOSName: "swp24", Label: "24", Profile: "QSFP28-100G"},
			"E1/25": {NOSName: "swp25", Label: "25", Profile: "QSFP28-100G"},
			"E1/26": {NOSName: "swp26", Label: "26", Profile: "QSFP28-100G"},
			"E1/27": {NOSName: "swp27", Label: "27", Profile: "QSFP28-100G"},
			"E1/28": {NOSName: "swp28", Label: "28", Profile: "QSFP28-100G"},
			"E1/29": {NOSName: "swp29", Label: "29", Profile: "QSFP28-100G"},
			"E1/30": {NOSName: "swp30", Label: "30", Profile: "QSFP28-100G"},
			"E1/31": {NOSName: "swp31", Label: "31", Profile: "QSFP28-100G"},
			"E1/32": {NOSName: "swp32", Label: "32", Profile: "QSFP28-100G"},
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"QSFP28-100G": { // TODO verify, 2x25G isn't listed in the HW manual and Cumulus docs
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x100G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x1G":   {Offsets: []string{"0"}},
						"1x10G":  {Offsets: []string{"0"}},
						"1x25G":  {Offsets: []string{"0"}},
						"1x40G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"4x1G":   {Offsets: []string{"0", "1", "2", "3"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
		},
	},
}
