// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

// Spectrum-4.
// End of Life, Cumulus 5.8.0 or later. The SN5600D (48V DC busbar) version is still in production.
// 64x OSFP at up to 800G and a bonus SFP28 port 65 at 1G/10G/25G without breakouts. Max 257 ports (256 + 1 bonus).
// All ports can be broken out to 2x or 4x, only the odd ports can be broken out to 8x and that disables the subsequent
// even port. The model specific HW manual text says odd ports while a generic caption says any port.
// Ports are numbered down each column of 4 rows.
// Spectrum-4 limits multicast packets to 512 bytes or larger.
// Intel Xeon E-2276ME CPU, 32GB RAM.
var CumulusNvidiaSN5600 = wiringapi.SwitchProfile{
	Name: "nvidia-sn5600",
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "NVIDIA SN5600",
		SwitchSilicon: SiliconNVIDIASpectrum4,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          true,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          true,
			ESLAG:         true,
			ECMPRoCEQPN:   false, // TODO verify
		},
		Notes:    "Experimental, not tested on actual hardware. Transceiver orientation of bottom two rows of cages is upside down. Breaking out an odd port into 8 ports disables the even port next to it. All OSFP ports support up to 18W transceivers.",
		NOSType:  meta.NOSTypeCumulusMlx,
		Platform: "x86_64-nv-sn5600-r0", // TODO verify the platform string ONIE reports, likely "x86_64-nvidia_sn5600-r0" (SONiC sonic-buildimage device/mellanox)
		MaxPorts: 256,
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "eth0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "swp1", Label: "1", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/2":  {NOSName: "swp2", Label: "2", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/3":  {NOSName: "swp3", Label: "3", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/4":  {NOSName: "swp4", Label: "4", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/5":  {NOSName: "swp5", Label: "5", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/6":  {NOSName: "swp6", Label: "6", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/7":  {NOSName: "swp7", Label: "7", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/8":  {NOSName: "swp8", Label: "8", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/9":  {NOSName: "swp9", Label: "9", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/10": {NOSName: "swp10", Label: "10", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/11": {NOSName: "swp11", Label: "11", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/12": {NOSName: "swp12", Label: "12", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/13": {NOSName: "swp13", Label: "13", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/14": {NOSName: "swp14", Label: "14", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/15": {NOSName: "swp15", Label: "15", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/16": {NOSName: "swp16", Label: "16", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/17": {NOSName: "swp17", Label: "17", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/18": {NOSName: "swp18", Label: "18", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/19": {NOSName: "swp19", Label: "19", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/20": {NOSName: "swp20", Label: "20", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/21": {NOSName: "swp21", Label: "21", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/22": {NOSName: "swp22", Label: "22", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/23": {NOSName: "swp23", Label: "23", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/24": {NOSName: "swp24", Label: "24", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/25": {NOSName: "swp25", Label: "25", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/26": {NOSName: "swp26", Label: "26", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/27": {NOSName: "swp27", Label: "27", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/28": {NOSName: "swp28", Label: "28", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/29": {NOSName: "swp29", Label: "29", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/30": {NOSName: "swp30", Label: "30", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/31": {NOSName: "swp31", Label: "31", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/32": {NOSName: "swp32", Label: "32", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/33": {NOSName: "swp33", Label: "33", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/34": {NOSName: "swp34", Label: "34", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/35": {NOSName: "swp35", Label: "35", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/36": {NOSName: "swp36", Label: "36", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/37": {NOSName: "swp37", Label: "37", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/38": {NOSName: "swp38", Label: "38", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/39": {NOSName: "swp39", Label: "39", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/40": {NOSName: "swp40", Label: "40", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/41": {NOSName: "swp41", Label: "41", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/42": {NOSName: "swp42", Label: "42", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/43": {NOSName: "swp43", Label: "43", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/44": {NOSName: "swp44", Label: "44", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/45": {NOSName: "swp45", Label: "45", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/46": {NOSName: "swp46", Label: "46", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/47": {NOSName: "swp47", Label: "47", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/48": {NOSName: "swp48", Label: "48", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/49": {NOSName: "swp49", Label: "49", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/50": {NOSName: "swp50", Label: "50", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/51": {NOSName: "swp51", Label: "51", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/52": {NOSName: "swp52", Label: "52", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/53": {NOSName: "swp53", Label: "53", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/54": {NOSName: "swp54", Label: "54", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/55": {NOSName: "swp55", Label: "55", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/56": {NOSName: "swp56", Label: "56", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/57": {NOSName: "swp57", Label: "57", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/58": {NOSName: "swp58", Label: "58", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/59": {NOSName: "swp59", Label: "59", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/60": {NOSName: "swp60", Label: "60", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/61": {NOSName: "swp61", Label: "61", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/62": {NOSName: "swp62", Label: "62", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/63": {NOSName: "swp63", Label: "63", Profile: "OSFP-800G" + wiringapi.OddPortProfileSuffix},
			"E1/64": {NOSName: "swp64", Label: "64", Profile: "OSFP-800G" + wiringapi.EvenPortProfileSuffix},
			"E1/65": {NOSName: "swp65", Label: "65", Profile: "SFP28-25G", Bonus: true},
		},
		// TODO verify, 8x10G isn't documented and Cumulus docs list 40G which is missing here (SONiC allows it for 1x and 2x)
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"SFP28-25G": {
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "25G",
					Supported: []string{"1G", "10G", "25G"},
				},
			},
			"OSFP-800G" + wiringapi.OddPortProfileSuffix: {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x800G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x10G":  {Offsets: []string{"0"}},
						"1x25G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"1x200G": {Offsets: []string{"0"}},
						"1x400G": {Offsets: []string{"0"}},
						"1x800G": {Offsets: []string{"0"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"2x100G": {Offsets: []string{"0", "1"}},
						"2x200G": {Offsets: []string{"0", "1"}},
						"2x400G": {Offsets: []string{"0", "1"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x50G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x100G": {Offsets: []string{"0", "1", "2", "3"}},
						"4x200G": {Offsets: []string{"0", "1", "2", "3"}},
						"8x10G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}, DisablesPortOffsets: []string{"1"}},
						"8x25G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}, DisablesPortOffsets: []string{"1"}},
						"8x50G":  {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}, DisablesPortOffsets: []string{"1"}},
						"8x100G": {Offsets: []string{"0", "1", "2", "3", "4", "5", "6", "7"}, DisablesPortOffsets: []string{"1"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
			"OSFP-800G" + wiringapi.EvenPortProfileSuffix: {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x800G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x10G":  {Offsets: []string{"0"}},
						"1x25G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"1x200G": {Offsets: []string{"0"}},
						"1x400G": {Offsets: []string{"0"}},
						"1x800G": {Offsets: []string{"0"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"2x100G": {Offsets: []string{"0", "1"}},
						"2x200G": {Offsets: []string{"0", "1"}},
						"2x400G": {Offsets: []string{"0", "1"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x50G":  {Offsets: []string{"0", "1", "2", "3"}},
						"4x100G": {Offsets: []string{"0", "1", "2", "3"}},
						"4x200G": {Offsets: []string{"0", "1", "2", "3"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
		},
	},
}
