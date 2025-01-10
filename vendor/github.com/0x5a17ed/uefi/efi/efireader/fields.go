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

package efireader

import (
	"encoding/binary"
	"fmt"
	"io"
)

type FieldReader struct {
	reader io.Reader
	offset *int64
}

func (r *FieldReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	*r.offset += int64(n)
	return
}

func (r *FieldReader) Offset() int64 {
	return *r.offset
}

func (r *FieldReader) ReadFields(fields ...any) (err error) {
	for i, d := range fields {
		if err = binary.Read(r, binary.LittleEndian, d); err != nil {
			err = fmt.Errorf("field #%d: %w", i, err)
			return
		}
	}
	return
}

func NewFieldReader(reader io.Reader, offset *int64) *FieldReader {
	if offset == nil {
		offset = new(int64)
	}
	return &FieldReader{reader: reader, offset: offset}
}

func ReadFields(r io.Reader, fields ...any) (n int64, err error) {
	err = NewFieldReader(r, &n).ReadFields(fields...)
	return
}
