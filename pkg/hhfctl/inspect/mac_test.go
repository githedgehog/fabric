// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareMACPorts(t *testing.T) {
	// MAC ports are "agentName/ifaceName". Sorting must group by switch
	// (natural order on the agent name) and, within a switch, keep management
	// ports first and order data ports naturally (Ethernet2 before Ethernet10).
	// A plain lexical sort would put leaf-10 before leaf-2 and Ethernet10
	// before Ethernet2.
	got := []string{
		"leaf-10/Ethernet2",
		"leaf-2/Ethernet1",
		"leaf-2/Ethernet10",
		"leaf-2/Ethernet2",
		"leaf-2/M1",
		"leaf-10/Ethernet1",
	}

	slices.SortFunc(got, compareMACPorts)

	require.Equal(t, []string{
		"leaf-2/M1",
		"leaf-2/Ethernet1",
		"leaf-2/Ethernet2",
		"leaf-2/Ethernet10",
		"leaf-10/Ethernet1",
		"leaf-10/Ethernet2",
	}, got)
}
