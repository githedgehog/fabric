// Copyright 2023 Hedgehog
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

package switchstate

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
)

const (
	MetricNamespace = "fabric"
	MetricSubsystem = "agent"
)

type Registry struct {
	reg *prometheus.Registry

	stateSync sync.RWMutex
	state     *agentapi.SwitchState

	InterfaceMetrics   InterfaceMetrics
	InterfaceCounters  InterfaceCounters
	TransceiverMetrics TransceiverMetrics
	BGPNeighborMetrics BGPNeighborMetrics
	PlatformMetrics    PlatformMetrics
	CriticalResources  CRMMetrics

	AgentMetrics AgentMetrics
}

type InterfaceMetrics struct {
	Enabled      *prometheus.GaugeVec
	AdminStatus  *prometheus.GaugeVec
	OperStatus   *prometheus.GaugeVec
	LastChange   *prometheus.GaugeVec
	RateInterval *prometheus.GaugeVec
}

type InterfaceCounters struct {
	InBits                       *prometheus.GaugeVec
	InBitsPerSecond              *prometheus.GaugeVec
	InBroadcastPkts              *prometheus.GaugeVec
	InDiscards                   *prometheus.GaugeVec
	InErrors                     *prometheus.GaugeVec
	InMulticastPkts              *prometheus.GaugeVec
	InOctets                     *prometheus.GaugeVec
	InOctetsPerSecond            *prometheus.GaugeVec
	InPkts                       *prometheus.GaugeVec
	InPktsPerSecond              *prometheus.GaugeVec
	InUnicastPkts                *prometheus.GaugeVec
	InUtilization                *prometheus.GaugeVec
	LastClear                    *prometheus.GaugeVec
	OutBits                      *prometheus.GaugeVec
	OutBitsPerSecond             *prometheus.GaugeVec
	OutBroadcastPkts             *prometheus.GaugeVec
	OutDiscards                  *prometheus.GaugeVec
	OutErrors                    *prometheus.GaugeVec
	OutMulticastPkts             *prometheus.GaugeVec
	OutOctets                    *prometheus.GaugeVec
	OutOctetsPerSecond           *prometheus.GaugeVec
	OutPkts                      *prometheus.GaugeVec
	OutPktsPerSecond             *prometheus.GaugeVec
	OutUnicastPkts               *prometheus.GaugeVec
	OutUtilization               *prometheus.GaugeVec
	QueueDroppedBits             *prometheus.GaugeVec
	QueueDroppedOctets           *prometheus.GaugeVec
	QueueDroppedPkts             *prometheus.GaugeVec
	QueueECNMarkedBits           *prometheus.GaugeVec
	QueueECNMarkedOctets         *prometheus.GaugeVec
	QueueECNMarkedPkts           *prometheus.GaugeVec
	QueuePeriodicWatermark       *prometheus.GaugeVec
	QueuePersistentWatermark     *prometheus.GaugeVec
	QueueTransmitBits            *prometheus.GaugeVec
	QueueTransmitBitsPerSecond   *prometheus.GaugeVec
	QueueTransmitOctets          *prometheus.GaugeVec
	QueueTransmitOctetsPerSecond *prometheus.GaugeVec
	QueueTransmitPkts            *prometheus.GaugeVec
	QueueTransmitPktsPerSecond   *prometheus.GaugeVec
	QueueWatermark               *prometheus.GaugeVec
	QueueWREDDroppedPkts         *prometheus.GaugeVec
}

type TransceiverMetrics struct {
	AlarmRxPowerHi   *prometheus.GaugeVec
	AlarmRxPowerLo   *prometheus.GaugeVec
	AlarmTempHi      *prometheus.GaugeVec
	AlarmTempLo      *prometheus.GaugeVec
	AlarmTxBiasHi    *prometheus.GaugeVec
	AlarmTxBiasLo    *prometheus.GaugeVec
	AlarmTxPowerHi   *prometheus.GaugeVec
	AlarmTxPowerLo   *prometheus.GaugeVec
	AlarmVoltHi      *prometheus.GaugeVec
	AlarmVoltLo      *prometheus.GaugeVec
	Rx1Power         *prometheus.GaugeVec
	Rx2Power         *prometheus.GaugeVec
	Rx3Power         *prometheus.GaugeVec
	Rx4Power         *prometheus.GaugeVec
	Rx5Power         *prometheus.GaugeVec
	Rx6Power         *prometheus.GaugeVec
	Rx7Power         *prometheus.GaugeVec
	Rx8Power         *prometheus.GaugeVec
	Temperature      *prometheus.GaugeVec
	Tx1Bias          *prometheus.GaugeVec
	Tx1Power         *prometheus.GaugeVec
	Tx2Bias          *prometheus.GaugeVec
	Tx2Power         *prometheus.GaugeVec
	Tx3Bias          *prometheus.GaugeVec
	Tx3Power         *prometheus.GaugeVec
	Tx4Bias          *prometheus.GaugeVec
	Tx4Power         *prometheus.GaugeVec
	Tx5Bias          *prometheus.GaugeVec
	Tx5Power         *prometheus.GaugeVec
	Tx6Bias          *prometheus.GaugeVec
	Tx6Power         *prometheus.GaugeVec
	Tx7Bias          *prometheus.GaugeVec
	Tx7Power         *prometheus.GaugeVec
	Tx8Bias          *prometheus.GaugeVec
	Tx8Power         *prometheus.GaugeVec
	Voltage          *prometheus.GaugeVec
	WarningRxPowerHi *prometheus.GaugeVec
	WarningRxPowerLo *prometheus.GaugeVec
	WarningTempHi    *prometheus.GaugeVec
	WarningTempLo    *prometheus.GaugeVec
	WarningTxBiasHi  *prometheus.GaugeVec
	WarningTxBiasLo  *prometheus.GaugeVec
	WarningTxPowerHi *prometheus.GaugeVec
	WarningTxPowerLo *prometheus.GaugeVec
	WarningVoltHi    *prometheus.GaugeVec
	WarningVoltLo    *prometheus.GaugeVec
}

type BGPNeighborMetrics struct {
	ConnectionsDropped     *prometheus.GaugeVec
	Enabled                *prometheus.GaugeVec
	EstablishedTransitions *prometheus.GaugeVec
	PeerType               *prometheus.GaugeVec
	SessionState           *prometheus.GaugeVec
	Messages               BGPNeighborMetricsMessages
	Prefixes               BGPNeighborMetricsPrefixes
}

type BGPNeighborMetricsMessages struct {
	Received BGPNeighborMetricsMessagesCounters
	Sent     BGPNeighborMetricsMessagesCounters
}

type BGPNeighborMetricsPrefixes struct {
	Received          *prometheus.GaugeVec
	ReceivedPrePolicy *prometheus.GaugeVec
	Sent              *prometheus.GaugeVec
}

type BGPNeighborMetricsMessagesCounters struct {
	Capability   *prometheus.GaugeVec
	Keepalive    *prometheus.GaugeVec
	Notification *prometheus.GaugeVec
	Open         *prometheus.GaugeVec
	RouteRefresh *prometheus.GaugeVec
	Update       *prometheus.GaugeVec
}

type PlatformMetrics struct {
	Fan         PlatformFanMetrics
	PSU         PlatformPSUMetrics
	Temperature PlatformTemperatureMetrics
}

type PlatformFanMetrics struct {
	Speed    *prometheus.GaugeVec
	Presence *prometheus.GaugeVec
	Status   *prometheus.GaugeVec
}

type PlatformPSUMetrics struct {
	InputCurrent  *prometheus.GaugeVec
	InputPower    *prometheus.GaugeVec
	InputVoltage  *prometheus.GaugeVec
	OutputCurrent *prometheus.GaugeVec
	OutputPower   *prometheus.GaugeVec
	OutputVoltage *prometheus.GaugeVec
	Presence      *prometheus.GaugeVec
	Status        *prometheus.GaugeVec
}

type PlatformTemperatureMetrics struct {
	Temperature           *prometheus.GaugeVec
	HighThreshold         *prometheus.GaugeVec
	CriticalHighThreshold *prometheus.GaugeVec
	LowThreshold          *prometheus.GaugeVec
	CriticalLowThreshold  *prometheus.GaugeVec
}

type CRMMetrics struct {
	ACLStats CRMACLStatsMetrics
	Stats    CRMStatsMetrics
}

type CRMACLStatsMetrics struct {
	GroupsAvailable *prometheus.GaugeVec
	GroupsUsed      *prometheus.GaugeVec
	TablesAvailable *prometheus.GaugeVec
	TablesUsed      *prometheus.GaugeVec
}

type CRMStatsMetrics struct {
	DnatEntriesAvailable         prometheus.Gauge
	DnatEntriesUsed              prometheus.Gauge
	FdbEntriesAvailable          prometheus.Gauge
	FdbEntriesUsed               prometheus.Gauge
	IpmcEntriesAvailable         prometheus.Gauge
	IpmcEntriesUsed              prometheus.Gauge
	Ipv4NeighborsAvailable       prometheus.Gauge
	Ipv4NeighborsUsed            prometheus.Gauge
	Ipv4NexthopsAvailable        prometheus.Gauge
	Ipv4NexthopsUsed             prometheus.Gauge
	Ipv4RoutesAvailable          prometheus.Gauge
	Ipv4RoutesUsed               prometheus.Gauge
	Ipv6NeighborsAvailable       prometheus.Gauge
	Ipv6NeighborsUsed            prometheus.Gauge
	Ipv6NexthopsAvailable        prometheus.Gauge
	Ipv6NexthopsUsed             prometheus.Gauge
	Ipv6RoutesAvailable          prometheus.Gauge
	Ipv6RoutesUsed               prometheus.Gauge
	NexthopGroupMembersAvailable prometheus.Gauge
	NexthopGroupMembersUsed      prometheus.Gauge
	NexthopGroupsAvailable       prometheus.Gauge
	NexthopGroupsUsed            prometheus.Gauge
	SnatEntriesAvailable         prometheus.Gauge
	SnatEntriesUsed              prometheus.Gauge
}

type AgentMetrics struct {
	HeartbeatsTotal     prometheus.Counter
	Generation          prometheus.Gauge
	Version             *prometheus.GaugeVec
	HeartbeatDuration   prometheus.Histogram
	ConfigApplyDuration prometheus.Histogram
	KubeApplyDuration   prometheus.Histogram
}

func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()
	autoreg := promauto.With(reg)

	labels := prometheus.Labels{}

	newInterfaceGaugeVec := func(name string, help string) *prometheus.GaugeVec {
		return autoreg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   MetricNamespace,
			Subsystem:   MetricSubsystem,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		}, []string{"interface"})
	}

	newInterfaceQueueGaugeVec := func(name string, help string) *prometheus.GaugeVec {
		return autoreg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   MetricNamespace,
			Subsystem:   MetricSubsystem,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		}, []string{"interface", "queue"})
	}

	newTransceiverGaugeVec := func(name string, help string) *prometheus.GaugeVec {
		return autoreg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   MetricNamespace,
			Subsystem:   MetricSubsystem,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		}, []string{"transceiver"})
	}

	newBGPNeighborGaugeVec := func(name string, help string) *prometheus.GaugeVec {
		return autoreg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   MetricNamespace,
			Subsystem:   MetricSubsystem,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		}, []string{"vrf", "neighbor"})
	}

	newBGPNeighborPrefixesGaugeVec := func(name string, help string) *prometheus.GaugeVec {
		return autoreg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   MetricNamespace,
			Subsystem:   MetricSubsystem,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		}, []string{"vrf", "neighbor", "afisafi"})
	}

	newPlatformGaugeVec := func(name string, help string) *prometheus.GaugeVec {
		return autoreg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   MetricNamespace,
			Subsystem:   MetricSubsystem,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		}, []string{"name"})
	}

	newCRMACLStatsGaugeVec := func(name string, help string) *prometheus.GaugeVec {
		return autoreg.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   MetricNamespace,
			Subsystem:   MetricSubsystem,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		}, []string{"direction", "type"})
	}

	newCRMStatsGaugeVec := func(name string, help string) prometheus.Gauge {
		return autoreg.NewGauge(prometheus.GaugeOpts{
			Namespace:   MetricNamespace,
			Subsystem:   MetricSubsystem,
			Name:        name,
			Help:        help,
			ConstLabels: labels,
		})
	}

	r := &Registry{
		reg: reg,

		InterfaceMetrics: InterfaceMetrics{
			Enabled:      newInterfaceGaugeVec("interface_enabled", "Whether the interface is enabled"),
			AdminStatus:  newInterfaceGaugeVec("interface_admin_status", "Admin status of the interface"),
			OperStatus:   newInterfaceGaugeVec("interface_oper_status", "Operational status of the interface"),
			LastChange:   newInterfaceGaugeVec("interface_last_change", "Time of last change in interface status"),
			RateInterval: newInterfaceGaugeVec("interface_rate_interval", "Rate interval for interface counters"),
		},
		InterfaceCounters: InterfaceCounters{
			InBits:                       newInterfaceGaugeVec("interface_in_bits", "Incoming bits"),
			InBitsPerSecond:              newInterfaceGaugeVec("interface_in_bits_per_second", "Incoming bits per second"),
			InBroadcastPkts:              newInterfaceGaugeVec("interface_in_broadcast_pkts", "Incoming broadcast packets"),
			InDiscards:                   newInterfaceGaugeVec("interface_in_discards", "Incoming discards"),
			InErrors:                     newInterfaceGaugeVec("interface_in_errors", "Incoming errors"),
			InMulticastPkts:              newInterfaceGaugeVec("interface_in_multicast_pkts", "Incoming multicast packets"),
			InOctets:                     newInterfaceGaugeVec("interface_in_octets", "Incoming octets"),
			InOctetsPerSecond:            newInterfaceGaugeVec("interface_in_octets_per_second", "Incoming octets per second"),
			InPkts:                       newInterfaceGaugeVec("interface_in_pkts", "Incoming packets"),
			InPktsPerSecond:              newInterfaceGaugeVec("interface_in_pkts_per_second", "Incoming packets per second"),
			InUnicastPkts:                newInterfaceGaugeVec("interface_in_unicast_pkts", "Incoming unicast packets"),
			InUtilization:                newInterfaceGaugeVec("interface_in_utilization", "Incoming utilization"),
			LastClear:                    newInterfaceGaugeVec("interface_last_clear", "Time of last counter clear"),
			OutBits:                      newInterfaceGaugeVec("interface_out_bits", "Outgoing bits"),
			OutBitsPerSecond:             newInterfaceGaugeVec("interface_out_bits_per_second", "Outgoing bits per second"),
			OutBroadcastPkts:             newInterfaceGaugeVec("interface_out_broadcast_pkts", "Outgoing broadcast packets"),
			OutDiscards:                  newInterfaceGaugeVec("interface_out_discards", "Outgoing discards"),
			OutErrors:                    newInterfaceGaugeVec("interface_out_errors", "Outgoing errors"),
			OutMulticastPkts:             newInterfaceGaugeVec("interface_out_multicast_pkts", "Outgoing multicast packets"),
			OutOctets:                    newInterfaceGaugeVec("interface_out_octets", "Outgoing octets"),
			OutOctetsPerSecond:           newInterfaceGaugeVec("interface_out_octets_per_second", "Outgoing octets per second"),
			OutPkts:                      newInterfaceGaugeVec("interface_out_pkts", "Outgoing packets"),
			OutPktsPerSecond:             newInterfaceGaugeVec("interface_out_pkts_per_second", "Outgoing packets per second"),
			OutUnicastPkts:               newInterfaceGaugeVec("interface_out_unicast_pkts", "Outgoing unicast packets"),
			OutUtilization:               newInterfaceGaugeVec("interface_out_utilization", "Outgoing utilization"),
			QueueDroppedBits:             newInterfaceQueueGaugeVec("interface_queue_dropped_bits", "Queue dropped bits"),
			QueueDroppedOctets:           newInterfaceQueueGaugeVec("interface_queue_dropped_octets", "Queue dropped octets"),
			QueueDroppedPkts:             newInterfaceQueueGaugeVec("interface_queue_dropped_pkts", "Queue dropped packets"),
			QueueECNMarkedBits:           newInterfaceQueueGaugeVec("interface_queue_ecn_marked_bits", "Queue ECN marked bits"),
			QueueECNMarkedOctets:         newInterfaceQueueGaugeVec("interface_queue_ecn_marked_octets", "Queue ECN marked octets"),
			QueueECNMarkedPkts:           newInterfaceQueueGaugeVec("interface_queue_ecn_marked_pkts", "Queue ECN marked packets"),
			QueuePeriodicWatermark:       newInterfaceQueueGaugeVec("interface_queue_periodic_watermark", "Queue periodic watermark"),
			QueuePersistentWatermark:     newInterfaceQueueGaugeVec("interface_queue_persistent_watermark", "Queue persistent watermark"),
			QueueTransmitBits:            newInterfaceQueueGaugeVec("interface_queue_transmit_bits", "Queue transmit bits"),
			QueueTransmitBitsPerSecond:   newInterfaceQueueGaugeVec("interface_queue_transmit_bits_per_second", "Queue transmit bits per second"),
			QueueTransmitOctets:          newInterfaceQueueGaugeVec("interface_queue_transmit_octets", "Queue transmit octets"),
			QueueTransmitOctetsPerSecond: newInterfaceQueueGaugeVec("interface_queue_transmit_octets_per_second", "Queue transmit octets per second"),
			QueueTransmitPkts:            newInterfaceQueueGaugeVec("interface_queue_transmit_pkts", "Queue transmit packets"),
			QueueTransmitPktsPerSecond:   newInterfaceQueueGaugeVec("interface_queue_transmit_pkts_per_second", "Queue transmit packets per second"),
			QueueWatermark:               newInterfaceQueueGaugeVec("interface_queue_watermark", "Queue watermark"),
			QueueWREDDroppedPkts:         newInterfaceQueueGaugeVec("interface_queue_wred_dropped_pkts", "Queue WRED dropped packets"),
		},
		TransceiverMetrics: TransceiverMetrics{
			AlarmRxPowerHi:   newTransceiverGaugeVec("transceiver_alarm_rx_power_hi", "Alarm rx power hi"),
			AlarmRxPowerLo:   newTransceiverGaugeVec("transceiver_alarm_rx_power_lo", "Alarm rx power lo"),
			AlarmTempHi:      newTransceiverGaugeVec("transceiver_alarm_temp_hi", "Alarm temp hi"),
			AlarmTempLo:      newTransceiverGaugeVec("transceiver_alarm_temp_lo", "Alarm temp lo"),
			AlarmTxBiasHi:    newTransceiverGaugeVec("transceiver_alarm_tx_bias_hi", "Alarm tx bias hi"),
			AlarmTxBiasLo:    newTransceiverGaugeVec("transceiver_alarm_tx_bias_lo", "Alarm tx bias lo"),
			AlarmTxPowerHi:   newTransceiverGaugeVec("transceiver_alarm_tx_power_hi", "Alarm tx power hi"),
			AlarmTxPowerLo:   newTransceiverGaugeVec("transceiver_alarm_tx_power_lo", "Alarm tx power lo"),
			AlarmVoltHi:      newTransceiverGaugeVec("transceiver_alarm_volt_hi", "Alarm volt hi"),
			AlarmVoltLo:      newTransceiverGaugeVec("transceiver_alarm_volt_lo", "Alarm volt lo"),
			Rx1Power:         newTransceiverGaugeVec("transceiver_rx1_power", "Rx1 power"),
			Rx2Power:         newTransceiverGaugeVec("transceiver_rx2_power", "Rx2 power"),
			Rx3Power:         newTransceiverGaugeVec("transceiver_rx3_power", "Rx3 power"),
			Rx4Power:         newTransceiverGaugeVec("transceiver_rx4_power", "Rx4 power"),
			Rx5Power:         newTransceiverGaugeVec("transceiver_rx5_power", "Rx5 power"),
			Rx6Power:         newTransceiverGaugeVec("transceiver_rx6_power", "Rx6 power"),
			Rx7Power:         newTransceiverGaugeVec("transceiver_rx7_power", "Rx7 power"),
			Rx8Power:         newTransceiverGaugeVec("transceiver_rx8_power", "Rx8 power"),
			Temperature:      newTransceiverGaugeVec("transceiver_temperature", "Temperature"),
			Tx1Bias:          newTransceiverGaugeVec("transceiver_tx1_bias", "Tx1 bias"),
			Tx1Power:         newTransceiverGaugeVec("transceiver_tx1_power", "Tx1 power"),
			Tx2Bias:          newTransceiverGaugeVec("transceiver_tx2_bias", "Tx2 bias"),
			Tx2Power:         newTransceiverGaugeVec("transceiver_tx2_power", "Tx2 power"),
			Tx3Bias:          newTransceiverGaugeVec("transceiver_tx3_bias", "Tx3 bias"),
			Tx3Power:         newTransceiverGaugeVec("transceiver_tx3_power", "Tx3 power"),
			Tx4Bias:          newTransceiverGaugeVec("transceiver_tx4_bias", "Tx4 bias"),
			Tx4Power:         newTransceiverGaugeVec("transceiver_tx4_power", "Tx4 power"),
			Tx5Bias:          newTransceiverGaugeVec("transceiver_tx5_bias", "Tx5 bias"),
			Tx5Power:         newTransceiverGaugeVec("transceiver_tx5_power", "Tx5 power"),
			Tx6Bias:          newTransceiverGaugeVec("transceiver_tx6_bias", "Tx6 bias"),
			Tx6Power:         newTransceiverGaugeVec("transceiver_tx6_power", "Tx6 power"),
			Tx7Bias:          newTransceiverGaugeVec("transceiver_tx7_bias", "Tx7 bias"),
			Tx7Power:         newTransceiverGaugeVec("transceiver_tx7_power", "Tx7 power"),
			Tx8Bias:          newTransceiverGaugeVec("transceiver_tx8_bias", "Tx8 bias"),
			Tx8Power:         newTransceiverGaugeVec("transceiver_tx8_power", "Tx8 power"),
			Voltage:          newTransceiverGaugeVec("transceiver_voltage", "Voltage"),
			WarningRxPowerHi: newTransceiverGaugeVec("transceiver_warning_rx_power_hi", "Warning rx power hi"),
			WarningRxPowerLo: newTransceiverGaugeVec("transceiver_warning_rx_power_lo", "Warning rx power lo"),
			WarningTempHi:    newTransceiverGaugeVec("transceiver_warning_temp_hi", "Warning temp hi"),
			WarningTempLo:    newTransceiverGaugeVec("transceiver_warning_temp_lo", "Warning temp lo"),
			WarningTxBiasHi:  newTransceiverGaugeVec("transceiver_warning_tx_bias_hi", "Warning tx bias hi"),
			WarningTxBiasLo:  newTransceiverGaugeVec("transceiver_warning_tx_bias_lo", "Warning tx bias lo"),
			WarningTxPowerHi: newTransceiverGaugeVec("transceiver_warning_tx_power_hi", "Warning tx power hi"),
			WarningTxPowerLo: newTransceiverGaugeVec("transceiver_warning_tx_power_lo", "Warning tx power lo"),
			WarningVoltHi:    newTransceiverGaugeVec("transceiver_warning_volt_hi", "Warning volt hi"),
			WarningVoltLo:    newTransceiverGaugeVec("transceiver_warning_volt_lo", "Warning volt lo"),
		},
		BGPNeighborMetrics: BGPNeighborMetrics{
			ConnectionsDropped:     newBGPNeighborGaugeVec("bgp_neighbor_connections_dropped", "Number of dropped BGP connections"),
			Enabled:                newBGPNeighborGaugeVec("bgp_neighbor_enabled", "Whether the BGP neighbor is enabled"),
			EstablishedTransitions: newBGPNeighborGaugeVec("bgp_neighbor_established_transitions", "Number of established BGP neighbor transitions"),
			PeerType:               newBGPNeighborGaugeVec("bgp_neighbor_peer_type", "Type of BGP peer"),
			SessionState:           newBGPNeighborGaugeVec("bgp_neighbor_session_state", "State of BGP session"),
			Messages: BGPNeighborMetricsMessages{
				Received: BGPNeighborMetricsMessagesCounters{
					Capability:   newBGPNeighborGaugeVec("bgp_neighbor_messages_received_capability", "Number of received BGP capability messages"),
					Keepalive:    newBGPNeighborGaugeVec("bgp_neighbor_messages_received_keepalive", "Number of received BGP keepalive messages"),
					Notification: newBGPNeighborGaugeVec("bgp_neighbor_messages_received_notification", "Number of received BGP notification messages"),
					Open:         newBGPNeighborGaugeVec("bgp_neighbor_messages_received_open", "Number of received BGP open messages"),
					RouteRefresh: newBGPNeighborGaugeVec("bgp_neighbor_messages_received_route_refresh", "Number of received BGP route refresh messages"),
					Update:       newBGPNeighborGaugeVec("bgp_neighbor_messages_received_update", "Number of received BGP update messages"),
				},
				Sent: BGPNeighborMetricsMessagesCounters{
					Capability:   newBGPNeighborGaugeVec("bgp_neighbor_messages_sent_capability", "Number of sent BGP capability messages"),
					Keepalive:    newBGPNeighborGaugeVec("bgp_neighbor_messages_sent_keepalive", "Number of sent BGP keepalive messages"),
					Notification: newBGPNeighborGaugeVec("bgp_neighbor_messages_sent_notification", "Number of sent BGP notification messages"),
					Open:         newBGPNeighborGaugeVec("bgp_neighbor_messages_sent_open", "Number of sent BGP open messages"),
					RouteRefresh: newBGPNeighborGaugeVec("bgp_neighbor_messages_sent_route_refresh", "Number of sent BGP route refresh messages"),
					Update:       newBGPNeighborGaugeVec("bgp_neighbor_messages_sent_update", "Number of sent BGP update messages"),
				},
			},
			Prefixes: BGPNeighborMetricsPrefixes{
				Received:          newBGPNeighborPrefixesGaugeVec("bgp_neighbor_prefixes_received", "Number of received BGP prefixes"),
				ReceivedPrePolicy: newBGPNeighborPrefixesGaugeVec("bgp_neighbor_prefixes_received_pre_policy", "Number of received BGP prefixes pre-policy"),
				Sent:              newBGPNeighborPrefixesGaugeVec("bgp_neighbor_prefixes_sent", "Number of sent BGP prefixes"),
			},
		},
		PlatformMetrics: PlatformMetrics{
			Fan: PlatformFanMetrics{
				Speed:    newPlatformGaugeVec("platform_fan_speed", "Fan speed"),
				Presence: newPlatformGaugeVec("platform_fan_presence", "Fan presence"),
				Status:   newPlatformGaugeVec("platform_fan_status", "Fan status"),
			},
			PSU: PlatformPSUMetrics{
				InputCurrent:  newPlatformGaugeVec("platform_psu_input_current", "PSU input current"),
				InputPower:    newPlatformGaugeVec("platform_psu_input_power", "PSU input power"),
				InputVoltage:  newPlatformGaugeVec("platform_psu_input_voltage", "PSU input voltage"),
				OutputCurrent: newPlatformGaugeVec("platform_psu_output_current", "PSU output current"),
				OutputPower:   newPlatformGaugeVec("platform_psu_output_power", "PSU output power"),
				OutputVoltage: newPlatformGaugeVec("platform_psu_output_voltage", "PSU output voltage"),
				Presence:      newPlatformGaugeVec("platform_psu_presence", "PSU presence"),
				Status:        newPlatformGaugeVec("platform_psu_status", "PSU status"),
			},
			Temperature: PlatformTemperatureMetrics{
				Temperature:           newPlatformGaugeVec("platform_sensor_temperature", "Sensor temperature"),
				HighThreshold:         newPlatformGaugeVec("platform_sensor_high_threshold", "Sensor high threshold"),
				CriticalHighThreshold: newPlatformGaugeVec("platform_sensor_critical_high_threshold", "Sensor critical high threshold"),
				LowThreshold:          newPlatformGaugeVec("platform_sensor_low_threshold", "Sensor low threshold"),
				CriticalLowThreshold:  newPlatformGaugeVec("platform_sensor_critical_low_threshold", "Sensor critical low threshold"),
			},
		},
		CriticalResources: CRMMetrics{
			ACLStats: CRMACLStatsMetrics{
				GroupsAvailable: newCRMACLStatsGaugeVec("critical_resource_acl_groups_available", "Number of available groups"),
				GroupsUsed:      newCRMACLStatsGaugeVec("critical_resource_acl_groups_used", "Number of used groups"),
				TablesAvailable: newCRMACLStatsGaugeVec("critical_resource_acl_tables_available", "Number of available tables"),
				TablesUsed:      newCRMACLStatsGaugeVec("critical_resource_acl_tables_used", "Number of used tables"),
			},
			Stats: CRMStatsMetrics{
				DnatEntriesAvailable:         newCRMStatsGaugeVec("critical_resource_dnat_entries_available", "Number of available DNAT entries"),
				DnatEntriesUsed:              newCRMStatsGaugeVec("critical_resource_dnat_entries_used", "Number of used DNAT entries"),
				FdbEntriesAvailable:          newCRMStatsGaugeVec("critical_resource_fdb_entries_available", "Number of available FDB entries"),
				FdbEntriesUsed:               newCRMStatsGaugeVec("critical_resource_fdb_entries_used", "Number of used FDB entries"),
				IpmcEntriesAvailable:         newCRMStatsGaugeVec("critical_resource_ipmc_entries_available", "Number of available IPMC entries"),
				IpmcEntriesUsed:              newCRMStatsGaugeVec("critical_resource_ipmc_entries_used", "Number of used IPMC entries"),
				Ipv4NeighborsAvailable:       newCRMStatsGaugeVec("critical_resource_ipv4_neighbors_available", "Number of available IPv4 neighbors"),
				Ipv4NeighborsUsed:            newCRMStatsGaugeVec("critical_resource_ipv4_neighbors_used", "Number of used IPv4 neighbors"),
				Ipv4NexthopsAvailable:        newCRMStatsGaugeVec("critical_resource_ipv4_nexthops_available", "Number of available IPv4 nexthops"),
				Ipv4NexthopsUsed:             newCRMStatsGaugeVec("critical_resource_ipv4_nexthops_used", "Number of used IPv4 nexthops"),
				Ipv4RoutesAvailable:          newCRMStatsGaugeVec("critical_resource_ipv4_routes_available", "Number of available IPv4 routes"),
				Ipv4RoutesUsed:               newCRMStatsGaugeVec("critical_resource_ipv4_routes_used", "Number of used IPv4 routes"),
				Ipv6NeighborsAvailable:       newCRMStatsGaugeVec("critical_resource_ipv6_neighbors_available", "Number of available IPv6 neighbors"),
				Ipv6NeighborsUsed:            newCRMStatsGaugeVec("critical_resource_ipv6_neighbors_used", "Number of used IPv6 neighbors"),
				Ipv6NexthopsAvailable:        newCRMStatsGaugeVec("critical_resource_ipv6_nexthops_available", "Number of available IPv6 nexthops"),
				Ipv6NexthopsUsed:             newCRMStatsGaugeVec("critical_resource_ipv6_nexthops_used", "Number of used IPv6 nexthops"),
				Ipv6RoutesAvailable:          newCRMStatsGaugeVec("critical_resource_ipv6_routes_available", "Number of available IPv6 routes"),
				Ipv6RoutesUsed:               newCRMStatsGaugeVec("critical_resource_ipv6_routes_used", "Number of used IPv6 routes"),
				NexthopGroupMembersAvailable: newCRMStatsGaugeVec("critical_resource_nexthop_group_members_available", "Number of available nexthop group members"),
				NexthopGroupMembersUsed:      newCRMStatsGaugeVec("critical_resource_nexthop_group_members_used", "Number of used nexthop group members"),
				NexthopGroupsAvailable:       newCRMStatsGaugeVec("critical_resource_nexthop_groups_available", "Number of available nexthop groups"),
				NexthopGroupsUsed:            newCRMStatsGaugeVec("critical_resource_nexthop_groups_used", "Number of used nexthop groups"),
				SnatEntriesAvailable:         newCRMStatsGaugeVec("critical_resource_snat_entries_available", "Number of available SNAT entries"),
				SnatEntriesUsed:              newCRMStatsGaugeVec("critical_resource_snat_entries_used", "Number of used SNAT entries"),
			},
		},
		AgentMetrics: AgentMetrics{
			HeartbeatsTotal: autoreg.NewCounter(prometheus.CounterOpts{
				Namespace:   MetricNamespace,
				Subsystem:   MetricSubsystem,
				Name:        "agent_heartbeats_total",
				Help:        "Number of agent heartbeats",
				ConstLabels: labels,
			}),
			Generation: autoreg.NewGauge(prometheus.GaugeOpts{
				Namespace:   MetricNamespace,
				Subsystem:   MetricSubsystem,
				Name:        "agent_generation",
				Help:        "Generation of the agent K8s object",
				ConstLabels: labels,
			}),
			Version: autoreg.NewGaugeVec(prometheus.GaugeOpts{
				Namespace:   MetricNamespace,
				Subsystem:   MetricSubsystem,
				Name:        "agent_version",
				Help:        "Version of the agent binary",
				ConstLabels: labels,
			}, []string{"version"}),
			HeartbeatDuration: autoreg.NewHistogram(prometheus.HistogramOpts{
				Namespace:   MetricNamespace,
				Subsystem:   MetricSubsystem,
				Name:        "agent_heartbeat_duration_seconds",
				Help:        "Duration of agent heartbeats",
				ConstLabels: labels,
				Buckets:     []float64{3, 5, 7, 10, 15},
			}),
			ConfigApplyDuration: autoreg.NewHistogram(prometheus.HistogramOpts{
				Namespace:   MetricNamespace,
				Subsystem:   MetricSubsystem,
				Name:        "agent_config_apply_duration_seconds",
				Help:        "Duration of agent config applies",
				ConstLabels: labels,
				Buckets:     []float64{5, 10, 20, 30, 45, 60, 120, 300},
			}),
			KubeApplyDuration: autoreg.NewHistogram(prometheus.HistogramOpts{
				Namespace:   MetricNamespace,
				Subsystem:   MetricSubsystem,
				Name:        "agent_kube_apply_duration_seconds",
				Help:        "Duration of agent config from kube applies",
				ConstLabels: labels,
				Buckets:     []float64{5, 10, 20, 30, 45, 60, 120, 300},
			}),
		},
	}

	return r
}

func (r *Registry) GetSwitchState() *agentapi.SwitchState {
	r.stateSync.RLock()
	defer r.stateSync.RUnlock()

	return r.state
}

func (r *Registry) SaveSwitchState(state *agentapi.SwitchState) {
	r.stateSync.Lock()
	defer r.stateSync.Unlock()

	r.state = state
}

func (r *Registry) ServeMetrics(port uint16) error {
	router := chi.NewRouter()
	router.Use(middleware.Recoverer)
	router.Use(middleware.Heartbeat("/ping"))

	router.Handle("/metrics", promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{
		Registry: r.reg,
		Timeout:  10 * time.Second,
	}))

	server := &http.Server{
		Handler:           router,
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		ReadHeaderTimeout: 30 * time.Second,
	}

	return errors.Wrapf(server.ListenAndServe(), "failed to start metrics server at %s", server.Addr)
}
