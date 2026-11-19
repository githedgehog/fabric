// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package meta

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
)

func TestValidateName(t *testing.T) {
	for _, tt := range []struct {
		name  string
		valid bool
	}{
		{name: "leaf-1", valid: true},
		{name: "ext.ext-1", valid: true},
		{name: strings.Repeat("a", MaxNameLength), valid: true},
		{name: strings.Repeat("a", MaxNameLength+1)},
		{name: ""},
		{name: "Leaf-1"},
		{name: "leaf_1"},
		{name: "leaf-"},
		{name: "a/b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName("name", tt.name)
			require.Equal(t, tt.valid, IsValidName(tt.name))
			if !tt.valid {
				require.ErrorIs(t, err, ErrInvalidName)

				return
			}

			require.NoError(t, err)
			// the names end up in labels, as values and as the name part of keys
			require.Empty(t, kvalidation.IsValidLabelValue(tt.name))
			require.Empty(t, kvalidation.IsQualifiedName("switch.fabric.githedgehog.com/"+tt.name))
		})
	}
}
