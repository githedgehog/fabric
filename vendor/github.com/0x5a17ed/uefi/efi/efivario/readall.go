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

	"github.com/0x5a17ed/uefi/efi/efiguid"
)

func ReadAll(c Context, name string, guid efiguid.GUID) (
	attrs Attributes,
	out []byte,
	err error,
) {
	hint, err := c.GetSizeHint(name, guid)
	if err != nil || hint < 0 {
		hint = 8
	}

	out = make([]byte, hint)
	for i := int(hint) << 1; i <= 4096; i = i << 1 {
		var n int
		attrs, n, err = c.Get(name, guid, out)
		if err != nil {
			if errors.Is(err, ErrInsufficientSpace) {
				out = append(make([]byte, i-len(out)), out...)
				continue
			}
			return
		}
		out = out[:n]
		return
	}
	return
}
