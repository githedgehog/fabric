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

type MapFn[T, V any] func(T) V

type MapIterator[T, V any] struct {
	it  itkit.Iterator[T]
	fn  MapFn[T, V]
	cur V
}

func (m *MapIterator[T, V]) Next() (ok bool) {
	if ok = m.it.Next(); ok {
		m.cur = m.fn(m.it.Value())
	}
	return
}

func (m *MapIterator[T, V]) Value() V { return m.cur }

// Map returns an iterator that applies MapFn function to every item
// of iterkit.Iterator iterable, yielding the results.
func Map[T, V any](it itkit.Iterator[T], fn MapFn[T, V]) itkit.Iterator[V] {
	return &MapIterator[T, V]{it: it, fn: fn}
}
