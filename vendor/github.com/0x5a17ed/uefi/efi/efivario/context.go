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
	"errors"
	"io"

	"github.com/0x5a17ed/itkit"

	"github.com/0x5a17ed/uefi/efi/efiguid"
)

var (
	ErrInsufficientSpace = errors.New("buffer too small")
	ErrNotFound          = errors.New("variable not found")
)

type VariableNameItem struct {
	Name string
	GUID efiguid.GUID
}

type VariableNameIterator interface {
	io.Closer
	itkit.Iterator[VariableNameItem]
	Iter() itkit.Iterator[VariableNameItem]
	Err() error
}

type Context interface {
	io.Closer

	// GetSizeHint hints the value size of the variable.
	GetSizeHint(name string, guid efiguid.GUID) (int64, error)

	// Get reads a specific EFI variable and returns the content
	// in the slice passed through out.
	Get(name string, guid efiguid.GUID, out []byte) (Attributes, int, error)

	// Set writes a specific EFI variable.
	Set(name string, guid efiguid.GUID, attrs Attributes, value []byte) error

	// Delete causes the entire variable to be removed from
	// existence completely.
	Delete(name string, guid efiguid.GUID) error

	// VariableNames returns an Iterator which enumerates all
	// EFI variables that are currently set on the current system.
	VariableNames() (VariableNameIterator, error)
}
