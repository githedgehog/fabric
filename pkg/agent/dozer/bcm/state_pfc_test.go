// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
)

// trimmed from what a VS switch returns for /qos/interfaces/interface
const pfcQoSJSON = `{"openconfig-qos:qos": {"interfaces": {"interface": [{
  "interface-id": "Ethernet0",
  "config": {"interface-id": "Ethernet0"},
  "output": {"queues": {"queue": [
    {"name": "Ethernet0:3", "state": {"name": "Ethernet0:3", "traffic-type": "UC", "transmit-pkts": "10"}},
    {"name": "Ethernet0:13", "state": {"name": "Ethernet0:13", "traffic-type": "MC", "transmit-pkts": "0"}}
  ]}},
  "pfc": {
    "pfc-priorities": {"pfc-priority": [
      {"dot1p": 0, "config": {"dot1p": 0, "enable": false},
       "state": {"dot1p": 0, "enable": false, "statistics": {"pause-frames-rx": "0", "pause-frames-tx": "0"}}},
      {"dot1p": 3, "config": {"dot1p": 3, "enable": true},
       "state": {"dot1p": 3, "enable": true, "statistics": {"pause-frames-rx": "42", "pause-frames-tx": "7"}}}
    ]},
    "pfc-queue": {"pfc-queue": [
      {"queue": 3, "config": {"queue": 3}, "state": {"queue": 3, "statistics": {"queue": 3,
        "rx-drop": "5", "rx-drop-last": "0", "rx-ok": "0", "rx-ok-last": "0", "status": "",
        "storm-detected": "2", "storm-restored": "1", "tx-drop": "6", "tx-drop-last": "0", "tx-ok": "0", "tx-ok-last": "0"}}},
      {"queue": 13, "config": {"queue": 13}, "state": {"queue": 13, "statistics": {"queue": 13,
        "rx-drop": "0", "storm-detected": "0", "storm-restored": "0", "tx-drop": "0"}}},
      {"queue": 19, "config": {"queue": 19}, "state": {"queue": 19, "statistics": {"queue": 19,
        "rx-drop": "0", "storm-detected": "0", "storm-restored": "0", "tx-drop": "0"}}}
    ]}
  }
}]}}}`

func TestInterfacePFCMetrics(t *testing.T) {
	client := newGNMIMock()
	require.NoError(t, oc.Unmarshal([]byte(pfcQoSJSON), client.root))

	p := &BroadcomProcessor{client: client}
	reg := switchstate.NewRegistry()
	swState := &agentapi.SwitchState{Interfaces: map[string]agentapi.SwitchStateInterface{"E1/1": {Enabled: true}}}
	require.NoError(t, p.updateInterfaceQueuesMetrics(t.Context(), reg, swState, &agentapi.Agent{}, map[string]string{"Ethernet0": "E1/1"}))

	val := func(g *prometheus.GaugeVec, labels ...string) float64 {
		m := &dto.Metric{}
		require.NoError(t, g.WithLabelValues(labels...).Write(m))

		return m.GetGauge().GetValue()
	}

	c := reg.InterfaceCounters
	require.InDelta(t, 0, val(c.PFCEnabled, "E1/1", "0", "E1/1"), 0)
	require.InDelta(t, 1, val(c.PFCEnabled, "E1/1", "3", "E1/1"), 0)
	require.InDelta(t, 42, val(c.PFCPauseFramesRx, "E1/1", "3", "E1/1"), 0)
	require.InDelta(t, 7, val(c.PFCPauseFramesTx, "E1/1", "3", "E1/1"), 0)
	require.InDelta(t, 2, val(c.QueuePFCStormDetected, "E1/1", "UC3", "E1/1"), 0)
	require.InDelta(t, 1, val(c.QueuePFCStormRestored, "E1/1", "UC3", "E1/1"), 0)
	require.InDelta(t, 5, val(c.QueuePFCRxDroppedPkts, "E1/1", "UC3", "E1/1"), 0)
	require.InDelta(t, 6, val(c.QueuePFCTxDroppedPkts, "E1/1", "UC3", "E1/1"), 0)

	count := func(c prometheus.Collector) int {
		ch := make(chan prometheus.Metric, 10)
		c.Collect(ch)
		close(ch)

		return len(ch)
	}

	// pause frames only for priority 3, the one with PFC on
	require.Equal(t, 2, count(c.PFCEnabled))
	require.Equal(t, 1, count(c.PFCPauseFramesRx))
	require.Equal(t, 1, count(c.PFCPauseFramesTx))
	// only UC3: MC13 is a multicast queue and 19 has no output queue to name it after
	require.Equal(t, 1, count(c.QueuePFCStormDetected))

	pfc := client.root.Qos.Interfaces.Interface["Ethernet0"].Pfc
	update := func() {
		require.NoError(t, p.updateInterfaceQueuesMetrics(t.Context(), reg, swState, &agentapi.Agent{}, map[string]string{"Ethernet0": "E1/1"}))
	}

	// a watchdog queue the switch stops reporting is dropped while PFC stays on
	delete(pfc.PfcQueue.PfcQueue, 3)
	update()
	require.Equal(t, 1, count(c.PFCPauseFramesRx))
	require.Equal(t, 0, count(c.QueuePFCStormDetected))
	require.Equal(t, 0, count(c.QueuePFCStormRestored))
	require.Equal(t, 0, count(c.QueuePFCRxDroppedPkts))
	require.Equal(t, 0, count(c.QueuePFCTxDroppedPkts))

	// turning PFC off drops the pause frames reported so far
	prio3 := pfc.PfcPriorities.PfcPriority[3]
	prio3.Config.Enable = new(false)
	prio3.State.Enable = new(false)
	update()
	require.Equal(t, 2, count(c.PFCEnabled))
	require.Equal(t, 0, count(c.PFCPauseFramesRx))
	require.Equal(t, 0, count(c.PFCPauseFramesTx))

	// and so does a priority the switch stops reporting
	prio3.Config.Enable = new(true)
	prio3.State.Enable = new(true)
	update()
	require.Equal(t, 1, count(c.PFCPauseFramesRx))
	delete(pfc.PfcPriorities.PfcPriority, 3)
	update()
	require.Equal(t, 1, count(c.PFCEnabled))
	require.Equal(t, 0, count(c.PFCPauseFramesRx))
	require.Equal(t, 0, count(c.PFCPauseFramesTx))
}
