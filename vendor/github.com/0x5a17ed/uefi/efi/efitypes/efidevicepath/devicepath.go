// Copyright (c) 2022 Arthur Skowronek <0x5a17ed@tuta.io> and contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// <https://www.apache.org/licenses/LICENSE-2.0>
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package efidevicepath

import (
	"fmt"
	"io"
	"strings"

	"github.com/0x5a17ed/itkit"
	"github.com/0x5a17ed/itkit/iters/sliceit"

	"github.com/0x5a17ed/uefi/efi/efireader"
)

// DevicePathType is used in Head to distinguish individual
// Device Path node types.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009325>
type DevicePathType uint8

const (
	_ DevicePathType = iota

	// HardwareType describes a device that is attached to the
	// resource domain of a system.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009458>
	HardwareType

	// ACPIType contains ACPI Device IDs that represent a
	// device’s Plug and Play Hardware ID and its corresponding
	// unique persistent ID.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009807>
	ACPIType

	// MessagingType describes the connection of a device outside
	// the resource domain of the system.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1010040>
	MessagingType

	// MediaType describes the portion of a medium that is being
	// abstracted by a boot service.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1352757>
	MediaType

	// BIOSBootType describes the booting of non-EFI-aware
	// operating systems.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1376904>
	BIOSBootType

	// EndOfPathType terminates a Hardware Device Path node.
	EndOfPathType DevicePathType = 0x7F
)

// DevicePathSubType varies depending on the DevicePathType value.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009325>
type DevicePathSubType uint8

// Head provides generic path/location information
// concerning a physical device or logical device.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009264>
type Head struct {
	Type    DevicePathType
	SubType DevicePathSubType

	// Length describes the size of this structure including
	// variable length data in bytes. Length is always 4 + n bytes.
	Length uint16
}

func (h *Head) Is(t DevicePathType, st DevicePathSubType) bool {
	return h.Type == t && h.SubType == st
}

type DevicePath interface {
	io.ReaderFrom

	GetHead() *Head

	// Text returns a text representation of a Device Path.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1012867>
	Text() string
}

// DevicePaths defines the programmatic path to a device.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009325>
type DevicePaths []DevicePath

func text(iter itkit.Iterator[DevicePath]) (out string, last DevicePath) {
	var b strings.Builder
	if !iter.Next() {
		return
	}

	last = iter.Value()
	if last.GetHead().Type == EndOfPathType {
		return
	}
	b.WriteString(iter.Value().Text())

	for iter.Next() {
		last = iter.Value()
		if last.GetHead().Type == EndOfPathType {
			break
		}
		b.WriteString("/")
		b.WriteString(iter.Value().Text())
	}
	out = b.String()
	return
}

func (p *DevicePaths) AllText() (out []string) {
	if p != nil {
		iter := sliceit.In(*p)
		for {
			item, last := text(iter)
			out = append(out, item)

			if last == nil || last.GetHead().Is(EndOfPathType, 255) {
				break
			}
		}
	}
	return
}

func (p *DevicePaths) ReadFrom(r io.Reader) (n int64, err error) {
	var quit bool

	fr := efireader.NewFieldReader(r, &n)
	for !quit {
		sfr := efireader.NewFieldReader(fr, nil)

		var head Head
		if err = sfr.ReadFields(&head); err != nil {
			return fr.Offset(), fmt.Errorf("head: %w", err)
		}

		bodyReader := io.LimitReader(sfr, int64(head.Length-4))

		var d DevicePath
		switch head.Type {
		case HardwareType:
			d, err = ParseHardwareDevicePath(bodyReader, head)
		case ACPIType:
			d, err = ParseACPIDevicePath(bodyReader, head)
		case MessagingType:
			d, err = ParseMessagingDevicePath(bodyReader, head)
		case MediaType:
			d, err = ParseMediaDevicePath(bodyReader, head)
		case BIOSBootType:
			d, err = ParseBIOSDevicePath(bodyReader, head)
		case EndOfPathType:
			d = &EndOfPath{head}
			quit = head.SubType == EndEntireSubType
		default:
			d, err = ParseUnrecognizedDevicePath(bodyReader, head)
		}

		if err != nil {
			return
		}
		*p = append(*p, d)

		if left := int64(head.Length) - sfr.Offset(); left > 0 {
			_, err = io.CopyN(io.Discard, sfr, left)
			if err != nil {
				return fr.Offset(), fmt.Errorf("copy: %w", err)
			}
		}
	}
	return
}
