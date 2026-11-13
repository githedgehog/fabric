// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

// Spectrum-3. It's the 64x 200G QSFP56 SN4600, not the 64x 100G QSFP28 SN4600C (the currently orderable MSN4600 SKUs
// are SN4600C).
// End of Life (EOL notice LCR-000783), Cumulus 5.1.0 or later.
// 64x QSFP56 (NRZ and PAM4) at up to 200G. All ports can be broken out to 2x, only the odd ports can be broken out to
// 4x and that disables the adjacent port. The HW manual says "above or below" and Cumulus docs say "the first and third
// or second and fourth rows". Max 128 ports.
// PAM4 passive copper and breakout cables are supported up to 2.5m.
// The HW manual specifications table says 64x QSFP28 and 12.8Tb/s while its introduction says 64x QSFP56 and 25.6Tb/s.
// Intel x86 2.2GHz quad core CPU, 16GB RAM.
var CumulusNvidiaSN4600 = wiringapi.SwitchProfile{
	Name: "nvidia-sn4600",
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "NVIDIA SN4600",
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
		Notes:    "Breaking out an odd port into 4 ports disables the even port next to it. All ports support up to 5W transceivers, ports 49-50, 53-54, 57-58 and 61-62 up to 6.5W",
		NOSType:  meta.NOSTypeCumulusMlx,
		Platform: "x86-64-nv-sn4600", // TODO verify the platform string ONIE reports, likely "x86_64-mlnx_msn4600-r0" (SONiC sonic-buildimage device/mellanox)
		MaxPorts: 128,
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "eth0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "swp1", Label: "1", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/2":  {NOSName: "swp2", Label: "2", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/3":  {NOSName: "swp3", Label: "3", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/4":  {NOSName: "swp4", Label: "4", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/5":  {NOSName: "swp5", Label: "5", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/6":  {NOSName: "swp6", Label: "6", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/7":  {NOSName: "swp7", Label: "7", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/8":  {NOSName: "swp8", Label: "8", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/9":  {NOSName: "swp9", Label: "9", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/10": {NOSName: "swp10", Label: "10", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/11": {NOSName: "swp11", Label: "11", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/12": {NOSName: "swp12", Label: "12", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/13": {NOSName: "swp13", Label: "13", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/14": {NOSName: "swp14", Label: "14", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/15": {NOSName: "swp15", Label: "15", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/16": {NOSName: "swp16", Label: "16", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/17": {NOSName: "swp17", Label: "17", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/18": {NOSName: "swp18", Label: "18", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/19": {NOSName: "swp19", Label: "19", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/20": {NOSName: "swp20", Label: "20", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/21": {NOSName: "swp21", Label: "21", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/22": {NOSName: "swp22", Label: "22", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/23": {NOSName: "swp23", Label: "23", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/24": {NOSName: "swp24", Label: "24", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/25": {NOSName: "swp25", Label: "25", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/26": {NOSName: "swp26", Label: "26", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/27": {NOSName: "swp27", Label: "27", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/28": {NOSName: "swp28", Label: "28", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/29": {NOSName: "swp29", Label: "29", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/30": {NOSName: "swp30", Label: "30", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/31": {NOSName: "swp31", Label: "31", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/32": {NOSName: "swp32", Label: "32", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/33": {NOSName: "swp33", Label: "33", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/34": {NOSName: "swp34", Label: "34", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/35": {NOSName: "swp35", Label: "35", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/36": {NOSName: "swp36", Label: "36", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/37": {NOSName: "swp37", Label: "37", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/38": {NOSName: "swp38", Label: "38", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/39": {NOSName: "swp39", Label: "39", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/40": {NOSName: "swp40", Label: "40", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/41": {NOSName: "swp41", Label: "41", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/42": {NOSName: "swp42", Label: "42", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/43": {NOSName: "swp43", Label: "43", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/44": {NOSName: "swp44", Label: "44", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/45": {NOSName: "swp45", Label: "45", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/46": {NOSName: "swp46", Label: "46", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/47": {NOSName: "swp47", Label: "47", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/48": {NOSName: "swp48", Label: "48", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/49": {NOSName: "swp49", Label: "49", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/50": {NOSName: "swp50", Label: "50", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/51": {NOSName: "swp51", Label: "51", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/52": {NOSName: "swp52", Label: "52", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/53": {NOSName: "swp53", Label: "53", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/54": {NOSName: "swp54", Label: "54", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/55": {NOSName: "swp55", Label: "55", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/56": {NOSName: "swp56", Label: "56", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/57": {NOSName: "swp57", Label: "57", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/58": {NOSName: "swp58", Label: "58", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/59": {NOSName: "swp59", Label: "59", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/60": {NOSName: "swp60", Label: "60", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/61": {NOSName: "swp61", Label: "61", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/62": {NOSName: "swp62", Label: "62", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
			"E1/63": {NOSName: "swp63", Label: "63", Profile: "QSFP56-200G" + wiringapi.OddPortProfileSuffix},
			"E1/64": {NOSName: "swp64", Label: "64", Profile: "QSFP56-200G" + wiringapi.EvenPortProfileSuffix},
		},
		// TODO verify, Cumulus docs list 1x1G, 1x10G and 1x40G which are missing here, 2x1G isn't documented and 2x10G and
		// 2x25G are only listed by SONiC
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"QSFP56-200G" + wiringapi.OddPortProfileSuffix: {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x200G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x25G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"1x200G": {Offsets: []string{"0"}},
						"2x1G":   {Offsets: []string{"0", "1"}},
						"2x10G":  {Offsets: []string{"0", "1"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"2x100G": {Offsets: []string{"0", "1"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x50G":  {Offsets: []string{"0", "1", "2", "3"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
			"QSFP56-200G" + wiringapi.EvenPortProfileSuffix: {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x200G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x25G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"1x200G": {Offsets: []string{"0"}},
						"2x1G":   {Offsets: []string{"0", "1"}},
						"2x10G":  {Offsets: []string{"0", "1"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"2x100G": {Offsets: []string{"0", "1"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
		},
	},
}
