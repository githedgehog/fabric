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

package ittuple

import (
	"fmt"
)

// T2 represents a generic tuple holding 2 values.
type T2[TL, TR any] struct {
	Left  TL
	Right TR
}

// Len returns the number of values held by the tuple.
func (t T2[TL, TR]) Len() int {
	return 2
}

// Values returns the values held by the tuple.
func (t T2[TL, TR]) Values() (TL, TR) {
	return t.Left, t.Right
}

// Array returns an array of the tuple values.
func (t T2[TL, TR]) Array() [2]any {
	return [2]any{t.Left, t.Right}
}

// Slice returns a slice of the tuple values.
func (t T2[TL, TR]) Slice() []any {
	a := t.Array()
	return a[:]
}

// String returns the string representation of the tuple.
func (t T2[TL, TR]) String() string {
	return fmt.Sprintf("[%#v %#v]", t.Slice()...)
}

func NewT2[TL, TR any](left TL, right TR) T2[TL, TR] {
	return T2[TL, TR]{Left: left, Right: right}
}
