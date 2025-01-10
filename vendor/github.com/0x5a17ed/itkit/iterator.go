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

package itkit

// An Iterator allows consuming individual items in a stream of items.
type Iterator[T any] interface {
	// Next advances the iterator to the first/next item,
	// returning true if successful meaning there is an item
	// available to be fetched with Value and false otherwise.
	Next() bool

	// Value returns the current item if there is any and panics
	// otherwise.
	//
	// Note: Use Next to ensure there is an item.
	Value() T
}
