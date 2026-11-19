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

package efidevicepath

import (
	"fmt"
	"io"

	"github.com/0x5a17ed/uefi/efi/efihex"
)

// EndOfPath terminates a Device Path.
type EndOfPath struct{ Head }

func (p *EndOfPath) ReadFrom(r io.Reader) (n int64, err error) { return 0, nil }
func (p *EndOfPath) GetHead() *Head                            { return &p.Head }
func (p *EndOfPath) Text() string                              { return "" }

const (
	_ DevicePathSubType = iota

	// EndSingleSubType terminates one Device Path instance and
	// denotes the start of another.
	EndSingleSubType

	// EndEntireSubType terminates an entire Device Path.
	EndEntireSubType DevicePathSubType = 0xff
)

// UnrecognizedDevicePath represents a Device Path that is unimplemented.
type UnrecognizedDevicePath struct {
	Head

	Data []byte
}

func (p *UnrecognizedDevicePath) ReadFrom(r io.Reader) (n int64, err error) {
	p.Data, err = io.ReadAll(r)
	n = int64(len(p.Data))
	return
}

func (p *UnrecognizedDevicePath) GetHead() *Head {
	return &p.Head
}

func (p *UnrecognizedDevicePath) Text() string {
	return fmt.Sprintf(
		"Path(%d,%d,%s)",
		p.Head.Type,
		p.Head.SubType,
		efihex.EncodeToString(p.Data),
	)
}

func ParseUnrecognizedDevicePath(r io.Reader, h Head) (p DevicePath, err error) {
	p = &UnrecognizedDevicePath{Head: h}
	_, err = p.ReadFrom(r)
	return
}
