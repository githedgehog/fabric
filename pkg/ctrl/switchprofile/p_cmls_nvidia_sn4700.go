// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

// Spectrum-3.
// Active, Cumulus 5.1.0 or later (Cumulus 4.2 notes mention support since 4.2.1).
// 32x QSFP-DD at up to 400G. All ports can be broken out to 2x or 4x, only the top 16 (odd) ports can be broken out to
// 8x and that disables the adjacent port. Max 128 ports.
// 400G DR4 transceivers need RS FEC.
// Intel x86 2.2GHz quad core CPU, 16GB RAM.
var CumulusNvidiaSN4700 = wiringapi.SwitchProfile{
	Name: "nvidia-sn4700",
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "NVIDIA SN4700",
		SwitchSilicon: SiliconNVIDIASpectrum3,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          true,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          true,
			ESLAG:         true,
			ECMPRoCEQPN:   false, // TODO verify
		},
		Notes:    "Breaking out an odd port into 8 ports disables the even port next to it. Auto-negotiation isn't supported with 400G transceivers, speed must be set explicitly. All ports support up to 12W transceivers",
		NOSType:  meta.NOSTypeCumulusMlx,
		Platform: "x86-64-nv-sn4700", // TODO verify the platform string ONIE reports, likely "x86_64-mlnx_msn4700-r0" (SONiC sonic-buildimage device/mellanox)
		MaxPorts: 128,
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "eth0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "swp1", Label: "1", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/2":  {NOSName: "swp2", Label: "2", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/3":  {NOSName: "swp3", Label: "3", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/4":  {NOSName: "swp4", Label: "4", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/5":  {NOSName: "swp5", Label: "5", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/6":  {NOSName: "swp6", Label: "6", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/7":  {NOSName: "swp7", Label: "7", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/8":  {NOSName: "swp8", Label: "8", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/9":  {NOSName: "swp9", Label: "9", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/10": {NOSName: "swp10", Label: "10", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/11": {NOSName: "swp11", Label: "11", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/12": {NOSName: "swp12", Label: "12", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/13": {NOSName: "swp13", Label: "13", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/14": {NOSName: "swp14", Label: "14", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/15": {NOSName: "swp15", Label: "15", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/16": {NOSName: "swp16", Label: "16", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/17": {NOSName: "swp17", Label: "17", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/18": {NOSName: "swp18", Label: "18", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/19": {NOSName: "swp19", Label: "19", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/20": {NOSName: "swp20", Label: "20", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/21": {NOSName: "swp21", Label: "21", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/22": {NOSName: "swp22", Label: "22", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/23": {NOSName: "swp23", Label: "23", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/24": {NOSName: "swp24", Label: "24", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/25": {NOSName: "swp25", Label: "25", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/26": {NOSName: "swp26", Label: "26", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/27": {NOSName: "swp27", Label: "27", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/28": {NOSName: "swp28", Label: "28", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/29": {NOSName: "swp29", Label: "29", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/30": {NOSName: "swp30", Label: "30", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
			"E1/31": {NOSName: "swp31", Label: "31", Profile: "QSFPDD-400G" + wiringapi.OddPortProfileSuffix},
			"E1/32": {NOSName: "swp32", Label: "32", Profile: "QSFPDD-400G" + wiringapi.EvenPortProfileSuffix},
		},
		// TODO verify, Cumulus docs list 8x1G, 8x10G and 8x25G for the odd ports which are missing here
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"QSFPDD-400G" + wiringapi.OddPortProfileSuffix: {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x400G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x1G":   {Offsets: []string{"0"}},
						"1x10G":  {Offsets: []string{"0"}},
						"1x25G":  {Offsets: []string{"0"}},
						"1x40G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"1x200G": {Offsets: []string{"0"}},
						"1x400G": {Offsets: []string{"0"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"2x100G": {Offsets: []string{"0", "1"}},
						"2x200G": {Offsets: []string{"0", "1"}},
						"4x1G":   {Offsets: []string{"0", "1", "2", "3"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x50G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x100G": {Offsets: []string{"0", "1", "2", "3"}},
						"8x50G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
			"QSFPDD-400G" + wiringapi.EvenPortProfileSuffix: {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x400G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x1G":   {Offsets: []string{"0"}},
						"1x10G":  {Offsets: []string{"0"}},
						"1x25G":  {Offsets: []string{"0"}},
						"1x40G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"1x200G": {Offsets: []string{"0"}},
						"1x400G": {Offsets: []string{"0"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"2x100G": {Offsets: []string{"0", "1"}},
						"2x200G": {Offsets: []string{"0", "1"}},
						"4x1G":   {Offsets: []string{"0", "1", "2", "3"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x50G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x100G": {Offsets: []string{"0", "1", "2", "3"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
		},
	},
}
