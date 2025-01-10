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

//go:build linux

package efivario

import (
	"errors"
	"os"
	"syscall"

	"github.com/spf13/afero"
	"go.uber.org/multierr"
	"golang.org/x/sys/unix"
)

const (
	FS_IMMUTABLE_FL = 0x00000010
)

type flags uint32

func (a flags) IsSet(attrs flags) bool  { return a&attrs != 0 }
func (a flags) Clear(attrs flags) flags { return a & ^attrs }
func (a flags) Set(attrs flags) flags   { return a | attrs }

func getFlags(fd uintptr) (flags, error) {
	attrs, err := unix.IoctlGetInt(int(fd), unix.FS_IOC_GETFLAGS)
	return flags(attrs), err
}

func setFlags(fd uintptr, attr flags) error {
	return unix.IoctlSetPointerInt(int(fd), unix.FS_IOC_SETFLAGS, int(attr))
}

func resolveOsFile(f afero.File) (o *os.File, ok bool) {
	// Unwrap afero.BasePathFile instances.
	for {
		if baseFile, ok := f.(*afero.BasePathFile); ok {
			f = baseFile.File
			continue
		}
		break
	}

	o, ok = f.(*os.File)
	return
}

func withInnerFileDescriptor(f *os.File, cb func(fd uintptr) error) (err error) {
	rawConn, err := f.SyscallConn()
	if err != nil {
		return err
	}

	err2 := rawConn.Control(func(fd uintptr) {
		err2 := cb(fd)
		// Report anything else other than unsupported error.
		if !errors.Is(err2, syscall.ENOTTY) {
			err = multierr.Append(err, err2)
		}
	})
	err = multierr.Append(err, err2)
	return
}

type safeguard struct {
	*os.File
	fl flags
}

func (g *safeguard) disable() (wasProtected bool, err error) {
	if g != nil {
		err = withInnerFileDescriptor(g.File, func(fd uintptr) (err error) {
			wasProtected = g.fl.IsSet(FS_IMMUTABLE_FL)
			if !wasProtected {
				return nil
			}
			g.fl = g.fl.Clear(FS_IMMUTABLE_FL)
			return setFlags(fd, g.fl)
		})
	}
	return
}

func (g *safeguard) enable() error {
	return withInnerFileDescriptor(g.File, func(fd uintptr) error {
		g.fl = g.fl.Set(FS_IMMUTABLE_FL)
		return setFlags(fd, g.fl)
	})
}

func openSafeguard(fs afero.Fs, fpath string) (p *safeguard, err error) {
	f, err := fs.OpenFile(fpath, os.O_RDONLY, 0644)
	if err != nil {
		switch {
		case errors.Is(err, afero.ErrFileNotFound):
			fallthrough
		case errors.Is(err, syscall.ENOENT):
			return nil, nil
		default:
			return nil, err
		}
	}

	osFile, ok := resolveOsFile(f)
	if !ok {
		// The protection operation is not implemented by the
		// underlying filesystem and thus can't be performed.
		return nil, f.Close()
	}

	p = &safeguard{File: osFile}
	err = withInnerFileDescriptor(osFile, func(fd uintptr) (err error) {
		p.fl, err = getFlags(fd)
		return
	})
	return
}
