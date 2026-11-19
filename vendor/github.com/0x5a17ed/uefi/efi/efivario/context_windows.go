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

type sysEnvVarsAPIImpl struct{}

func (impl sysEnvVarsAPIImpl) Get(lpName *uint16, lpGuid *uint16, buf []byte, attrs *uint32) (n uint32, err error) {
	return efiwindows.GetFirmwareEnvironmentVariableEx(lpName, lpGuid, buf, attrs)
}

func (impl sysEnvVarsAPIImpl) Set(lpName *uint16, lpGuid *uint16, buf []byte, attrs uint32) (err error) {
	return efiwindows.SetFirmwareEnvironmentVariableEx(lpName, lpGuid, buf, attrs)
}

func (impl sysEnvVarsAPIImpl) Enumerate(InformationClass uint32, buf *byte, bufLen *uint32) (ntstatus error) {
	return efiwindows.NtEnumerateSystemEnvironmentValuesEx(InformationClass, buf, bufLen)
}

func (impl sysEnvVarsAPIImpl) Query(
	name *windows.NTUnicodeString,
	guid *efiwindows.GUID,
	buf *byte,
	bufLen *uint32,
	attrs *uint32,
) (ntstatus error) {
	return efiwindows.NtQuerySystemEnvironmentValueEx(name, guid, buf, bufLen, attrs)
}

type bufferVarEntry struct {
	NextEntryOffset uint32

	Guid efiguid.GUID

	Name string
}

func (e *bufferVarEntry) ReadFrom(r io.Reader) (n int64, err error) {
	fr := efireader.NewFieldReader(r, &n)

	if err = fr.ReadFields(&e.NextEntryOffset, &e.Guid); err != nil {
		return n, err
	}

	// Read remainder of the entry.
	var entryBuffer bytes.Buffer
	if e.NextEntryOffset > 0 {
		// Read until next entry.
		entryLength := int64(e.NextEntryOffset) - fr.Offset()
		if _, err = io.CopyN(&entryBuffer, fr, entryLength); err != nil {
			return n, err
		}

	} else {
		// Read all what's left in the buffer.
		if _, err = io.Copy(&entryBuffer, fr); err != nil {
			return n, err
		}
	}

	// Set name field.
	nameBytes, err := efireader.ReadUTF16NullBytes(&entryBuffer)
	if err != nil {
		return n, err
	}
	e.Name = efireader.UTF16ZBytesToString(nameBytes)

	return n, nil
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
		Name: entry.Name,
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

type sysEnvVarsAPI interface {
	Get(lpName *uint16, lpGuid *uint16, buf []byte, attrs *uint32) (n uint32, err error)

	Set(lpName *uint16, lpGuid *uint16, buf []byte, attrs uint32) (err error)

	Enumerate(InformationClass uint32, buf *byte, bufLen *uint32) (ntstatus error)

	Query(
		name *windows.NTUnicodeString,
		guid *efiwindows.GUID,
		buf *byte,
		bufLen *uint32,
		attrs *uint32,
	) (ntstatus error)
}

// WindowsContext provides an implementation of the Context API
// for the windows platform.
type WindowsContext struct {
	api sysEnvVarsAPI
}

// Ensure the public facing API in Context is implemented by WindowsContext.
var _ Context = &WindowsContext{}

func (c WindowsContext) Close() error {
	return nil
}

func (c WindowsContext) VariableNames() (VariableNameIterator, error) {
	var bufLen uint32

	// Try first a null buffer to figure out how large the buffer needs to be.
	if err := c.api.Enumerate(1, nil, &bufLen); err != nil {
		if !errors.Is(err, windows.STATUS_BUFFER_TOO_SMALL) {
			return nil, err
		}
	}

	buf := make([]byte, bufLen)
	if err := c.api.Enumerate(1, &buf[0], &bufLen); err != nil {
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
	err = c.api.Query(&uName, &guid, nil, &bufLen, nil)
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

	length, err := c.api.Get(lpName, lpGuid, out, (*uint32)(&a))
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

	err = c.api.Set(lpName, lpGuid, value, (uint32)(attributes))
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

	err = c.api.Set(lpName, lpGuid, nil, 0)
	if err != nil {
		return fmt.Errorf("efivario/Delete: %w", err)
	}
	return nil
}

func NewDefaultContext() Context {
	return &WindowsContext{api: sysEnvVarsAPIImpl{}}
}
