// Copyright (c) 2022 Arthur Skowronek <0x5a17ed@tuta.io>
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
	"github.com/0x5a17ed/itkit/iters/sliceit"
)

// ChainIterator chains multiple Iterator iterators together,
// traversing the given iterators until they are exhausted and
// proceeding with the next iterator.
type ChainIterator[T any] struct {
	iters itkit.Iterator[itkit.Iterator[T]]

	current itkit.Iterator[T]
}

// Ensure ChainIterator conforms to the Iterator protocol.
var _ itkit.Iterator[struct{}] = &ChainIterator[struct{}]{}

func (c *ChainIterator[T]) Next() bool {
	if c.current != nil && c.current.Next() {
		return true
	}
	for c.iters.Next() {
		c.current = c.iters.Value()
		if c.current.Next() {
			return true
		}
	}
	return false
}

func (c *ChainIterator[T]) Value() T {
	return c.current.Value()
}

// ChainI returns a ChainIterator chaining multiple Iterator iterators
// together.
func ChainI[T any](iters itkit.Iterator[itkit.Iterator[T]]) itkit.Iterator[T] {
	return &ChainIterator[T]{iters: iters}
}

// ChainV is the variadic version of ChainI.
func ChainV[T any](iters ...itkit.Iterator[T]) itkit.Iterator[T] {
	return ChainI(sliceit.In(iters))
}
