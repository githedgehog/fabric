// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

func TestSwitchASNImmutable(t *testing.T) {
	oldSw := &wiringapi.Switch{Spec: wiringapi.SwitchSpec{ASN: 65100}}
	newSw := &wiringapi.Switch{Spec: wiringapi.SwitchSpec{ASN: 65099}}

	_, err := (&SwitchWebhook{}).ValidateUpdate(t.Context(), oldSw, newSw)
	require.ErrorContains(t, err, "asn is immutable")
}
