// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package switchprofile

import (
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

// Spectrum (gen 1), the revision isn't documented. It's likely A1, but ESLAG is disabled until it's verified on a unit
// as EVPN multihoming isn't supported on A0.
// Active, Cumulus 5.2.0 or later. Avoid 5.16.1 on it (use 5.16.5 or later).
// Spectrum (gen 1) ACLs are limited to 1000 IPv4 and 500 IPv6 rules and the forwarding tables are smaller.
// 48x RJ45 10M/100M/1G (auto-negotiation is required at 1G) and 4x QSFP28 (NRZ only) at up to 100G.
// All QSFP28 ports can be broken out to 4x or 2x, there are no blocking requirements.
// Intel x86 dual core CPU, 448Gb/s throughput.
var CumulusNvidiaSN2201 = wiringapi.SwitchProfile{
	Name: "nvidia-sn2201",
	Spec: wiringapi.SwitchProfileSpec{
		DisplayName:   "NVIDIA SN2201",
		SwitchSilicon: SiliconNVIDIASpectrum,
		Features: wiringapi.SwitchProfileFeatures{
			Subinterfaces: true,
			ACLs:          true,
			L2VNI:         true,
			L3VNI:         true,
			RoCE:          false,
			ESLAG:         false, // TODO enable if it's Spectrum A1 (Device Version >= 16 in decode-syseeprom), EVPN multihoming isn't supported on A0
			ECMPRoCEQPN:   false, // TODO verify
		},
		Notes:    "Ports 49 and 51 support up to 5W transceivers, ports 50 and 52 up to 3.5W.",
		NOSType:  meta.NOSTypeCumulusMlx,
		Platform: "x86-64-nv-sn2201", // TODO verify the platform string ONIE reports, likely "x86_64-nvidia_sn2201-r0" (SONiC sonic-buildimage device/mellanox)
		MaxPorts: 64,
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "eth0", Management: true, OniePortName: "eth0"},
			"E1/1":  {NOSName: "swp1", Label: "1", Profile: "RJ45-1G"},
			"E1/2":  {NOSName: "swp2", Label: "2", Profile: "RJ45-1G"},
			"E1/3":  {NOSName: "swp3", Label: "3", Profile: "RJ45-1G"},
			"E1/4":  {NOSName: "swp4", Label: "4", Profile: "RJ45-1G"},
			"E1/5":  {NOSName: "swp5", Label: "5", Profile: "RJ45-1G"},
			"E1/6":  {NOSName: "swp6", Label: "6", Profile: "RJ45-1G"},
			"E1/7":  {NOSName: "swp7", Label: "7", Profile: "RJ45-1G"},
			"E1/8":  {NOSName: "swp8", Label: "8", Profile: "RJ45-1G"},
			"E1/9":  {NOSName: "swp9", Label: "9", Profile: "RJ45-1G"},
			"E1/10": {NOSName: "swp10", Label: "10", Profile: "RJ45-1G"},
			"E1/11": {NOSName: "swp11", Label: "11", Profile: "RJ45-1G"},
			"E1/12": {NOSName: "swp12", Label: "12", Profile: "RJ45-1G"},
			"E1/13": {NOSName: "swp13", Label: "13", Profile: "RJ45-1G"},
			"E1/14": {NOSName: "swp14", Label: "14", Profile: "RJ45-1G"},
			"E1/15": {NOSName: "swp15", Label: "15", Profile: "RJ45-1G"},
			"E1/16": {NOSName: "swp16", Label: "16", Profile: "RJ45-1G"},
			"E1/17": {NOSName: "swp17", Label: "17", Profile: "RJ45-1G"},
			"E1/18": {NOSName: "swp18", Label: "18", Profile: "RJ45-1G"},
			"E1/19": {NOSName: "swp19", Label: "19", Profile: "RJ45-1G"},
			"E1/20": {NOSName: "swp20", Label: "20", Profile: "RJ45-1G"},
			"E1/21": {NOSName: "swp21", Label: "21", Profile: "RJ45-1G"},
			"E1/22": {NOSName: "swp22", Label: "22", Profile: "RJ45-1G"},
			"E1/23": {NOSName: "swp23", Label: "23", Profile: "RJ45-1G"},
			"E1/24": {NOSName: "swp24", Label: "24", Profile: "RJ45-1G"},
			"E1/25": {NOSName: "swp25", Label: "25", Profile: "RJ45-1G"},
			"E1/26": {NOSName: "swp26", Label: "26", Profile: "RJ45-1G"},
			"E1/27": {NOSName: "swp27", Label: "27", Profile: "RJ45-1G"},
			"E1/28": {NOSName: "swp28", Label: "28", Profile: "RJ45-1G"},
			"E1/29": {NOSName: "swp29", Label: "29", Profile: "RJ45-1G"},
			"E1/30": {NOSName: "swp30", Label: "30", Profile: "RJ45-1G"},
			"E1/31": {NOSName: "swp31", Label: "31", Profile: "RJ45-1G"},
			"E1/32": {NOSName: "swp32", Label: "32", Profile: "RJ45-1G"},
			"E1/33": {NOSName: "swp33", Label: "33", Profile: "RJ45-1G"},
			"E1/34": {NOSName: "swp34", Label: "34", Profile: "RJ45-1G"},
			"E1/35": {NOSName: "swp35", Label: "35", Profile: "RJ45-1G"},
			"E1/36": {NOSName: "swp36", Label: "36", Profile: "RJ45-1G"},
			"E1/37": {NOSName: "swp37", Label: "37", Profile: "RJ45-1G"},
			"E1/38": {NOSName: "swp38", Label: "38", Profile: "RJ45-1G"},
			"E1/39": {NOSName: "swp39", Label: "39", Profile: "RJ45-1G"},
			"E1/40": {NOSName: "swp40", Label: "40", Profile: "RJ45-1G"},
			"E1/41": {NOSName: "swp41", Label: "41", Profile: "RJ45-1G"},
			"E1/42": {NOSName: "swp42", Label: "42", Profile: "RJ45-1G"},
			"E1/43": {NOSName: "swp43", Label: "43", Profile: "RJ45-1G"},
			"E1/44": {NOSName: "swp44", Label: "44", Profile: "RJ45-1G"},
			"E1/45": {NOSName: "swp45", Label: "45", Profile: "RJ45-1G"},
			"E1/46": {NOSName: "swp46", Label: "46", Profile: "RJ45-1G"},
			"E1/47": {NOSName: "swp47", Label: "47", Profile: "RJ45-1G"},
			"E1/48": {NOSName: "swp48", Label: "48", Profile: "RJ45-1G"},
			"E1/49": {NOSName: "swp49", Label: "49", Profile: "QSFP28-100G"},
			"E1/50": {NOSName: "swp50", Label: "50", Profile: "QSFP28-100G"},
			"E1/51": {NOSName: "swp51", Label: "51", Profile: "QSFP28-100G"},
			"E1/52": {NOSName: "swp52", Label: "52", Profile: "QSFP28-100G"},
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"RJ45-1G": { // TODO verify, HW manual and Cumulus docs say 10M and 100M are supported too
				Speed: &wiringapi.SwitchProfilePortProfileSpeed{
					Default:   "1G",
					Supported: []string{"1G"},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
			// TODO verify, Cumulus docs list only 1x40G, 1x100G, 2x50G, 4x1G, 4x10G and 4x25G (4x1G and 4x25G are
			// missing here, 1x25G, 1x50G, 2x25G and 2x40G aren't listed)
			"QSFP28-100G": {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x100G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x25G":  {Offsets: []string{"0"}},
						"1x40G":  {Offsets: []string{"0"}},
						"1x50G":  {Offsets: []string{"0"}},
						"1x100G": {Offsets: []string{"0"}},
						"2x25G":  {Offsets: []string{"0", "1"}},
						"2x40G":  {Offsets: []string{"0", "1"}},
						"2x50G":  {Offsets: []string{"0", "1"}},
						"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
					},
				},
				AutoNegAllowed: true,
				AutoNegDefault: true,
			},
		},
	},
}
