// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package valid

import (
	"reflect"

	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// isNil catches a nil pointer wrapped in the interface as well, which a plain nil check misses
func isNil(obj kclient.Object) bool {
	if obj == nil {
		return true
	}
	v := reflect.ValueOf(obj)

	return v.Kind() == reflect.Pointer && v.IsNil()
}
