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
	"encoding/binary"
	"fmt"
	"io"

	"github.com/0x5a17ed/uefi/efi/efiguid"
	"github.com/0x5a17ed/uefi/efi/efihex"
	"github.com/0x5a17ed/uefi/efi/efireader"
)

//go:generate go run github.com/hexaflex/stringer -type=PartitionFormat,SignatureType -output mediapath_string.go

// PartitionFormat describes the partition format.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1012056>
type PartitionFormat uint8

const (
	_ PartitionFormat = iota

	// PCATPartitionFormat describes a PC-AT compatible legacy
	// MBR. Partition Start and Partition Size come from
	// PartitionStartingLBA and PartitionSizeInLBA in
	// HardDriveMediaDevicePath for the partition.
	PCATPartitionFormat

	// GUIDPartitionFormat describes a GUID Partition Table.
	GUIDPartitionFormat
)

// SignatureType describes the signature type.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1012056>
type SignatureType uint8

const (
	// NoneSignatureType means No Disk Signature.
	NoneSignatureType SignatureType = iota

	// PCATSignatureType means the 32-bit signature from
	// address 0x1b8 of the type 0x01 MBR.
	PCATSignatureType

	// GUIDSignatureType means the signature from a GUID
	// partition table.
	GUIDSignatureType
)

// HardDriveMediaDevicePath represents a partition on a hard drive.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1012056>
type HardDriveMediaDevicePath struct {
	Head

	// PartitionNumber describes the entry in a partition table,
	// starting with entry 1.  Partition number zero represents
	// the entire device.
	PartitionNumber uint32

	// PartitionStartLBA is the starting LBA of the partition on
	// the hard drive.
	PartitionStartLBA uint64

	// Size of the partition in units of Logical Blocks.
	PartitionSizeLBA uint64

	// PartitionSignature is the partition signature. Its value
	// depends on the SignatureType field.
	// 	- If SignatureType is 0, this field has to be
	// 	initialized with 16 zeroes.
	// 	- If SignatureType is 1, the MBR signature is stored
	// 	in the first 4 bytes of this field. The other 12 bytes
	// 	are initialized with zeroes.
	// 	- If SignatureType is 2, this field contains a 16 byte signature
	PartitionSignature [16]byte

	// PartitionFormat describes the format of the partition table.
	PartitionFormat PartitionFormat

	SignatureType SignatureType
}

func (p *HardDriveMediaDevicePath) GetHead() *Head {
	return &p.Head
}

func (p *HardDriveMediaDevicePath) Text() string {
	var t, sig, r string

	switch p.PartitionFormat {
	case PCATPartitionFormat:
		t = "MBR"
		sig = fmt.Sprintf("%#08x", binary.LittleEndian.Uint32(p.PartitionSignature[:]))
	case GUIDPartitionFormat:
		t = "GPT"
		sig = (efiguid.GUID)(p.PartitionSignature).String()
	}

	if p.PartitionNumber != 0 {
		r = fmt.Sprintf(",%#x,%#x", p.PartitionStartLBA, p.PartitionSizeLBA)
	}

	return fmt.Sprintf("HD(%d,%s,%s%s)", p.PartitionNumber, t, sig, r)
}

func (p *HardDriveMediaDevicePath) ReadFrom(r io.Reader) (n int64, err error) {
	return efireader.ReadFields(
		r,
		&p.PartitionNumber,
		&p.PartitionStartLBA,
		&p.PartitionSizeLBA,
		&p.PartitionSignature,
		&p.PartitionFormat,
		&p.SignatureType,
	)
}

// CDROMDevicePath defines a system partition that exists on a CD-ROM.
//
// Section 10.3.5.2
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1012169>
type CDROMDevicePath struct {
	Head

	// BootEntry number from the Boot Catalog. The Initial/Default
	// entry is defined as zero.
	BootEntry uint32

	// PartitionStart is the starting RBA of the partition on the
	// medium. CD-ROMs use Relative logical Block Addressing.
	PartitionStartRBA uint64

	// PartitionSize is the size of the partition in units of
	// Blocks, also called Sectors.
	PartitionSize uint64
}

func (p *CDROMDevicePath) ReadFrom(r io.Reader) (n int64, err error) {
	return efireader.ReadFields(r, &p.BootEntry, &p.PartitionStartRBA, &p.PartitionSize)
}

func (p *CDROMDevicePath) GetHead() *Head {
	return &p.Head
}

func (p *CDROMDevicePath) Text() string {
	return fmt.Sprintf("CDROM(%d,%x,%x)", p.BootEntry, p.PartitionStartRBA, p.PartitionSize)
}

// VendorMediaDevicePath describes a file path node.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G14.1012246>
type VendorMediaDevicePath struct {
	Head

	// VendorGUID is the Vendor-assigned GUID that defines the
	// data that follows.
	VendorGUID efiguid.GUID

	// VendorDefinedData is the Vendor-defined variable size data.
	VendorDefinedData []byte
}

func (p *VendorMediaDevicePath) Text() string {
	return fmt.Sprintf("VenMedia(%s,%s)", p.VendorGUID, efihex.EncodeToString(p.VendorDefinedData))
}

func (p *VendorMediaDevicePath) GetHead() *Head {
	return &p.Head
}

func (p *VendorMediaDevicePath) ReadFrom(r io.Reader) (n int64, err error) {
	fr := efireader.NewFieldReader(r, &n)

	if err = fr.ReadFields(&p.VendorGUID); err != nil {
		return
	}

	p.VendorDefinedData, err = io.ReadAll(fr)
	return
}

// FilePathDevicePath describes a file path node.
type FilePathDevicePath struct {
	Head

	PathName []byte
}

func (f *FilePathDevicePath) Text() string {
	return fmt.Sprintf("File(%s)", efireader.UTF16ZBytesToString(f.PathName))
}

func (p *FilePathDevicePath) GetHead() *Head {
	return &p.Head
}

func (p *FilePathDevicePath) ReadFrom(r io.Reader) (n int64, err error) {
	wrapper := efireader.NewFieldReader(r, &n)
	p.PathName, err = efireader.ReadUTF16NullBytes(wrapper)
	return
}

const (
	_ DevicePathSubType = iota

	// HardDriveSubType represents a partition on a hard drive.
	HardDriveSubType

	// CDROMSubType defines a system partition that exists on a CD-ROM.
	CDROMSubType

	// VendorMediaSubType is a vendor-defined media Device Path.
	VendorMediaSubType

	FilePathSubType
	MediaProtocolSubType

	PIWGFirmwareFileSubType
	PIWGFirmwareVolumeSubType

	RelativeOffsetRangeSubType

	RAMDiskSubType
)

func ParseMediaDevicePath(f io.Reader, h Head) (p DevicePath, err error) {
	switch h.SubType {
	case HardDriveSubType:
		p = &HardDriveMediaDevicePath{Head: h}
	case CDROMSubType:
		p = &CDROMDevicePath{Head: h}
	case VendorMediaSubType:
		p = &VendorMediaDevicePath{Head: h}
	case FilePathSubType:
		p = &FilePathDevicePath{Head: h}
	default:
		p = &UnrecognizedDevicePath{Head: h}
	}

	if _, err := p.ReadFrom(f); err != nil {
		return nil, fmt.Errorf("efi/devicepath: type %d-%d: %w", h.Type, h.SubType, err)
	}
	return
}
