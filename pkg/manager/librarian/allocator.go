package librarian

import (
	"math"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	"golang.org/x/exp/constraints"
)

type Values[Value comparable] interface {
	Add(Value) error
	Remove(Value)
	Next() (Value, error)
}

type Allocator[Value comparable] struct {
	Values Values[Value]
}

func (a *Allocator[Value]) Allocate(known map[string]Value, updates map[string]bool) (map[string]Value, error) {
	updated := map[string]Value{}

	for key, val := range known {
		if updates[key] {
			updated[key] = val
			if err := a.Values.Add(val); err != nil {
				return nil, errors.Errorf("failed to reuse value for %s", key)
			}
		} else {
			a.Values.Remove(val)
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

func (v *NextFreeValueFromRanges[Value]) Add(val Value) error {
	valid := false
	for _, r := range v.ranges {
		if r[0] <= val && val <= r[1] {
			valid = true
			break
		}
	}
	if !valid {
		return errors.Errorf("value %d is not in any of the ranges", val)
	}

	v.taken[val] = true

	return nil
}

func (v *NextFreeValueFromRanges[Value]) Remove(val Value) {
	delete(v.taken, val)
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

func (v *BalancedValues[Value]) Add(val Value) error {
	if _, ok := v.usage[val]; !ok {
		return errors.Errorf("value %v is not in the list", val)
	}

	v.usage[val]++

	return nil
}

func (v *BalancedValues[Value]) Remove(val Value) {
	if _, ok := v.usage[val]; !ok {
		return
	}

	if v.usage[val] > 0 {
		v.usage[val]--
	}
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
