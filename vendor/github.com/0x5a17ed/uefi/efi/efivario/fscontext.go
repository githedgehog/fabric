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

package efivario

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"syscall"

	"github.com/0x5a17ed/itkit"
	"github.com/spf13/afero"
	"go.uber.org/multierr"

	"github.com/0x5a17ed/uefi/efi/efiguid"
)

var nameRegex = regexp.MustCompile(`^([^-]+)-([\da-fA-F]{8}-[\da-fA-F]{4}-[\da-fA-F]{4}-[\da-fA-F]{4}-[\da-fA-F]{12})$`)

// fsVarNameIterator is a variable name iterator for FsContext
type fsVarNameIterator struct {
	f       afero.File
	err     error
	current *VariableNameItem
}

func (it *fsVarNameIterator) Close() error {
	return it.f.Close()
}

func (it *fsVarNameIterator) Iter() itkit.Iterator[VariableNameItem] {
	return it
}

func (it *fsVarNameIterator) Next() bool {
	for {
		names, err := it.f.Readdirnames(1)
		if err != nil || len(names) < 1 {
			if !errors.Is(err, io.EOF) {
				it.err = err
			}
			it.current = nil
			return false
		}

		matches := nameRegex.FindStringSubmatch(names[0])
		if matches == nil {
			continue
		}

		g, err := efiguid.FromString(matches[2])
		if err != nil {
			it.err = err
			it.current = nil
			return false
		}

		it.current = &VariableNameItem{Name: matches[1], GUID: g}
		return true
	}
}

func (it *fsVarNameIterator) Value() VariableNameItem {
	return *it.current
}

func (it *fsVarNameIterator) Err() error {
	return it.err
}

func getFileName(name string, guid efiguid.GUID) string {
	return fmt.Sprintf("%s-%s", name, guid)
}

// FsContext provides an implementation of the Context API
// for platforms using a directory/file-based representation of
// the EFI variable service.
type FsContext struct {
	fs afero.Fs
}

// Ensure the public facing API in Context is implemented by FsContext.
var _ Context = &FsContext{}

func (c FsContext) Close() error {
	return nil
}

func (c FsContext) VariableNames() (VariableNameIterator, error) {
	f, err := c.fs.Open("")
	if err != nil {
		return nil, err
	}
	return &fsVarNameIterator{f: f}, nil
}

func (c FsContext) readEfiVarFileName(name string, out []byte) (a Attributes, n int, err error) {
	f, err := c.fs.Open(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOENT) {
			// Overwrite error with a custom error type.
			err = ErrNotFound
		}
		return
	}
	defer multierr.AppendInvoke(&err, multierr.Close(f))

	if err = binary.Read(f, binary.LittleEndian, &a); err != nil {
		return
	}

	if n, err = f.Read(out); err != nil {
		return
	}

	// Ensure to return ErrInsufficientSpace if there is more data
	// available to read than out can hold.
	var tmp [1]byte
	switch _, err = f.Read(tmp[:]); err {
	case io.EOF:
		err = nil
	case nil:
		err = ErrInsufficientSpace
	}
	return
}

func (c FsContext) writeEfiVarFileName(name string, value []byte, attrs Attributes) (err error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, attrs); err != nil {
		return fmt.Errorf("write attr: %w", err)
	}

	if _, err := buf.Write(value); err != nil {
		return fmt.Errorf("write value: %w", err)
	}

	guard, err := openSafeguard(c.fs, name)
	if err != nil {
		return fmt.Errorf("guard open: %w", err)
	}
	if guard != nil {
		defer multierr.AppendInvoke(&err, multierr.Invoke(func() error {
			if err := guard.Close(); err != nil {
				return fmt.Errorf("guard close: %w", err)
			}
			return nil
		}))
	}

	wasProtected, err := guard.disable()
	if err != nil {
		return fmt.Errorf("disable protection: %w", err)
	}
	if wasProtected {
		defer multierr.AppendInvoke(&err, multierr.Invoke(func() error {
			if err := guard.enable(); err != nil {
				return fmt.Errorf("enable protection: %w", err)
			}
			return nil
		}))
	}

	flags := os.O_WRONLY | os.O_CREATE
	if attrs&AppendWrite != 0 {
		flags |= os.O_APPEND
	}

	f, err := c.fs.OpenFile(name, flags, 0644)
	if err != nil {
		return err
	}
	defer multierr.AppendInvoke(&err, multierr.Close(f))

	if _, err := buf.WriteTo(f); err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		switch {
		case errors.Is(err, fs.ErrInvalid):
			fallthrough
		case errors.Is(err, syscall.EINVAL):
			// fsync is not implemented by efivarfs yet so
			// calling it here might sound silly, which it
			// actually is.  Lets just ignore it for now.
			return nil
		default:
			return err
		}
	}
	return nil
}

func (c FsContext) deleteEfiFile(name string) error {
	guard, err := openSafeguard(c.fs, name)
	if err != nil {
		return fmt.Errorf("guard open: %w", err)
	}
	if guard != nil {
		defer multierr.AppendInvoke(&err, multierr.Invoke(func() error {
			if err := guard.Close(); err != nil {
				return fmt.Errorf("guard close: %w", err)
			}
			return nil
		}))
	}

	if _, err := guard.disable(); err != nil {
		return fmt.Errorf("guard disable: %w", err)
	}

	if err := c.fs.Remove(name); err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOENT) {
			// Overwrite error with a custom error type.
			err = ErrNotFound
		}
		return fmt.Errorf("remove: %w", err)
	}
	return nil
}

func (c FsContext) GetSizeHint(name string, guid efiguid.GUID) (int64, error) {
	fi, err := c.fs.Stat(getFileName(name, guid))
	if err != nil {
		return 0, err
	}
	return fi.Size() - 4, nil
}

func (c FsContext) Get(name string, guid efiguid.GUID, out []byte) (a Attributes, n int, err error) {
	a, n, err = c.readEfiVarFileName(getFileName(name, guid), out)
	if err != nil {
		err = fmt.Errorf("efivario/get: %w", err)
	}
	return
}

func (c FsContext) Set(name string, guid efiguid.GUID, attributes Attributes, value []byte) (err error) {
	if err = c.writeEfiVarFileName(getFileName(name, guid), value, attributes); err != nil {
		err = fmt.Errorf("efivario/set: %w", err)
	}
	return
}

func (c FsContext) Delete(name string, guid efiguid.GUID) (err error) {
	if err = c.deleteEfiFile(getFileName(name, guid)); err != nil {
		err = fmt.Errorf("efivario/delete: %w", err)
	}
	return
}

func NewFileSystemContext(fs afero.Fs) *FsContext {
	return &FsContext{fs: fs}
}
