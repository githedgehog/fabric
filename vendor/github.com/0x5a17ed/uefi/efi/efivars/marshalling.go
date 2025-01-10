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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

func primitiveUnmarshaller[T any](r io.Reader) (out T, err error) {
	err = binary.Read(r, binary.LittleEndian, &out)
	return
}

func primitiveMarshaller[T any](w io.Writer, inp T) error {
	return binary.Write(w, binary.LittleEndian, inp)
}

func sliceUnmarshaller[T any](r io.Reader) (out []T, err error) {
	for i := 0; ; i += 1 {
		var item T
		err = binary.Read(r, binary.LittleEndian, &item)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			} else {
				err = fmt.Errorf("item #%d: %w", i, err)
			}
			return
		}
		out = append(out, item)
	}
}

func sliceMarshaller[T any](w io.Writer, inp []T) (err error) {
	var buf bytes.Buffer

	for i, item := range inp {
		err := binary.Write(&buf, binary.LittleEndian, item)
		if err != nil {
			return fmt.Errorf("item #%d: %w", i, err)
		}
	}

	_, err = buf.WriteTo(w)
	return
}

type readerFrom[T any] interface {
	io.ReaderFrom
	*T
}

func structUnmarshaller[T any, PT readerFrom[T]](r io.Reader) (out *T, err error) {
	var value T
	_, err = PT(&value).ReadFrom(r)
	if err == nil {
		out = &value
	}
	return
}

func structMarshaller[T io.WriterTo](w io.Writer, inp T) (err error) {
	_, err = inp.WriteTo(w)
	return
}
