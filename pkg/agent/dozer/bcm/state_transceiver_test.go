// Copyright 2026 Hedgehog
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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
)

// dac is a DAC cable: it reports a length and a CMIS state, but no firmware
var dac = agentapi.SwitchStateTransceiver{
	Description:   "OSFP112 4x(200GBASE-CR2-DAC)-2.0M",
	CableClass:    "DAC",
	FormFactor:    "OSFP112",
	ConnectorType: "NO_SEPARABLE_CONNECTOR",
	Present:       "PRESENT",
	CableLength:   2,
	OperStatus:    "active",
	SerialNumber:  "MEO21Q10505",
	Vendor:        "OEM",
	VendorPart:    "MCP7Y40-N002",
	VendorOUI:     "48-B0-2D",
	VendorRev:     "01",
	CMISStatus:    "Ready",
	CMISRev:       "5.1",
}

// optic is a QSFP28 optic: no CMIS at all, no firmware and no cable length
var optic = agentapi.SwitchStateTransceiver{
	Description:   "QSFP28 100GBASE-CWDM4",
	CableClass:    "FIBER",
	FormFactor:    "QSFP28",
	ConnectorType: "LC_CONNECTOR",
	Present:       "PRESENT",
	OperStatus:    "active",
	SerialNumber:  "1QT-20001439",
	Vendor:        "O-NET",
	VendorPart:    "1AT-3Q3Q9211-01A",
	VendorOUI:     "34-78-77",
	VendorRev:     "01",
}

const (
	dacInfo = "cable_class=DAC,cmis_rev=5.1,conn_type=NO_SEPARABLE_CONNECTOR," +
		"descr=OSFP112 4x(200GBASE-CR2-DAC)-2.0M,firmware=,form_factor=OSFP112,length=2," +
		"serial=MEO21Q10505,transceiver=E1/1,vendor=OEM,vendor_oui=48-B0-2D," +
		"vendor_part=MCP7Y40-N002,vendor_rev=01"
	opticInfo = "cable_class=FIBER,cmis_rev=,conn_type=LC_CONNECTOR," +
		"descr=QSFP28 100GBASE-CWDM4,firmware=,form_factor=QSFP28,length=," +
		"serial=1QT-20001439,transceiver=E1/2,vendor=O-NET,vendor_oui=34-78-77," +
		"vendor_part=1AT-3Q3Q9211-01A,vendor_rev=01"
)

func TestUpdateTransceiverInfoMetrics(t *testing.T) {
	t.Parallel()

	failed := dac
	failed.CMISStatus = "Failed"
	failed.OperStatus = "inactive"
	failed.SerialNumber = "MEO21Q10506"

	reg := switchstate.NewRegistry()
	updateTransceiverInfoMetrics(reg, &agentapi.SwitchState{
		Transceivers: map[string]agentapi.SwitchStateTransceiver{
			"E1/1": dac,
			"E1/2": optic,
			"E1/3": failed,
			// an empty cage still reports a component, it just says nothing about it
			"E1/4": {},
		},
	})

	// every cage reports, so an empty port is visible as such
	require.Equal(t, map[string]float64{
		"transceiver=E1/1": 1,
		"transceiver=E1/2": 1,
		"transceiver=E1/3": 1,
		"transceiver=E1/4": 0,
	}, metricSeries(t, reg, "transceiver_present"))

	// only the present ones have an oper status to report
	require.Equal(t, map[string]float64{
		"transceiver=E1/1": 1,
		"transceiver=E1/2": 1,
		"transceiver=E1/3": 0,
	}, metricSeries(t, reg, "transceiver_active"))

	// the optic reports no CMIS state at all, which is not the same as not being ready
	require.Equal(t, map[string]float64{
		"transceiver=E1/1": 1,
		"transceiver=E1/3": 0,
	}, metricSeries(t, reg, "transceiver_cmis_ready"))

	info := metricSeries(t, reg, "transceiver_info")
	require.Len(t, info, 3)
	require.Equal(t, float64(1), info[dacInfo])
	require.Equal(t, float64(1), info[opticInfo])
}

// TestUpdateTransceiverInfoMetricsSwap covers the reset: a transceiver that's been swapped or pulled must not leave its
// series behind, and its identity is in the labels so a new one can't overwrite it.
func TestUpdateTransceiverInfoMetricsSwap(t *testing.T) {
	t.Parallel()

	reg := switchstate.NewRegistry()
	swState := &agentapi.SwitchState{
		Transceivers: map[string]agentapi.SwitchStateTransceiver{
			"E1/1": dac,
			"E1/2": optic,
		},
	}

	updateTransceiverInfoMetrics(reg, swState)
	require.Len(t, metricSeries(t, reg, "transceiver_info"), 2)

	// the DAC is replaced by an optic and the optic is pulled
	swState.Transceivers["E1/1"] = optic
	swState.Transceivers["E1/2"] = agentapi.SwitchStateTransceiver{}

	updateTransceiverInfoMetrics(reg, swState)

	// the only series left is the optic in its new port, reported under its own labels
	require.Equal(t, map[string]float64{
		strings.Replace(opticInfo, "transceiver=E1/2", "transceiver=E1/1", 1): 1,
	}, metricSeries(t, reg, "transceiver_info"))

	// neither of them reports a CMIS state now, so the DAC's Ready is gone rather than stuck at 1
	require.Empty(t, metricSeries(t, reg, "transceiver_cmis_ready"))
	require.Equal(t, map[string]float64{"transceiver=E1/1": 1}, metricSeries(t, reg, "transceiver_active"))
	require.Equal(t, map[string]float64{
		"transceiver=E1/1": 1,
		"transceiver=E1/2": 0,
	}, metricSeries(t, reg, "transceiver_present"))
}
