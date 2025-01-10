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
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"unicode/utf16"
)

func ReadASCIINullBytes(r io.Reader) (out []byte, err error) {
	var block [1]byte
	for {
		if _, err = io.ReadFull(r, block[:]); err != nil {
			return
		}

		out = append(out, block[:]...)
		if bytes.Equal(block[:], []byte{0x00}) {
			return
		}
	}
}

// ASCIIZBytesToString decodes a null byte terminated ascii byte sequence to a string.
func ASCIIZBytesToString(b []byte) (s string) {
	if i := bytes.IndexByte(b, 0); i != -1 {
		b = b[:i]
	}
	return string(b)
}

func ReadUTF16NullBytes(r io.Reader) (out []byte, err error) {
	var block [2]byte
	for {
		if _, err = io.ReadFull(r, block[:]); err != nil {
			return
		}

		out = append(out, block[:]...)
		if bytes.Equal(block[:], []byte{0x00, 0x00}) {
			return
		}
	}
}

// UTF16BytesToString decodes an unterminated UTF-16 byte sequence to a string.
func UTF16BytesToString(b []byte) string {
	out := make([]uint16, len(b)>>1)
	for i, j := 0, 0; j < len(b); i, j = i+1, j+2 {
		out[i] = binary.LittleEndian.Uint16(b[j:])
	}
	return string(utf16.Decode(out))
}

// UTF16ZBytesToString decodes a null byte terminated UTF-16 byte sequence to a string.
func UTF16ZBytesToString(b []byte) (s string) {
	s = UTF16BytesToString(b)
	if i := strings.IndexByte(s, 0); i != -1 {
		s = s[:i]
	}
	return
}
