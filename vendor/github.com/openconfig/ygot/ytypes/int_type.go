// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ytypes

import (
	"fmt"
	"reflect"

	log "github.com/golang/glog"
	"github.com/openconfig/goyang/pkg/yang"
)

// Refer to: https://tools.ietf.org/html/rfc6020#section-9.2.

var (
	// defaultIntegerRange is the default allowed range of values for the key
	// integer type, if no other range restrictions are specified.
	defaultIntegerRange = map[yang.TypeKind]yang.YangRange{
		yang.Yint8:   yang.Int8Range,
		yang.Yint16:  yang.Int16Range,
		yang.Yint32:  yang.Int32Range,
		yang.Yint64:  yang.Int64Range,
		yang.Yuint8:  yang.Uint8Range,
		yang.Yuint16: yang.Uint16Range,
		yang.Yuint32: yang.Uint32Range,
		yang.Yuint64: yang.Uint64Range,
	}

	// typeKindFromKind maps the primitive type kinds of Go to the
	// enumerated TypeKind used in goyang.
	typeKindFromKind = map[reflect.Kind]yang.TypeKind{
		reflect.Int8:    yang.Yint8,
		reflect.Int16:   yang.Yint16,
		reflect.Int32:   yang.Yint32,
		reflect.Int64:   yang.Yint64,
		reflect.Uint8:   yang.Yuint8,
		reflect.Uint16:  yang.Yuint16,
		reflect.Uint32:  yang.Yuint32,
		reflect.Uint64:  yang.Yuint64,
		reflect.Bool:    yang.Ybool,
		reflect.Float64: yang.Ydecimal64,
		reflect.String:  yang.Ystring,
	}
)

// ValidateIntRestrictions checks that the given signed int matches the
// schema's range restrictions (if any). It returns an error if the validation
// fails.
func ValidateIntRestrictions(schemaType *yang.YangType, intVal int64) error {
	if !isInRanges(schemaType.Range, yang.FromInt(intVal)) {
		return fmt.Errorf("signed integer value %v is outside specified ranges", intVal)
	}
	return nil
}

// ValidateUintRestrictions checks that the given unsigned int matches the
// schema's range restrictions (if any). It returns an error if the validation
// fails.
func ValidateUintRestrictions(schemaType *yang.YangType, uintVal uint64) error {
	if !isInRanges(schemaType.Range, yang.FromUint(uintVal)) {
		return fmt.Errorf("unsigned integer value %v is outside specified ranges", uintVal)
	}
	return nil
}

// validateInt validates value, which must be a Go integer type, against the
// given schema.
func validateInt(schema *yang.Entry, value interface{}) error {
	// Check that the schema itself is valid.
	if err := validateIntSchema(schema); err != nil {
		return err
	}

	// util.DbgPrint("validateInt type %s with value %v", util.YangTypeToDebugString(schema.Type), value)

	kind := schema.Type.Kind

	// Check that type of value is the type expected from the schema.
	if typeKindFromKind[reflect.TypeOf(value).Kind()] != kind {
		return fmt.Errorf("non %v type %T with value %v for schema %s", kind, value, value, schema.Name)
	}

	// Check that the value satisfies any range restrictions.
	if isSigned(kind) {
		if err := ValidateIntRestrictions(schema.Type, reflect.ValueOf(value).Int()); err != nil {
			return fmt.Errorf("schema %q: %v", schema.Name, err)
		}
	} else {
		if err := ValidateUintRestrictions(schema.Type, reflect.ValueOf(value).Uint()); err != nil {
			return fmt.Errorf("schema %q: %v", schema.Name, err)
		}
	}

	return nil
}

// validateIntSlice validates value, which must be a Go integer slice type,
// against the given schema.
func validateIntSlice(schema *yang.Entry, value interface{}) error {
	// Check that the schema itself is valid.
	if err := validateIntSchema(schema); err != nil {
		return err
	}

	// util.DbgPrint("validateIntSlice type %s with value %v", util.YangTypeToDebugString(schema.Type), value)

	kind := schema.Type.Kind
	val := reflect.ValueOf(value)

	// Check that type of value is the type expected from the schema.
	if val.Kind() != reflect.Slice || yang.TypeKindFromName[reflect.TypeOf(value).Elem().Name()] != kind {
		return fmt.Errorf("got type %T with value %v, want []%v for schema %s", value, value, kind, schema.Name)
	}

	// Each slice element must be valid.
	for i := 0; i < val.Len(); i++ {
		if err := validateInt(schema, val.Index(i).Interface()); err != nil {
			return fmt.Errorf("invalid element at index %d: %v for schema %s", i, err, schema.Name)
		}
	}

	// Each slice element must be unique.
	// Refer to: https://tools.ietf.org/html/rfc6020#section-7.7.
	tbl := make(map[yang.Number]bool)
	for i := 0; i < val.Len(); i++ {
		v := toNumber(schema, val.Index(i))
		if tbl[v] {
			return fmt.Errorf("duplicate integer: %v for schema %s", v, schema.Name)
		}
		tbl[v] = true
	}

	return nil
}

// validateIntSchema validates the given integer type schema. This is a quick
// check rather than a comprehensive validation against the RFC.
// It is assumed that such a validation is done when the schema is parsed from
// source YANG.
func validateIntSchema(schema *yang.Entry) error {
	if schema == nil {
		return fmt.Errorf("int schema is nil")
	}
	if schema.Type == nil {
		return fmt.Errorf("int schema %s Type is nil", schema.Name)
	}
	kind := schema.Type.Kind
	ranges := schema.Type.Range

	if !isIntegerType(kind) {
		return fmt.Errorf("%v is not an integer type for schema %s", kind, schema.Name)
	}

	// Ensure ranges have valid value types.
	for _, r := range ranges {
		if r.Max.Less(r.Min) {
			return fmt.Errorf("int range cannot be a negative window %#v for schema %s", r, schema.Path())
		}
		if !isSigned(kind) && (r.Min.Negative || r.Max.Negative) {
			return fmt.Errorf("unsigned int cannot have negative range boundaries %v for schema %s", r, schema.Name)
		}
	}

	// Ensure range values fall within ranges for each type.
	for _, r := range ranges {
		if !legalValue(schema, r.Min) {
			return fmt.Errorf("min value %v for boundary is out of range for type %v for schema %s", r.Min.Value, kind, schema.Name)
		}
		if !legalValue(schema, r.Max) {
			return fmt.Errorf("max value %v for boundary is out of range for type %v for schema %s", r.Max.Value, kind, schema.Name)
		}
	}

	if len(ranges) != 0 {
		if errs := ranges.Validate(); errs != nil {
			return errs
		}
	}

	return nil
}

// legalValue reports whether val is within the range allowed for the given
// integer kind. kind must be an integer type.
func legalValue(schema *yang.Entry, val yang.Number) bool {
	yr := yang.YangRange{yang.YRange{Min: val, Max: val}}
	switch schema.Type.Kind {
	case yang.Yint8:
		return yang.Int8Range.Contains(yr)
	case yang.Yint16:
		return yang.Int16Range.Contains(yr)
	case yang.Yint32:
		return yang.Int32Range.Contains(yr)
	case yang.Yint64:
		return yang.Int64Range.Contains(yr)
	case yang.Yuint8:
		return yang.Uint8Range.Contains(yr)
	case yang.Yuint16:
		return yang.Uint16Range.Contains(yr)
	case yang.Yuint32:
		return yang.Uint32Range.Contains(yr)
	case yang.Yuint64:
		return yang.Uint64Range.Contains(yr)
	default:
		log.Errorf("illegal type %v in legalValue", schema.Type.Kind)
	}
	return false
}

// toNumber returns a yang.Number representation of val.
func toNumber(schema *yang.Entry, val reflect.Value) yang.Number {
	if isSigned(schema.Type.Kind) {
		return yang.FromInt(val.Int())
	}
	return yang.FromUint(val.Uint())
}

// isSigned reports whether kind is a signed integer type.
func isSigned(kind yang.TypeKind) bool {
	return kind == yang.Yint8 || kind == yang.Yint16 || kind == yang.Yint32 || kind == yang.Yint64
}

// isIntegerType reports whether schema is of an integer type.
func isIntegerType(kind yang.TypeKind) bool {
	switch kind {
	case yang.Yint8, yang.Yint16, yang.Yint32, yang.Yint64, yang.Yuint8, yang.Yuint16, yang.Yuint32, yang.Yuint64:
		return true
	default:
	}
	return false
}
