// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package librarian

import (
	"math"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	"golang.org/x/exp/constraints"
)

type Values[Value comparable] interface {
	Add(Value) bool
	Next() (Value, error)
}

type Allocator[Value comparable] struct {
	Values Values[Value]
}

func (a *Allocator[Value]) Allocate(known map[string]Value, updates map[string]bool) (map[string]Value, error) {
	updated := map[string]Value{}

	for key, val := range known {
		if updates[key] && a.Values.Add(val) {
			updated[key] = val
		}
	}

	for key, present := range updates {
		if !present {
			continue
		}
		if _, ok := updated[key]; ok {
			continue
		}

		if val, err := a.Values.Next(); err != nil {
			return nil, errors.Errorf("failed to allocate value for %s", key)
		} else {
			updated[key] = val
		}
	}

	return updated, nil
}

type NextFreeValueFromRanges[Value constraints.Unsigned] struct {
	ranges [][2]Value
	inc    Value

	taken        map[Value]bool
	fromRangeIdx int
	fromValue    Value
}

var _ Values[uint32] = &NextFreeValueFromRanges[uint32]{}

func NewNextFreeValueFromRanges[Value constraints.Unsigned](ranges [][2]Value, inc Value) *NextFreeValueFromRanges[Value] {
	return &NextFreeValueFromRanges[Value]{
		ranges: ranges,
		inc:    inc,
		taken:  map[Value]bool{},
	}
}

func NewNextFreeValueFromVLANRanges(ranges []meta.VLANRange) *NextFreeValueFromRanges[uint16] {
	ret := make([][2]uint16, len(ranges))

	for idx, r := range ranges {
		ret[idx] = [2]uint16{r.From, r.To}
	}

	return &NextFreeValueFromRanges[uint16]{
		ranges: ret,
		inc:    1,
		taken:  map[uint16]bool{},
	}
}

func (v *NextFreeValueFromRanges[Value]) Add(val Value) bool {
	if v.taken[val] {
		return false
	}

	valid := false
	for _, r := range v.ranges {
		if r[0] <= val && val <= r[1] && (val-r[0])%v.inc == 0 {
			valid = true
			break
		}
	}

	if valid {
		v.taken[val] = true
	}

	return valid
}

func (v *NextFreeValueFromRanges[Value]) Next() (Value, error) {
	for rangeIdx := v.fromRangeIdx; rangeIdx < len(v.ranges); rangeIdx++ {
		if v.fromValue < v.ranges[rangeIdx][0] {
			v.fromValue = v.ranges[rangeIdx][0]
		}

		for value := v.fromValue; value <= v.ranges[rangeIdx][1]; value += v.inc {
			if !v.taken[value] {
				v.fromRangeIdx = rangeIdx
				v.fromValue = value + v.inc
				return value, nil
			}
		}

		v.fromValue = 0
	}

	return 0, errors.New("no free value found")
}

type BalancedValues[Value comparable] struct {
	usage map[Value]uint32
}

var _ Values[string] = &BalancedValues[string]{}

func NewBalancedValues[Value comparable](vals []Value) *BalancedValues[Value] {
	usage := map[Value]uint32{}
	for _, val := range vals {
		usage[val] = 0
	}

	return &BalancedValues[Value]{
		usage: usage,
	}
}

func (v *BalancedValues[Value]) Add(val Value) bool {
	if _, ok := v.usage[val]; !ok {
		return false
	}

	v.usage[val]++

	return true
}

func (v *BalancedValues[Value]) Next() (Value, error) {
	var minVal Value
	minUsage := uint32(math.MaxUint32)

	for val, usage := range v.usage {
		if usage < minUsage {
			minVal = val
			minUsage = usage
		}
	}

	if minUsage == math.MaxUint32 {
		return minVal, errors.New("no free value found")
	}

	v.usage[minVal]++

	return minVal, nil
}
