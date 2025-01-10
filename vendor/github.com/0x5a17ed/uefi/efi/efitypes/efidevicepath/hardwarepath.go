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

	// PCISubType defines the path to the PCI configuration space
	// address for a PCI device.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009481>
	PCISubType

	// PCCARDSubType has no other documentation apart from its
	// binary structure.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009549>
	PCCARDSubType

	// MemoryMappedSubType has no other documentation apart from
	// its binary structure.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009609>
	MemoryMappedSubType

	// VendorHardwareSubType is a vendor-defined hardware
	// Device Paths.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009690>
	VendorHardwareSubType

	// ControllerSubType has no other documentation apart from
	// its binary structure.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1009756>
	ControllerSubType

	// BMCSubType defines the path to a
	// Baseboard Management Controller (BMC) host interface.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1355840>
	BMCSubType
)

// PCIDevicePath is a PCI Device Path.
//
// Section 10.3.2.1 "PCI Device Path"
type PCIDevicePath struct {
	Head
	Function byte
	Device   byte
}

func (p *PCIDevicePath) GetHead() *Head {
	return &p.Head
}

func (p *PCIDevicePath) Text() string {
	return fmt.Sprintf("Pci(%d,%d)", p.Function, p.Device)
}

func (p *PCIDevicePath) ReadFrom(r io.Reader) (n int64, err error) {
	return efireader.ReadFields(r, &p.Function, &p.Device)
}

func ParseHardwareDevicePath(f io.Reader, h Head) (p DevicePath, err error) {
	switch h.SubType {
	case PCISubType:
		p = &PCIDevicePath{Head: h}
	default:
		p = &UnrecognizedDevicePath{Head: h}
	}

	if _, err := p.ReadFrom(f); err != nil {
		return nil, err
	}
	return
}
