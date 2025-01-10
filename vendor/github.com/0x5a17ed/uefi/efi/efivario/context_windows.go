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

//go:build windows

package efivario

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"syscall"

	"github.com/0x5a17ed/itkit"
	"golang.org/x/sys/windows"

	"github.com/0x5a17ed/uefi/efi/efiguid"
	"github.com/0x5a17ed/uefi/efi/efireader"
	"github.com/0x5a17ed/uefi/efi/efivario/efiwindows"
)

type bufferVarEntry struct {
	Length uint32

	Guid efiguid.GUID

	Name []byte
}

func (e *bufferVarEntry) ReadFrom(r io.Reader) (n int64, err error) {
	fr := efireader.NewFieldReader(r, &n)

	if err = fr.ReadFields(&e.Length, &e.Guid); err != nil {
		return
	}

	e.Name = make([]byte, e.Length-20)
	if _, err = io.ReadFull(fr, e.Name); err != nil {
		return
	}

	return
}

type wapiVarNameIterator struct {
	buf     *bytes.Buffer
	current *VariableNameItem
	err     error
}

func (it *wapiVarNameIterator) Close() error {
	return nil
}

func (it *wapiVarNameIterator) Iter() itkit.Iterator[VariableNameItem] {
	return it
}

func (it *wapiVarNameIterator) Next() bool {
	var entry bufferVarEntry
	if _, err := entry.ReadFrom(it.buf); err != nil {
		if !errors.Is(err, io.EOF) && errors.Is(err, io.ErrUnexpectedEOF) {
			it.err = err
		}
		it.current = nil
		return false
	}

	it.current = &VariableNameItem{
		Name: efireader.UTF16ZBytesToString(entry.Name),
		GUID: entry.Guid,
	}
	return true
}

func (it *wapiVarNameIterator) Value() VariableNameItem {
	return *it.current
}

func (it *wapiVarNameIterator) Err() error {
	return it.err
}

func convertNameGuid(name string, guid efiguid.GUID) (lpName, lpGuid *uint16, err error) {
	lpName, err = syscall.UTF16PtrFromString(name)
	if err != nil {
		err = fmt.Errorf("utf16(%q): %w", name, err)
		return
	}

	lpGuid, err = syscall.UTF16PtrFromString(guid.Braced())
	if err != nil {
		err = fmt.Errorf("utf16(%q): %w", guid, err)
		return
	}

	return
}

// WindowsContext provides an implementation of the Context API
// for the windows platform.
type WindowsContext struct{}

// Ensure the public facing API in Context is implemented by WindowsContext.
var _ Context = &WindowsContext{}

func (c WindowsContext) Close() error {
	return nil
}

func (c WindowsContext) VariableNames() (VariableNameIterator, error) {
	var bufLen uint32
	if err := efiwindows.NtEnumerateSystemEnvironmentValuesEx(1, nil, &bufLen); err != nil {
		if !errors.Is(err, windows.STATUS_BUFFER_TOO_SMALL) {
			return nil, err
		}
	}

	buf := make([]byte, bufLen)
	if err := efiwindows.NtEnumerateSystemEnvironmentValuesEx(1, &buf[0], &bufLen); err != nil {
		return nil, err
	}

	return &wapiVarNameIterator{buf: bytes.NewBuffer(buf)}, nil
}

func (c WindowsContext) GetSizeHint(name string, guid efiguid.GUID) (int64, error) {
	lpName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, fmt.Errorf("efivario/GetSizeHint: utf16(%q): %w", name, err)
	}

	var uName windows.NTUnicodeString
	windows.RtlInitUnicodeString(&uName, lpName)

	var bufLen uint32
	err = efiwindows.NtQuerySystemEnvironmentValueEx(&uName, &guid, nil, &bufLen, nil)
	if err != nil && !errors.Is(err, windows.STATUS_BUFFER_TOO_SMALL) {
		return 0, fmt.Errorf("efivario/GetSizeHint: query(%q): %w", name, err)
	}
	return int64(bufLen), nil
}

func (c WindowsContext) Get(name string, guid efiguid.GUID, out []byte) (a Attributes, n int, err error) {
	lpName, lpGuid, err := convertNameGuid(name, guid)
	if err != nil {
		err = fmt.Errorf("efivario/Get: %w", err)
		return
	}

	length, err := efiwindows.GetFirmwareEnvironmentVariableEx(lpName, lpGuid, out, (*uint32)(&a))
	if err != nil {
		switch err {
		case windows.ERROR_INSUFFICIENT_BUFFER:
			err = ErrInsufficientSpace
		case windows.ERROR_ENVVAR_NOT_FOUND:
			err = ErrNotFound
		default:
			err = fmt.Errorf("efivario/Get: %w", err)
		}
		return
	}
	return a, int(length), err
}

func (c WindowsContext) Set(name string, guid efiguid.GUID, attributes Attributes, value []byte) error {
	lpName, lpGuid, err := convertNameGuid(name, guid)
	if err != nil {
		return fmt.Errorf("efivario/Set: %w", err)
	}

	err = efiwindows.SetFirmwareEnvironmentVariableEx(lpName, lpGuid, value, (uint32)(attributes))
	if err != nil {
		return fmt.Errorf("efivario/Set: %w", err)
	}
	return nil
}

func (c WindowsContext) Delete(name string, guid efiguid.GUID) error {
	lpName, lpGuid, err := convertNameGuid(name, guid)
	if err != nil {
		return fmt.Errorf("efivario/Delete: %w", err)
	}

	err = efiwindows.SetFirmwareEnvironmentVariableEx(lpName, lpGuid, nil, 0)
	if err != nil {
		return fmt.Errorf("efivario/Delete: %w", err)
	}
	return nil
}

func NewDefaultContext() Context {
	return &WindowsContext{}
}
