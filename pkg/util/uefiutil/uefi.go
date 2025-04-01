// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package uefiutil

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/0x5a17ed/uefi/efi/efireader"
	"github.com/0x5a17ed/uefi/efi/efivario"
	"github.com/0x5a17ed/uefi/efi/efivars"
	"github.com/pkg/errors"
)

// This implemetation is stolen from Das Boot with light modifications
// TODO extract to library

var efiCtx = efivario.NewDefaultContext()

var errEmptyBootOrder = errors.New("uefi: boot order is empty")

// MakeONIEDefaultBootEntryAndCleanup will ensure that ONIE is the first boot
// entry in the EFI BootOrder variable.
func MakeONIEDefaultBootEntryAndCleanup() error {
	// get ONIE boot entry variable
	onieBootEntryNumber, err := FindONIEBootEntry()
	if err != nil {
		return errors.Wrapf(err, "error finding uefi ONIE boot entry")
	}

	// get the boot order variable now
	_, bootOrder, err := efivars.BootOrder.Get(efiCtx)
	if err != nil {
		return errors.Wrapf(err, "error getting BootOrder")
	}
	if len(bootOrder) == 0 {
		return errEmptyBootOrder
	}

	// see if this needs adjustment
	if bootOrder[0] == onieBootEntryNumber {
		// ONIE is already the first entry, we can stop here
		return nil
	}

	// we need to move ONIE up to the front
	// build a new boot order
	newBootOrder := []uint16{onieBootEntryNumber}
	for _, num := range bootOrder {
		if num == onieBootEntryNumber {
			continue
		}
		newBootOrder = append(newBootOrder, num)
	}

	// prepare a string that we use for logging and errors
	newBootOrderStrings := make([]string, 0, len(newBootOrder))
	for _, num := range newBootOrder {
		newBootOrderStrings = append(newBootOrderStrings, fmt.Sprintf("%04X", num))
	}
	newBootOrderStr := strings.Join(newBootOrderStrings, ",")

	// write the boot order to the EFI variable
	if err := efivars.BootOrder.Set(efiCtx, newBootOrder); err != nil {
		return errors.Wrapf(err, "error setting BootOrder to %q", newBootOrderStr)
	}
	slog.Info("uefi: successfully set EFI BootOrder variable", "bootOrder", newBootOrderStr)

	return nil
}

// FindONIEBootEntry will find the UEFI ONIE boot entry
func FindONIEBootEntry() (uint16, error) {
	bootIterator, err := efivars.BootIterator(efiCtx)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get BootIterator")
	}
	defer bootIterator.Close()

	for bootIterator.Next() {
		bootEntry := bootIterator.Value()
		_, bootEntryLoadOptions, err := bootEntry.Variable.Get(efiCtx)
		if err != nil {
			continue
		}
		desc := efireader.UTF16ZBytesToString(bootEntryLoadOptions.Description)
		if strings.Contains(desc, "ONIE") {
			return bootEntry.Index, nil
		}
	}
	if err := bootIterator.Err(); err != nil {
		return 0, errors.Wrapf(err, "BootIterator aborted")
	}

	return 0, errors.Errorf("not found")
}
