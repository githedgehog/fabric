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

	BIOSBootSpecSubType
)

// BIOSBootSpecPath is used to describe the booting of non-EFI-aware operating systems.
//
// <https://uefi.org/specs/UEFI/2.9_A/10_Protocols_Device_Path_Protocol.html#bios-boot-specification-device-path-1>
type BIOSBootSpecPath struct {
	Head

	// DeviceType specifies an identification number that
	// describes what type of device this is as defined by the
	// BIOS Boot Specification.
	DeviceType uint16

	// StatusFlag as defined by the BIOS Boot Specification.
	StatusFlag uint16

	// Description is a zero-terminated string that describes
	// this device to a user.
	Description []byte
}

func (p *BIOSBootSpecPath) GetHead() *Head {
	return &p.Head
}

func (p *BIOSBootSpecPath) Text() string {
	return fmt.Sprintf(
		"BBS(%d,%s,%x)",
		p.DeviceType,
		formatString(efireader.ASCIIZBytesToString(p.Description)),
		p.StatusFlag,
	)
}

func (p *BIOSBootSpecPath) ReadFrom(r io.Reader) (n int64, err error) {
	fr := efireader.NewFieldReader(r, &n)

	err = fr.ReadFields(&p.DeviceType, &p.StatusFlag)
	if err != nil {
		return
	}

	p.Description, err = efireader.ReadASCIINullBytes(fr)

	return
}

func ParseBIOSDevicePath(f io.Reader, h Head) (p DevicePath, err error) {
	switch h.SubType {
	case BIOSBootSpecSubType:
		p = &BIOSBootSpecPath{Head: h}
	default:
		p = &UnrecognizedDevicePath{Head: h}
	}

	if _, err := p.ReadFrom(f); err != nil {
		return nil, err
	}
	return
}
