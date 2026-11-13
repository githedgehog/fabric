// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

// Spectrum (gen 1), shipped with both A0 and A1 revisions, so ESLAG is disabled (no EVPN multihoming on A0).
// End of Life (EOL notice LCR-000875), Cumulus 3.3.0 or later, still documented in Cumulus 5.18.
// Spectrum (gen 1) ACLs are limited to 1000 IPv4 and 500 IPv6 rules and the forwarding tables are smaller.
// All ports can be broken out to 2x, only the top 16 (odd) ports can be broken out to 4x and that disables the port
// below them (the subsequent even port). Max 64 ports.
// Known Cumulus 5.18 issues on Spectrum (gen 1): 25G links may not come back after a switchd restart (3071652), switchd
// crash during ISSU (3771168).
// Intel x86 1.4GHz dual core CPU, 8GB RAM.
var CumulusNvidiaSN2700 = wiringapi.SwitchProfile{
	Name: "nvidia-sn2700",
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "NVIDIA SN2700",
		SwitchSilicon: SiliconNVIDIASpectrum,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          true,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          true,
			ESLAG:         false, // EVPN multihoming needs Spectrum A1 and SN2700 shipped with both A0 and A1
			ECMPRoCEQPN:   false, // TODO verify
		},
		Notes:    "Breaking out an odd port into 4 ports disables the even port next to it. Only ports 1, 2, 31 and 32 support 5W transceivers.",
		NOSType:  meta.NOSTypeCumulusMlx,
		Platform: "x86_64-mlnx_msn2700-r0", // TODO verify, from NVIDIA ONIE CLI reference while the EEPROM reports generic "x86_64-mlnx_x86-r0"
		MaxPorts: 64,
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "eth0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "swp1", Label: "1", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/2":  {NOSName: "swp2", Label: "2", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/3":  {NOSName: "swp3", Label: "3", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/4":  {NOSName: "swp4", Label: "4", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/5":  {NOSName: "swp5", Label: "5", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/6":  {NOSName: "swp6", Label: "6", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/7":  {NOSName: "swp7", Label: "7", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/8":  {NOSName: "swp8", Label: "8", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/9":  {NOSName: "swp9", Label: "9", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/10": {NOSName: "swp10", Label: "10", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/11": {NOSName: "swp11", Label: "11", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/12": {NOSName: "swp12", Label: "12", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/13": {NOSName: "swp13", Label: "13", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/14": {NOSName: "swp14", Label: "14", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/15": {NOSName: "swp15", Label: "15", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/16": {NOSName: "swp16", Label: "16", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/17": {NOSName: "swp17", Label: "17", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/18": {NOSName: "swp18", Label: "18", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/19": {NOSName: "swp19", Label: "19", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/20": {NOSName: "swp20", Label: "20", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/21": {NOSName: "swp21", Label: "21", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/22": {NOSName: "swp22", Label: "22", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/23": {NOSName: "swp23", Label: "23", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/24": {NOSName: "swp24", Label: "24", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/25": {NOSName: "swp25", Label: "25", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/26": {NOSName: "swp26", Label: "26", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/27": {NOSName: "swp27", Label: "27", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/28": {NOSName: "swp28", Label: "28", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/29": {NOSName: "swp29", Label: "29", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/30": {NOSName: "swp30", Label: "30", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
			"E1/31": {NOSName: "swp31", Label: "31", Profile: "QSFP28-100G" + wiringapi.OddPortProfileSuffix},
			"E1/32": {NOSName: "swp32", Label: "32", Profile: "QSFP28-100G" + wiringapi.EvenPortProfileSuffix},
		},
		// TODO verify, Cumulus docs list 1x40G which is missing here, 1x50G and 2x25G aren't listed
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"QSFP28-100G" + wiringapi.OddPortProfileSuffix: {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x100G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x1G":   {Offsets: []string{"0"}},
						"1x10G":  {Offsets: []string{"0"}},
						"1x25G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"4x1G":   {Offsets: []string{"0", "1", "2", "3"}, DisablesPortOffsets: []string{"1"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}, DisablesPortOffsets: []string{"1"}},
						"4x25G":  {Offsets: []string{"0", "1", "2", "3"}, DisablesPortOffsets: []string{"1"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
			"QSFP28-100G" + wiringapi.EvenPortProfileSuffix: {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x100G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x1G":   {Offsets: []string{"0"}},
						"1x10G":  {Offsets: []string{"0"}},
						"1x25G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
		},
	},
}
