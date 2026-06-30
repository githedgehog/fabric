// Copyright 2024 Hedgehog
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

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

func TestSpecInterfaceEthernetFECEnforcer(t *testing.T) {
	const port = IfacePrefixPhysical + "0"

	for _, tt := range []struct {
		name        string
		actual      *dozer.SpecInterface
		desired     *dozer.SpecInterface
		wantActions int
	}{
		{
			// Unmanaged port: no explicit FEC requested. The device reports a concrete mode
			// (rs), but with no desired FEC the enforcer must not act (no drift).
			name:        "no desired FEC, device reports rs",
			actual:      &dozer.SpecInterface{FEC: pointer.To("rs")},
			desired:     &dozer.SpecInterface{FEC: nil},
			wantActions: 0,
		},
		{
			name:        "explicit rs desired, device unset",
			actual:      &dozer.SpecInterface{FEC: nil},
			desired:     &dozer.SpecInterface{FEC: pointer.To("rs")},
			wantActions: 1,
		},
		{
			name:        "explicit rs desired, device already rs",
			actual:      &dozer.SpecInterface{FEC: pointer.To("rs")},
			desired:     &dozer.SpecInterface{FEC: pointer.To("rs")},
			wantActions: 0,
		},
		{
			name:        "explicit fc desired, device reports rs",
			actual:      &dozer.SpecInterface{FEC: pointer.To("rs")},
			desired:     &dozer.SpecInterface{FEC: pointer.To("fc")},
			wantActions: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actions := &ActionQueue{}
			err := specInterfaceEthernetFECEnforcer.Handle("", port, tt.actual, tt.desired, actions)
			require.NoError(t, err)
			require.Len(t, actions.actions, tt.wantActions)
		})
	}
}
