// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckRelayedRequest(t *testing.T) {
	s := &Server{
		relayAllowlist: map[string]struct{}{
			"10.0.0.1": {},
		},
	}

	for _, tt := range []struct {
		name     string
		giaddr   string
		wantDrop bool
	}{
		{
			name:     "unrelayed (GIADDR 0.0.0.0) is allowed",
			giaddr:   "0.0.0.0",
			wantDrop: false,
		},
		{
			name:     "unknown GIADDR is dropped",
			giaddr:   "192.168.99.1",
			wantDrop: true,
		},
		{
			name:     "known leaf GIADDR is allowed",
			giaddr:   "10.0.0.1",
			wantDrop: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			giaddr := net.ParseIP(tt.giaddr)
			require.NotNil(t, giaddr)
			got := s.checkRelayedRequest(giaddr)
			if tt.wantDrop {
				assert.Error(t, got, "expected error")
			} else {
				assert.NoError(t, got, "expected no error")
			}
		})
	}
}
