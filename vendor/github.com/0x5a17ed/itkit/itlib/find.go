// Copyright (c) 2022 individual contributors.
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

package itlib

import (
	"github.com/0x5a17ed/itkit"
)

type EqualFn[T any] func(a, b T) bool

func Find[T any](it itkit.Iterator[T], fn EqualFn[T], needle T) (out T, ok bool) {
	for it.Next() {
		if fn(it.Value(), needle) {
			return it.Value(), true
		}
	}
	return
}

// Head returns the next value in the iterator and true, if the
// iterator has a next item, consuming it from the iterator as well.
// Returns the zero value and false otherwise.
func Head[T any](it itkit.Iterator[T]) (out T, ok bool) {
	if it.Next() {
		out, ok = it.Value(), true
	}
	return
}

// HeadOrElse returns the next value in the iterator, if the iterator
// has a next item, consuming it from the iterator. Returns the
// provided default value otherwise.
func HeadOrElse[T any](it itkit.Iterator[T], v T) T {
	if it.Next() {
		v = it.Value()
	}
	return v
}
