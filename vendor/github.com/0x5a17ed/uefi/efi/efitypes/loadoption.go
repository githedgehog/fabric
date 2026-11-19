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

package efitypes

import (
	"fmt"
	"io"

	"github.com/0x5a17ed/uefi/efi/efireader"
	"github.com/0x5a17ed/uefi/efi/efitypes/efidevicepath"
)

// Attributes describes options for a LoadOption.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G7.1344647>
type Attributes uint32

const (
	// ActiveAttribute determines whenever the given load option
	// is enabled or disabled.
	ActiveAttribute Attributes = 0x00000001

	// ForceReconnectAttribute forces all UEFI drivers
	// in the system to be disconnected and reconnected after the
	// last Driver#### load option is processed.
	ForceReconnectAttribute Attributes = 0x00000002

	// HiddenAttribute hides the load option in any menu
	// provided by the boot manager.
	HiddenAttribute Attributes = 0x00000008

	// CategoryAttribute is a bit-mask describing a
	// subfield of Attributes that provides details to
	// the boot manager to describe how it should group LoadOption
	// entries.
	CategoryAttribute Attributes = 0x00001F00

	// CategoryBootAttribute indicates the LoadOption is
	// meant to be part of the normal boot processing.
	CategoryBootAttribute Attributes = 0x00000000

	// CategoryAppAttribute indicates the LoadOption is
	// an executable which is not part of the normal boot
	// processing but can be optionally chosen for execution if
	// boot menu is provided, or via Hot Keys.
	CategoryAppAttribute Attributes = 0x00000100
)

// LoadOption describes an UEFI application being loaded and executed by
// the Boot Manager.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G6.999491>
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G7.1344647>
type LoadOption struct {
	// Attributes are the attributes for this load option entry.  Not
	// to be confused with EFI Variable attributes.
	Attributes Attributes

	// FilePathListLength describes the length in bytes of the FilePathList.
	FilePathListLength uint16

	// Description is the user readable description for the load
	// option in utf16 encoding.  This field ends with a Null character.
	Description []byte

	// FilePathList is an array of UEFI device paths.
	//
	// The first element of the array is a device path that
	// describes the device and location of the Image for this
	// load option and is specific to the device type.
	//
	// Other device paths may optionally exist in this slice,
	// but their usage is OSV specific.
	FilePathList efidevicepath.DevicePaths

	// OptionalData is a binary data buffer that is passed to the
	// loaded image.  If the field is zero bytes long, a NULL
	// pointer is passed to the loaded image.
	OptionalData []byte
}

// DescriptionString returns the Description field decoded as a string.
func (lo *LoadOption) DescriptionString() string {
	return efireader.UTF16ZBytesToString(lo.Description)
}

func (lo *LoadOption) ReadFrom(r io.Reader) (n int64, err error) {
	fr := efireader.NewFieldReader(r, &n)

	err = fr.ReadFields(&lo.Attributes, &lo.FilePathListLength)
	if err != nil {
		err = fmt.Errorf("LoadOption: %w", err)
		return
	}

	lo.Description, err = efireader.ReadUTF16NullBytes(fr)
	if err != nil {
		err = fmt.Errorf("LoadOption/Description: %w", err)
		return
	}

	if lo.FilePathListLength > 0 {
		r := io.LimitReader(fr, int64(lo.FilePathListLength))
		if _, err = lo.FilePathList.ReadFrom(r); err != nil {
			err = fmt.Errorf("LoadOption/FilepathList: %w", err)
			return
		}
	}

	lo.OptionalData, err = io.ReadAll(fr)

	return
}
