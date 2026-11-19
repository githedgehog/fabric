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

package efiguid

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/0x5a17ed/uefi/efi/efihex"
)

var (
	ErrBadFormat = errors.New("incorrect format")
	ErrBadLength = errors.New("incorrect length")
)

// This is terrible. UUIDs are naturally stored big endian. The String
// form of a UUID 00112233-4455-6677-8899AABBCCDDEEFF is therefore
// stored as 00 11 22 33 44 55 66 77 88 99 AA BB CC DD EE FF in memory.
// So the least significant bit is in the last byte.
//
// Now GUIDs are special. They store their individual groups of a UUID
// in little endian order. Except for the last two groups. These are
// interpreted as Big Endian in their string representation. Why?
// I would like to know as well! So in order to work with them in any
// meaningful way they need to be converted.

func encodeHex(dst []byte, uuid GUID) {
	_ = dst[len(uuid)-1] // bounds check hint to compiler
	efihex.EncodeLittleEndian(dst, uuid[:4])
	dst[8] = '-'
	efihex.EncodeLittleEndian(dst[9:13], uuid[4:6])
	dst[13] = '-'
	efihex.EncodeLittleEndian(dst[14:18], uuid[6:8])
	dst[18] = '-'
	efihex.EncodeBigEndian(dst[19:23], uuid[8:10])
	dst[23] = '-'
	efihex.EncodeBigEndian(dst[24:], uuid[10:])
}

type GUID [16]byte

func (u GUID) String() string {
	var buf [36]byte
	encodeHex(buf[:], u)
	return strings.ToUpper(string(buf[:]))
}

func (u GUID) Braced() string {
	var buf [38]byte
	buf[0] = '{'
	encodeHex(buf[1:], u)
	buf[37] = '}'
	return strings.ToUpper(string(buf[:]))
}

func swapEndianness(buf []byte) []byte {
	for i := 0; i < len(buf)/2; i++ {
		buf[i], buf[len(buf)-i-1] = buf[len(buf)-i-1], buf[i]
	}
	return buf
}

// decodeCanonical decodes GUID string in canonical format i.e
// "973a15e0-fa54-4fef-8d93-aebf7ace5d6b".
func (u *GUID) decodeCanonical(b []byte) (err error) {
	if b[8] != '-' || b[13] != '-' || b[18] != '-' || b[23] != '-' {
		return fmt.Errorf("uuid %q: %w", b, ErrBadFormat)
	}

	src := b[:]
	dst := u[:]

	for i, byteGroup := range []int{8, 4, 4, 4, 12} {
		if i > 0 {
			src = src[1:] // skip dash
		}
		_, err = hex.Decode(dst[:byteGroup/2], src[:byteGroup])
		if err != nil {
			return
		}

		src = src[byteGroup:]
		if i < 3 {
			swapEndianness(dst[0 : byteGroup/2])
		}
		dst = dst[byteGroup/2:]
	}

	return
}

func (u *GUID) UnmarshalText(s []byte) (err error) {
	switch len(s) {
	case 36:
		// xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		return u.decodeCanonical(s)
	case 38:
		// {xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx}
		if s[0] != '{' || s[len(s)-1] != '}' {
			return fmt.Errorf("uuid %q: %w", s, ErrBadFormat)
		}
		return u.decodeCanonical(s[1:])
	default:
		return fmt.Errorf("uuid %q: %w", s, ErrBadLength)
	}
}

func New(a uint32, b, c uint16, d [8]byte) (u GUID) {
	binary.LittleEndian.PutUint32(u[0:4], a)
	binary.LittleEndian.PutUint16(u[4:6], b)
	binary.LittleEndian.PutUint16(u[6:8], c)
	copy(u[8:16], d[:])
	return
}

func FromString(t string) (u GUID, err error) {
	err = u.UnmarshalText([]byte(t))
	return
}

// Must wrap a call to a function returning (GUID, error) and panics
// if the error is non-nil. Its intended use is for variable
// initializations.
func Must(u GUID, err error) GUID {
	if err != nil {
		panic(err)
	}
	return u
}

func MustFromString(s string) GUID {
	return Must(FromString(s))
}
