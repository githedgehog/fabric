// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package alloy

import (
	"fmt"
	"regexp"
)

var (
	alloyIdentRegex = regexp.MustCompile(`^[a-z]([_a-z0-9]*[a-z0-9])?$`)
	alloyIdentLen   = 32
)

func validateIdentifier(id string) error {
	if !alloyIdentRegex.MatchString(id) {
		return fmt.Errorf("invalid identifier: %s", id) //nolint:err113
	}
	if len(id) > alloyIdentLen {
		return fmt.Errorf("identifier too long: %s", id) //nolint:err113
	}

	return nil
}
