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
	"bytes"
	"fmt"
	"io"

	"github.com/0x5a17ed/uefi/efi/efiguid"
	"github.com/0x5a17ed/uefi/efi/efivario"
)

const (
	globalAccess = efivario.BootServiceAccess | efivario.RuntimeAccess

	defaultAttrs = efivario.NonVolatile | globalAccess
)

type MarshalFn[T any] func(w io.Writer, inp T) error
type UnmarshalFn[T any] func(r io.Reader) (T, error)

type Variable[T any] struct {
	name         string
	guid         efiguid.GUID
	defaultAttrs efivario.Attributes

	marshal   MarshalFn[T]
	unmarshal UnmarshalFn[T]
}

func (e Variable[T]) Get(c efivario.Context) (attrs efivario.Attributes, value T, err error) {
	if e.unmarshal == nil {
		err = fmt.Errorf("efivars/get(%s): unsupported", e.name)
		return
	}

	attrs, data, err := efivario.ReadAll(c, e.name, e.guid)
	if err != nil {
		err = fmt.Errorf("efivars/get(%s): load: %w", e.name, err)
		return
	}

	value, err = e.unmarshal(bytes.NewReader(data))
	if err != nil {
		err = fmt.Errorf("efivars/get(%s): parse: %w", e.name, err)
	}
	return
}

func (e Variable[T]) SetWithAttributes(c efivario.Context, attrs efivario.Attributes, value T) error {
	if e.marshal == nil {
		return fmt.Errorf("efivars/set(%s): unsupported", e.name)
	}

	var buf bytes.Buffer
	if err := e.marshal(&buf, value); err != nil {
		return fmt.Errorf("efivars/set(%s): write: %w", e.name, err)
	}
	return c.Set(e.name, e.guid, attrs, buf.Bytes())
}

func (e Variable[T]) Set(c efivario.Context, value T) error {
	return e.SetWithAttributes(c, e.defaultAttrs, value)
}
