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

	"github.com/0x5a17ed/uefi/efi/efireader"
)

const (
	_ DevicePathSubType = iota

	ACPISubType
)

func intToEISA(v int) string {
	vendor := v & 0xffff
	vendor1 := ((vendor >> 10) & 0x1f) + '@'
	vendor2 := ((vendor >> 5) & 0x1f) + '@'
	vendor3 := ((vendor >> 0) & 0x1f) + '@'

	device := v >> 16

	return fmt.Sprintf("%c%c%c%04X", vendor1, vendor2, vendor3, device)
}

// ACPIPath is a ACPI Device Path.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009828A>
type ACPIPath struct {
	Head
	HID uint32
	UID uint32
}

func (p *ACPIPath) GetHead() *Head {
	return &p.Head
}

func (p *ACPIPath) Text() string {
	return fmt.Sprintf("ACPI(%s,%d)", intToEISA(int(p.HID)), p.UID)
}

func (p *ACPIPath) ReadFrom(r io.Reader) (n int64, err error) {
	return efireader.ReadFields(r, &p.HID, &p.UID)
}

func ParseACPIDevicePath(f io.Reader, h Head) (p DevicePath, err error) {
	switch h.SubType {
	case ACPISubType:
		p = &ACPIPath{Head: h}
	default:
		p = &UnrecognizedDevicePath{Head: h}
	}

	if _, err := p.ReadFrom(f); err != nil {
		return nil, err
	}
	return
}
