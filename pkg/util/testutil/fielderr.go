// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// RequireFieldError checks that err is a field error and that its path exists in obj's type, following the json
// tags, as the paths in validation are written by hand and nothing else would notice them drifting from the API.
func RequireFieldError(t testing.TB, err error, obj any) {
	t.Helper()

	fieldErr, ok := errors.AsType[*field.Error](err)
	require.True(t, ok, "expected a field error, got %v", err)
	require.NoError(t, ResolveFieldPath(reflect.TypeOf(obj), fieldErr.Field), "field error %v", fieldErr)
}

// ResolveFieldPath walks a field path, e.g. spec.subnets[foo].dhcp.range.start, through the json tags of fieldType
func ResolveFieldPath(fieldType reflect.Type, path string) error {
	for path != "" {
		fieldType = deref(fieldType)

		if key, rest, ok := strings.Cut(path, "]"); strings.HasPrefix(path, "[") && ok {
			if kind := fieldType.Kind(); kind != reflect.Map && kind != reflect.Slice && kind != reflect.Array {
				return fmt.Errorf("%s]: %s is not a map or a list", key, fieldType) //nolint:err113
			}
			fieldType, path = fieldType.Elem(), strings.TrimPrefix(rest, ".")

			continue
		}

		end := strings.IndexAny(path, ".[")
		if end < 0 {
			end = len(path)
		}
		name := path[:end]
		path = strings.TrimPrefix(path[end:], ".")

		next, ok := jsonField(fieldType, name)
		if !ok {
			return fmt.Errorf("%s: no field with json name %q", fieldType, name) //nolint:err113
		}
		fieldType = next
	}

	return nil
}

func jsonField(structType reflect.Type, name string) (reflect.Type, bool) {
	if structType.Kind() != reflect.Struct {
		return nil, false
	}

	for f := range structType.Fields() {
		tagName, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tagName == name {
			return f.Type, true
		}
		if f.Anonymous && tagName == "" {
			if inner, ok := jsonField(deref(f.Type), name); ok {
				return inner, true
			}
		}
	}

	return nil, false
}

func deref(ptrType reflect.Type) reflect.Type {
	for ptrType.Kind() == reflect.Pointer {
		ptrType = ptrType.Elem()
	}

	return ptrType
}
