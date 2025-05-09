package lo

import (
	"fmt"
	"time"

	"github.com/samber/lo/internal/constraints"
	"github.com/samber/lo/internal/rand"
)

// IndexOf returns the index at which the first occurrence of a value is found in an array or return -1
// if the value cannot be found.
func IndexOf[T comparable](collection []T, element T) int {
	for i := range collection {
		if collection[i] == element {
			return i
		}
	}

	return -1
}

// LastIndexOf returns the index at which the last occurrence of a value is found in an array or return -1
// if the value cannot be found.
func LastIndexOf[T comparable](collection []T, element T) int {
	length := len(collection)

	for i := length - 1; i >= 0; i-- {
		if collection[i] == element {
			return i
		}
	}

	return -1
}

// Find search an element in a slice based on a predicate. It returns element and true if element was found.
func Find[T any](collection []T, predicate func(item T) bool) (T, bool) {
	for i := range collection {
		if predicate(collection[i]) {
			return collection[i], true
		}
	}

	var result T
	return result, false
}

// FindIndexOf searches an element in a slice based on a predicate and returns the index and true.
// It returns -1 and false if the element is not found.
func FindIndexOf[T any](collection []T, predicate func(item T) bool) (T, int, bool) {
	for i := range collection {
		if predicate(collection[i]) {
			return collection[i], i, true
		}
	}

	var result T
	return result, -1, false
}

// FindLastIndexOf searches last element in a slice based on a predicate and returns the index and true.
// It returns -1 and false if the element is not found.
func FindLastIndexOf[T any](collection []T, predicate func(item T) bool) (T, int, bool) {
	length := len(collection)

	for i := length - 1; i >= 0; i-- {
		if predicate(collection[i]) {
			return collection[i], i, true
		}
	}

	var result T
	return result, -1, false
}

// FindOrElse search an element in a slice based on a predicate. It returns the element if found or a given fallback value otherwise.
func FindOrElse[T any](collection []T, fallback T, predicate func(item T) bool) T {
	for i := range collection {
		if predicate(collection[i]) {
			return collection[i]
		}
	}

	return fallback
}

// FindKey returns the key of the first value matching.
func FindKey[K comparable, V comparable](object map[K]V, value V) (K, bool) {
	for k := range object {
		if object[k] == value {
			return k, true
		}
	}

	return Empty[K](), false
}

// FindKeyBy returns the key of the first element predicate returns truthy for.
func FindKeyBy[K comparable, V any](object map[K]V, predicate func(key K, value V) bool) (K, bool) {
	for k := range object {
		if predicate(k, object[k]) {
			return k, true
		}
	}

	return Empty[K](), false
}

// FindUniques returns a slice with all the unique elements of the collection.
// The order of result values is determined by the order they occur in the collection.
func FindUniques[T comparable, Slice ~[]T](collection Slice) Slice {
	isDupl := make(map[T]bool, len(collection))

	for i := range collection {
		duplicated, ok := isDupl[collection[i]]
		if !ok {
			isDupl[collection[i]] = false
		} else if !duplicated {
			isDupl[collection[i]] = true
		}
	}

	result := make(Slice, 0, len(collection)-len(isDupl))

	for i := range collection {
		if duplicated := isDupl[collection[i]]; !duplicated {
			result = append(result, collection[i])
		}
	}

	return result
}

// FindUniquesBy returns a slice with all the unique elements of the collection.
// The order of result values is determined by the order they occur in the array. It accepts `iteratee` which is
// invoked for each element in array to generate the criterion by which uniqueness is computed.
func FindUniquesBy[T any, U comparable, Slice ~[]T](collection Slice, iteratee func(item T) U) Slice {
	isDupl := make(map[U]bool, len(collection))

	for i := range collection {
		key := iteratee(collection[i])

		duplicated, ok := isDupl[key]
		if !ok {
			isDupl[key] = false
		} else if !duplicated {
			isDupl[key] = true
		}
	}

	result := make(Slice, 0, len(collection)-len(isDupl))

	for i := range collection {
		key := iteratee(collection[i])

		if duplicated := isDupl[key]; !duplicated {
			result = append(result, collection[i])
		}
	}

	return result
}

// FindDuplicates returns a slice with the first occurrence of each duplicated elements of the collection.
// The order of result values is determined by the order they occur in the collection.
func FindDuplicates[T comparable, Slice ~[]T](collection Slice) Slice {
	isDupl := make(map[T]bool, len(collection))

	for i := range collection {
		duplicated, ok := isDupl[collection[i]]
		if !ok {
			isDupl[collection[i]] = false
		} else if !duplicated {
			isDupl[collection[i]] = true
		}
	}

	result := make(Slice, 0, len(collection)-len(isDupl))

	for i := range collection {
		if duplicated := isDupl[collection[i]]; duplicated {
			result = append(result, collection[i])
			isDupl[collection[i]] = false
		}
	}

	return result
}

// FindDuplicatesBy returns a slice with the first occurrence of each duplicated elements of the collection.
// The order of result values is determined by the order they occur in the array. It accepts `iteratee` which is
// invoked for each element in array to generate the criterion by which uniqueness is computed.
func FindDuplicatesBy[T any, U comparable, Slice ~[]T](collection Slice, iteratee func(item T) U) Slice {
	isDupl := make(map[U]bool, len(collection))

	for i := range collection {
		key := iteratee(collection[i])

		duplicated, ok := isDupl[key]
		if !ok {
			isDupl[key] = false
		} else if !duplicated {
			isDupl[key] = true
		}
	}

	result := make(Slice, 0, len(collection)-len(isDupl))

	for i := range collection {
		key := iteratee(collection[i])

		if duplicated := isDupl[key]; duplicated {
			result = append(result, collection[i])
			isDupl[key] = false
		}
	}

	return result
}

// Min search the minimum value of a collection.
// Returns zero value when the collection is empty.
func Min[T constraints.Ordered](collection []T) T {
	var min T

	if len(collection) == 0 {
		return min
	}

	min = collection[0]

	for i := 1; i < len(collection); i++ {
		item := collection[i]

		if item < min {
			min = item
		}
	}

	return min
}

// MinIndex search the minimum value of a collection and the index of the minimum value.
// Returns (zero value, -1) when the collection is empty.
func MinIndex[T constraints.Ordered](collection []T) (T, int) {
	var (
		min   T
		index int
	)

	if len(collection) == 0 {
		return min, -1
	}

	min = collection[0]

	for i := 1; i < len(collection); i++ {
		item := collection[i]

		if item < min {
			min = item
			index = i
		}
	}

	return min, index
}

// MinBy search the minimum value of a collection using the given comparison function.
// If several values of the collection are equal to the smallest value, returns the first such value.
// Returns zero value when the collection is empty.
func MinBy[T any](collection []T, comparison func(a T, b T) bool) T {
	var min T

	if len(collection) == 0 {
		return min
	}

	min = collection[0]

	for i := 1; i < len(collection); i++ {
		item := collection[i]

		if comparison(item, min) {
			min = item
		}
	}

	return min
}

// MinIndexBy search the minimum value of a collection using the given comparison function and the index of the minimum value.
// If several values of the collection are equal to the smallest value, returns the first such value.
// Returns (zero value, -1) when the collection is empty.
func MinIndexBy[T any](collection []T, comparison func(a T, b T) bool) (T, int) {
	var (
		min   T
		index int
	)

	if len(collection) == 0 {
		return min, -1
	}

	min = collection[0]

	for i := 1; i < len(collection); i++ {
		item := collection[i]

		if comparison(item, min) {
			min = item
			index = i
		}
	}

	return min, index
}

// Earliest search the minimum time.Time of a collection.
// Returns zero value when the collection is empty.
func Earliest(times ...time.Time) time.Time {
	var min time.Time

	if len(times) == 0 {
		return min
	}

	min = times[0]

	for i := 1; i < len(times); i++ {
		item := times[i]

		if item.Before(min) {
			min = item
		}
	}

	return min
}

// EarliestBy search the minimum time.Time of a collection using the given iteratee function.
// Returns zero value when the collection is empty.
func EarliestBy[T any](collection []T, iteratee func(item T) time.Time) T {
	var earliest T

	if len(collection) == 0 {
		return earliest
	}

	earliest = collection[0]
	earliestTime := iteratee(collection[0])

	for i := 1; i < len(collection); i++ {
		itemTime := iteratee(collection[i])

		if itemTime.Before(earliestTime) {
			earliest = collection[i]
			earliestTime = itemTime
		}
	}

	return earliest
}

// Max searches the maximum value of a collection.
// Returns zero value when the collection is empty.
func Max[T constraints.Ordered](collection []T) T {
	var max T

	if len(collection) == 0 {
		return max
	}

	max = collection[0]

	for i := 1; i < len(collection); i++ {
		item := collection[i]

		if item > max {
			max = item
		}
	}

	return max
}

// MaxIndex searches the maximum value of a collection and the index of the maximum value.
// Returns (zero value, -1) when the collection is empty.
func MaxIndex[T constraints.Ordered](collection []T) (T, int) {
	var (
		max   T
		index int
	)

	if len(collection) == 0 {
		return max, -1
	}

	max = collection[0]

	for i := 1; i < len(collection); i++ {
		item := collection[i]

		if item > max {
			max = item
			index = i
		}
	}

	return max, index
}

// MaxBy search the maximum value of a collection using the given comparison function.
// If several values of the collection are equal to the greatest value, returns the first such value.
// Returns zero value when the collection is empty.
func MaxBy[T any](collection []T, comparison func(a T, b T) bool) T {
	var max T

	if len(collection) == 0 {
		return max
	}

	max = collection[0]

	for i := 1; i < len(collection); i++ {
		item := collection[i]

		if comparison(item, max) {
			max = item
		}
	}

	return max
}

// MaxIndexBy search the maximum value of a collection using the given comparison function and the index of the maximum value.
// If several values of the collection are equal to the greatest value, returns the first such value.
// Returns (zero value, -1) when the collection is empty.
func MaxIndexBy[T any](collection []T, comparison func(a T, b T) bool) (T, int) {
	var (
		max   T
		index int
	)

	if len(collection) == 0 {
		return max, -1
	}

	max = collection[0]

	for i := 1; i < len(collection); i++ {
		item := collection[i]

		if comparison(item, max) {
			max = item
			index = i
		}
	}

	return max, index
}

// Latest search the maximum time.Time of a collection.
// Returns zero value when the collection is empty.
func Latest(times ...time.Time) time.Time {
	var max time.Time

	if len(times) == 0 {
		return max
	}

	max = times[0]

	for i := 1; i < len(times); i++ {
		item := times[i]

		if item.After(max) {
			max = item
		}
	}

	return max
}

// LatestBy search the maximum time.Time of a collection using the given iteratee function.
// Returns zero value when the collection is empty.
func LatestBy[T any](collection []T, iteratee func(item T) time.Time) T {
	var latest T

	if len(collection) == 0 {
		return latest
	}

	latest = collection[0]
	latestTime := iteratee(collection[0])

	for i := 1; i < len(collection); i++ {
		itemTime := iteratee(collection[i])

		if itemTime.After(latestTime) {
			latest = collection[i]
			latestTime = itemTime
		}
	}

	return latest
}

// First returns the first element of a collection and check for availability of the first element.
func First[T any](collection []T) (T, bool) {
	length := len(collection)

	if length == 0 {
		var t T
		return t, false
	}

	return collection[0], true
}

// FirstOrEmpty returns the first element of a collection or zero value if empty.
func FirstOrEmpty[T any](collection []T) T {
	i, _ := First(collection)
	return i
}

// FirstOr returns the first element of a collection or the fallback value if empty.
func FirstOr[T any](collection []T, fallback T) T {
	i, ok := First(collection)
	if !ok {
		return fallback
	}

	return i
}

// Last returns the last element of a collection or error if empty.
func Last[T any](collection []T) (T, bool) {
	length := len(collection)

	if length == 0 {
		var t T
		return t, false
	}

	return collection[length-1], true
}

// LastOrEmpty returns the last element of a collection or zero value if empty.
func LastOrEmpty[T any](collection []T) T {
	i, _ := Last(collection)
	return i
}

// LastOr returns the last element of a collection or the fallback value if empty.
func LastOr[T any](collection []T, fallback T) T {
	i, ok := Last(collection)
	if !ok {
		return fallback
	}

	return i
}

// Nth returns the element at index `nth` of collection. If `nth` is negative, the nth element
// from the end is returned. An error is returned when nth is out of slice bounds.
func Nth[T any, N constraints.Integer](collection []T, nth N) (T, error) {
	n := int(nth)
	l := len(collection)
	if n >= l || -n > l {
		var t T
		return t, fmt.Errorf("nth: %d out of slice bounds", n)
	}

	if n >= 0 {
		return collection[n], nil
	}
	return collection[l+n], nil
}

// NthOr returns the element at index `nth` of collection.
// If `nth` is negative, it returns the nth element from the end.
// If `nth` is out of slice bounds, it returns the fallback value instead of an error.
func NthOr[T any, N constraints.Integer](collection []T, nth N, fallback T) T {
	value, err := Nth(collection, nth)
	if err != nil {
		return fallback
	}
	return value
}

// NthOrEmpty returns the element at index `nth` of collection.
// If `nth` is negative, it returns the nth element from the end.
// If `nth` is out of slice bounds, it returns the zero value (empty value) for that type.
func NthOrEmpty[T any, N constraints.Integer](collection []T, nth N) T {
	value, err := Nth(collection, nth)
	if err != nil {
		var zeroValue T
		return zeroValue
	}
	return value
}

// randomIntGenerator is a function that should return a random integer in the range [0, n)
// where n is the parameter passed to the randomIntGenerator.
type randomIntGenerator func(n int) int

// Sample returns a random item from collection.
func Sample[T any](collection []T) T {
	result := SampleBy(collection, rand.IntN)
	return result
}

// SampleBy returns a random item from collection, using randomIntGenerator as the random index generator.
func SampleBy[T any](collection []T, randomIntGenerator randomIntGenerator) T {
	size := len(collection)
	if size == 0 {
		return Empty[T]()
	}
	return collection[randomIntGenerator(size)]
}

// Samples returns N random unique items from collection.
func Samples[T any, Slice ~[]T](collection Slice, count int) Slice {
	results := SamplesBy(collection, count, rand.IntN)
	return results
}

// SamplesBy returns N random unique items from collection, using randomIntGenerator as the random index generator.
func SamplesBy[T any, Slice ~[]T](collection Slice, count int, randomIntGenerator randomIntGenerator) Slice {
	size := len(collection)

	copy := append(Slice{}, collection...)

	results := Slice{}

	for i := 0; i < size && i < count; i++ {
		copyLength := size - i

		index := randomIntGenerator(size - i)
		results = append(results, copy[index])

		// Removes element.
		// It is faster to swap with last element and remove it.
		copy[index] = copy[copyLength-1]
		copy = copy[:copyLength-1]
	}

	return results
}
