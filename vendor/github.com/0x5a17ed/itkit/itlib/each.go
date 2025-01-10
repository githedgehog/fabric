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
	"golang.org/x/exp/constraints"

	"github.com/0x5a17ed/itkit"
)

type ApplyFn[T any] func(item T)

// Apply walks through the given Iterator it and calls ApplyFn fn
// for every single entry.
func Apply[T any](it itkit.Iterator[T], fn ApplyFn[T]) {
	for it.Next() {
		fn(it.Value())
	}
}

type ApplyNFn[T any] func(i int, item T)

// ApplyN walks through the given Iterator it and calls ApplyNFn fn
// for every single entry together with its index.
func ApplyN[T any](it itkit.Iterator[T], fn ApplyNFn[T]) {
	for i := 0; it.Next(); i += 1 {
		fn(i, it.Value())
	}
}

type ApplyToFn[T, R any] func(obj R, item T)

// ApplyTo walks through the given Iterator it and calls ApplyToFn fn
// for every single entry with the given value in obj of type R,
// returning the passed value.
//
// ApplyTo behaves like ReduceWithInitial with the exception that the
// result of the callback for the previous iteration not being passed
// down to the next iteration.
func ApplyTo[T, R any](it itkit.Iterator[T], obj R, fn ApplyToFn[T, R]) R {
	for it.Next() {
		fn(obj, it.Value())
	}
	return obj
}

type EachFn[T any] func(item T) bool

// Each walks through the given Iterator it and calls EachFn fn for
// every single entry, aborting if EachFn fn returns true.
func Each[T any](it itkit.Iterator[T], fn EachFn[T]) {
	for it.Next() {
		if fn(it.Value()) {
			break
		}
	}
}

type EachNFn[T any] func(i int, item T) bool

// EachN walks through the given Iterator it and calls EachNFn fn for
// every single entry together with its index, aborting if the given
// function returns true.
func EachN[T any](it itkit.Iterator[T], fn EachNFn[T]) {
	for i := 0; it.Next(); i += 1 {
		if fn(i, it.Value()) {
			break
		}
	}
}

type AccumulatorFn[T, R any] func(R, T) R

// ReduceWithInitial reduces the given Iterator to a value which is the
// accumulated result of running each value through AccumulatorFn,
// where each successive invocation of AccumulatorFn is supplied
// the return value of the previous invocation.
func ReduceWithInitial[T, R any](initial R, it itkit.Iterator[T], fn AccumulatorFn[T, R]) (out R) {
	out = initial
	Apply(it, func(el T) { out = fn(out, it.Value()) })
	return
}

// Reduce reduces the given Iterator to a value which is the
// accumulated result of running each value through AccumulatorFn,
// where each successive invocation of AccumulatorFn is supplied
// the return value of the previous invocation.
func Reduce[T, R any](it itkit.Iterator[T], fn AccumulatorFn[T, R]) (out R) {
	var zero R
	return ReduceWithInitial(zero, it, fn)
}

// SumWithInitial accumulates the Iterator values based on the summation of their values.
func SumWithInitial[T constraints.Ordered](initial T, it itkit.Iterator[T]) T {
	return ReduceWithInitial[T, T](initial, it, func(a, b T) T { return a + b })
}

// Sum accumulates the Iterator values based on the summation of their values.
func Sum[T constraints.Ordered](it itkit.Iterator[T]) T {
	var zero T
	return SumWithInitial(zero, it)
}
