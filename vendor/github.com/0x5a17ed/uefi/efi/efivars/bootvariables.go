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

package efivars

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/0x5a17ed/itkit"
	"github.com/0x5a17ed/itkit/itlib"

	"github.com/0x5a17ed/uefi/efi/efitypes"
	"github.com/0x5a17ed/uefi/efi/efivario"
)

const (
	BootNextName    = "BootNext"
	BootCurrentName = "BootCurrent"
	BootOrderName   = "BootOrder"
)

var (
	bootOptionRegexp = regexp.MustCompile(`^Boot([\da-fA-F]{4})$`)
)

var (
	// BootNext specifies the first boot option on the next boot.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G7.1346720>
	BootNext = Variable[uint16]{
		name:         BootNextName,
		guid:         GlobalVariable,
		defaultAttrs: defaultAttrs,
		marshal:      primitiveMarshaller[uint16],
		unmarshal:    primitiveUnmarshaller[uint16],
	}

	// BootCurrent defines the Boot#### option that was selected
	// on the current boot.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G7.1346720>
	BootCurrent = Variable[uint16]{
		name:         BootCurrentName,
		guid:         GlobalVariable,
		defaultAttrs: defaultAttrs,
		marshal:      primitiveMarshaller[uint16],
		unmarshal:    primitiveUnmarshaller[uint16],
	}

	// BootOrder is an ordered list of the Boot#### options.
	//
	// The first element in the array is the value for the first
	// logical boot option, the second element is the value for
	// the second logical boot option, etc. The BootOrder order
	// list is used by the firmware’s boot manager as the default
	// boot order.
	//
	// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G7.1346720>
	BootOrder = Variable[[]uint16]{
		name:         BootOrderName,
		guid:         GlobalVariable,
		defaultAttrs: defaultAttrs,
		unmarshal:    sliceUnmarshaller[uint16],
		marshal:      sliceMarshaller[uint16],
	}
)

// Boot returns an EFI Variable pointing to the boot LoadOption
// for the given index.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#G7.1346720>
func Boot(i uint16) Variable[*efitypes.LoadOption] {
	return Variable[*efitypes.LoadOption]{
		name:      fmt.Sprintf("Boot%04X", i),
		guid:      GlobalVariable,
		unmarshal: structUnmarshaller[efitypes.LoadOption],
	}
}

// BootEntry describes a Boot efitypes.LoadOption value.
type BootEntry struct {
	Index    uint16
	Variable Variable[*efitypes.LoadOption]
}

// BootEntryIterator is an iterator yielding currently configured
// Boot efitypes.LoadOption values.
type BootEntryIterator struct {
	pit efivario.VariableNameIterator
	fit itkit.Iterator[*BootEntry]
}

func (it *BootEntryIterator) Close() error                     { return it.pit.Close() }
func (it *BootEntryIterator) Err() error                       { return it.pit.Err() }
func (it *BootEntryIterator) Iter() itkit.Iterator[*BootEntry] { return it.fit }
func (it *BootEntryIterator) Value() *BootEntry                { return it.fit.Value() }
func (it *BootEntryIterator) Next() bool                       { return it.fit.Next() }

func BootIterator(ctx efivario.Context) (*BootEntryIterator, error) {
	pit, err := ctx.VariableNames()
	if err != nil {
		return nil, err
	}

	fit := itlib.Map(pit.Iter(), func(vn efivario.VariableNameItem) *BootEntry {
		if vn.GUID != GlobalVariable {
			return nil
		}

		match := bootOptionRegexp.FindStringSubmatch(vn.Name)
		if match == nil {
			return nil
		}

		value, err := strconv.ParseInt(match[1], 16, 64)
		if err != nil {
			return nil
		}

		return &BootEntry{Index: uint16(value), Variable: Boot(uint16(value))}
	})
	fit = itlib.Filter(fit, func(be *BootEntry) bool { return be != nil })

	return &BootEntryIterator{pit: pit, fit: fit}, nil
}
