// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"golang.org/x/sync/errgroup"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// API name of the management port, the only one the profile maps by role instead of by NOS name
	mgmtPort = "M1"

	// The same vocabulary the SONiC agent uses, so that the dashboards work for both
	transceiverPresent    = "PRESENT"
	transceiverNotPresent = "NOT_PRESENT"
	transceiverOperActive = "active"

	nosPortPrefix       = "swp"
	notAvailable        = "N/A"
	lldpPortTypeMAC     = "mac"
	lastChangeLayout    = "2006/01/02 15:04:05.000"
	lastChangeUnknown   = "N/A"
	unicastQueuePrefix  = "UC"
	collectConcurrency  = 8
	vrfDefault          = "default"
	vrfMgmt             = "mgmt"
	bgpSessionClearing  = "clearing"
	bgpSessionDeleted   = "deleted"
	bgpNone             = "none"
	bgpRouterIDUnknown  = "0.0.0.0"
	secondsPerMinute    = 60
	secondsPerHour      = 60 * secondsPerMinute
	secondsPerDay       = 24 * secondsPerHour
	maxUtilizationPct   = 100
	microsecondsPerUnit = 1000
)

// UpdateSwitchState collects the switch state and metrics over the NVUE API. Without a client, which is the case
// until the agent has credentials for it, only the NOS version is reported from the local filesystem.
func (c *CumulusProcessor) UpdateSwitchState(ctx context.Context, agent *agentapi.Agent, reg *switchstate.Registry) error {
	if c.client == nil {
		c.warnNoClient.Do(func() {
			slog.Warn("NVUE client isn't configured, only reporting the NOS version")
		})

		return updateSwitchStateOffline(reg)
	}

	if agent.Spec.SwitchProfile == nil {
		return errNoSwitchProfile
	}

	portMap, err := agent.Spec.SwitchProfile.GetNOS2APIPortsFor(&agent.Spec.Switch)
	if err != nil {
		return fmt.Errorf("getting port mapping: %w", err)
	}

	start := time.Now()

	swState := &agentapi.SwitchState{
		Interfaces:   map[string]agentapi.SwitchStateInterface{},
		Breakouts:    map[string]agentapi.SwitchStateBreakout{},
		Transceivers: map[string]agentapi.SwitchStateTransceiver{},
		BGPNeighbors: map[string]map[string]agentapi.SwitchStateBGPNeighbor{},
		BFDPeers:     map[string]map[string]agentapi.SwitchStateBFDPeer{},
		Platform: agentapi.SwitchStatePlatform{
			Fans:         map[string]agentapi.SwitchStatePlatformFan{},
			PSUs:         map[string]agentapi.SwitchStatePlatformPSU{},
			Temperatures: map[string]agentapi.SwitchStatePlatformTemperature{},
		},
		Firmware: map[string]string{},
	}

	system, err := c.client.System(ctx)
	if err != nil {
		return fmt.Errorf("getting system: %w", err)
	}

	// the switch reports timestamps in its local time
	loc, err := time.LoadLocation(system.DateTime.Timezone)
	if err != nil {
		slog.Warn("Unknown switch timezone, assuming the local one", "timezone", system.DateTime.Timezone, "err", err)
		loc = time.Local
	}

	if err := c.updateInterfaces(ctx, reg, swState, portMap, loc); err != nil {
		return fmt.Errorf("updating interfaces: %w", err)
	}

	if err := c.updateTransceivers(ctx, reg, swState, portMap); err != nil {
		return fmt.Errorf("updating transceivers: %w", err)
	}

	if err := c.updateBGPNeighbors(ctx, reg, swState, agent, portMap); err != nil {
		return fmt.Errorf("updating bgp neighbors: %w", err)
	}

	if err := c.updatePlatform(ctx, reg, swState); err != nil {
		return fmt.Errorf("updating platform: %w", err)
	}

	if err := c.updateNOS(ctx, swState, system); err != nil {
		return fmt.Errorf("updating nos: %w", err)
	}

	// after everything that contributes to the transceiver state has run, it's collected from a few different paths
	updateTransceiverInfoMetrics(reg, swState)

	reg.SaveSwitchState(swState)

	slog.Debug("Switch state updated", "took", time.Since(start))

	return nil
}

func updateSwitchStateOffline(reg *switchstate.Registry) error {
	osReleaseData, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return fmt.Errorf("reading os-release: %w", err)
	}

	version := ""
	for line := range strings.Lines(string(osReleaseData)) {
		if after, ok := strings.CutPrefix(line, "VERSION="); ok {
			after = strings.TrimSpace(after)
			if len(after) > 2 {
				version = after[1 : len(after)-1]
			}

			break
		}
	}

	reg.SaveSwitchState(&agentapi.SwitchState{
		NOS: agentapi.SwitchStateNOS{
			SoftwareVersion: version,
		},
	})

	return nil
}

func (c *CumulusProcessor) updateInterfaces(ctx context.Context, reg *switchstate.Registry, swState *agentapi.SwitchState, portMap map[string]string, loc *time.Location) error {
	ifaces, err := c.client.Interfaces(ctx)
	if err != nil {
		return fmt.Errorf("getting interfaces: %w", err)
	}

	rates, err := c.client.InterfaceRates(ctx)
	if err != nil {
		return fmt.Errorf("getting interface rates: %w", err)
	}

	// neighbors are labeled by their own identity, the ones that are gone have to take their series with them
	reg.LLDPMetrics.LastUpdate.Reset()
	reg.LLDPMetrics.TTL.Reset()
	reg.LLDPMetrics.Info.Reset()

	// the traffic counters are only served per port, see below
	withCounters := map[string]string{}

	for nosName, iface := range ifaces {
		apiName, ok := apiInterfaceName(nosName, iface.Type, portMap)
		if !ok || iface.Link == nil {
			continue
		}

		transceiverName := transceiverForPort(apiName)
		ifState := interfaceState(nosName, iface.Link, loc)

		adminStatusID, err := ifState.AdminStatus.ID()
		if err != nil {
			return fmt.Errorf("getting admin status ID: %w", err)
		}
		operStatusID, err := ifState.OperStatus.ID()
		if err != nil {
			return fmt.Errorf("getting oper status ID: %w", err)
		}

		reg.InterfaceMetrics.Enabled.WithLabelValues(apiName, transceiverName).Set(boolToFloat64(ifState.Enabled))
		reg.InterfaceMetrics.AdminStatus.WithLabelValues(apiName, transceiverName).Set(float64(adminStatusID))
		reg.InterfaceMetrics.OperStatus.WithLabelValues(apiName, transceiverName).Set(float64(operStatusID))
		if !ifState.LastChange.IsZero() {
			reg.InterfaceMetrics.LastChange.WithLabelValues(apiName, transceiverName).Set(float64(ifState.LastChange.Unix()))
		}

		if r, ok := rates[nosName]; ok && ifState.Enabled {
			ifState.Counters = ratesCounters(reg, apiName, transceiverName, &r)
		}

		// every switch and IPMI would show up on the management interface
		if iface.LLDP != nil && iface.Type != nvue.InterfaceTypeEth {
			ifState.LLDPNeighbors = lldpNeighbors(iface.LLDP.Neighbor)
		}

		swState.Interfaces[apiName] = ifState

		if ifState.Enabled && iface.Type != nvue.InterfaceTypeEth {
			withCounters[nosName] = apiName
		}
	}

	counters, err := c.collectCounters(ctx, withCounters)
	if err != nil {
		return err
	}

	for apiName, ifCounters := range counters {
		ifState := swState.Interfaces[apiName]
		applyCounters(reg, &ifState, apiName, transceiverForPort(apiName), ifCounters)
		swState.Interfaces[apiName] = ifState
	}

	// iterating the ports and not the neighbors: a port without any has to report zero, nothing ever resets it
	for _, apiName := range portMap {
		if apiName == mgmtPort {
			continue
		}

		transceiverName := transceiverForPort(apiName)
		neighbors := swState.Interfaces[apiName].LLDPNeighbors
		reg.LLDPMetrics.Neighbors.WithLabelValues(apiName, transceiverName).Set(float64(len(neighbors)))

		for _, neighbor := range neighbors {
			reg.LLDPMetrics.Info.WithLabelValues(
				apiName, neighbor.SystemName, neighbor.Port, neighbor.MAC, neighbor.ChassisID,
				neighbor.SystemDescription, neighbor.Manufacturer, neighbor.Model, neighbor.SerialNumber,
				transceiverName,
			).Set(1)

			if neighbor.TTL > 0 {
				reg.LLDPMetrics.TTL.
					WithLabelValues(apiName, neighbor.SystemName, neighbor.Port, neighbor.MAC, transceiverName).
					Set(float64(neighbor.TTL))
			}
		}
	}

	return nil
}

// apiInterfaceName translates the NOS name of an interface into the API one, or reports that the interface isn't
// one the fabric tracks: only the front panel ports, the management port and the bonds are.
func apiInterfaceName(nosName, ifaceType string, portMap map[string]string) (string, bool) {
	switch ifaceType {
	case nvue.InterfaceTypeSWP, nvue.InterfaceTypeEth:
		apiName, ok := portMap[nosName]
		if !ok {
			// some models have a second management port the profiles don't list
			if ifaceType == nvue.InterfaceTypeEth {
				slog.Debug("Port mapping not found, ignoring for metrics", "interface", nosName)
			} else {
				slog.Warn("Port mapping not found, ignoring for metrics", "interface", nosName)
			}

			return "", false
		}

		return apiName, true
	case nvue.InterfaceTypeBond:
		return nosName, true
	default:
		return "", false
	}
}

func interfaceState(nosName string, link *nvue.InterfaceLink, loc *time.Location) agentapi.SwitchStateInterface {
	st := agentapi.SwitchStateInterface{
		MAC:           link.MACAddress,
		Speed:         link.Speed,
		AutoNegotiate: link.AutoNegotiate == nvue.StateEnabled,
		FEC:           link.FEC,
		// protodown is how every protocol (link flap protection, BPDU guard, MLAG...) takes a port down
		ErrDisabled: link.Protodown != "" && link.Protodown != nvue.StateDisabled,
	}

	switch link.AdminStatus {
	case "":
		st.AdminStatus = agentapi.AdminStatusUnset
	case string(agentapi.AdminStatusUp):
		st.AdminStatus = agentapi.AdminStatusUp
	case string(agentapi.AdminStatusDown):
		st.AdminStatus = agentapi.AdminStatusDown
	default:
		slog.Warn("Unknown admin status, ignoring", "interface", nosName, "adminStatus", link.AdminStatus)
	}
	st.Enabled = st.AdminStatus == agentapi.AdminStatusUp

	switch link.OperStatus {
	case "":
		st.OperStatus = agentapi.OperStatusUnset
	case string(agentapi.OperStatusUp):
		st.OperStatus = agentapi.OperStatusUp
	case string(agentapi.OperStatusDown):
		st.OperStatus = agentapi.OperStatusDown
	case string(agentapi.OperStatusTesting):
		st.OperStatus = agentapi.OperStatusTesting
	case string(agentapi.OperStatusDormant):
		st.OperStatus = agentapi.OperStatusDormant
	case "not-present":
		st.OperStatus = agentapi.OperStatusNotPresent
	case "lower-layer-down":
		st.OperStatus = agentapi.OperStatusLowerLayerDown
	default:
		st.OperStatus = agentapi.OperStatusUnknown
	}

	if link.OperStatusLastChange != "" && link.OperStatusLastChange != lastChangeUnknown {
		lastChange, err := time.ParseInLocation(lastChangeLayout, link.OperStatusLastChange, loc)
		if err != nil {
			slog.Debug("Failed to parse interface last change, ignoring", "interface", nosName, "lastChange", link.OperStatusLastChange, "err", err)
		} else {
			st.LastChange = kmetav1.Time{Time: lastChange}
		}
	}

	return st
}

func ratesCounters(reg *switchstate.Registry, apiName, transceiverName string, rates *nvue.InterfaceRates) *agentapi.SwitchStateInterfaceCounters {
	counters := &agentapi.SwitchStateInterfaceCounters{
		InBitsPerSecond:  cleanupFloat(rates.InBitsRate.Value),
		InPktsPerSecond:  cleanupFloat(rates.InPktsRate.Value),
		InUtilization:    utilizationPercent(rates.InUtilization),
		OutBitsPerSecond: cleanupFloat(rates.OutBitsRate.Value),
		OutPktsPerSecond: cleanupFloat(rates.OutPktsRate.Value),
		OutUtilization:   utilizationPercent(rates.OutUtilization),
	}

	if rates.LoadInterval.OK {
		reg.InterfaceMetrics.RateInterval.WithLabelValues(apiName, transceiverName).Set(rates.LoadInterval.Value)
	}

	reg.InterfaceCounters.InBitsPerSecond.WithLabelValues(apiName, transceiverName).Set(counters.InBitsPerSecond)
	reg.InterfaceCounters.InOctetsPerSecond.WithLabelValues(apiName, transceiverName).Set(counters.InBitsPerSecond / 8)
	reg.InterfaceCounters.InPktsPerSecond.WithLabelValues(apiName, transceiverName).Set(counters.InPktsPerSecond)
	reg.InterfaceCounters.InUtilization.WithLabelValues(apiName, transceiverName).Set(float64(counters.InUtilization))
	reg.InterfaceCounters.OutBitsPerSecond.WithLabelValues(apiName, transceiverName).Set(counters.OutBitsPerSecond)
	reg.InterfaceCounters.OutOctetsPerSecond.WithLabelValues(apiName, transceiverName).Set(counters.OutBitsPerSecond / 8)
	reg.InterfaceCounters.OutPktsPerSecond.WithLabelValues(apiName, transceiverName).Set(counters.OutPktsPerSecond)
	reg.InterfaceCounters.OutUtilization.WithLabelValues(apiName, transceiverName).Set(float64(counters.OutUtilization))

	return counters
}

// utilizationPercent rounds the percentage NVUE reports to the whole percent the state keeps
func utilizationPercent(n nvue.Number) uint8 {
	if !n.OK || n.Value < 0 {
		return 0
	}

	return uint8(math.Round(min(n.Value, maxUtilizationPct)))
}

// collectCounters fetches the per-port counters of the ports (NOS name to API name) concurrently, the API only
// serves them one port at a time and there can be hundreds with breakouts. Ports that don't keep any are skipped.
func (c *CumulusProcessor) collectCounters(ctx context.Context, ports map[string]string) (map[string]*nvue.InterfaceCounters, error) {
	var mu sync.Mutex
	counters := make(map[string]*nvue.InterfaceCounters, len(ports))

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(collectConcurrency)

	for nosName, apiName := range ports {
		group.Go(func() error {
			ifCounters, err := c.client.InterfaceCounters(ctx, nosName)
			if err != nil {
				if errors.Is(err, nvue.ErrNotFound) {
					return nil
				}

				return fmt.Errorf("getting counters for %s: %w", nosName, err)
			}

			mu.Lock()
			counters[apiName] = ifCounters
			mu.Unlock()

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err //nolint:wrapcheck // it's our own error from the goroutine
	}

	return counters, nil
}

func applyCounters(reg *switchstate.Registry, ifState *agentapi.SwitchStateInterface, apiName, transceiverName string, counters *nvue.InterfaceCounters) {
	if ifState.Counters == nil {
		ifState.Counters = &agentapi.SwitchStateInterfaceCounters{}
	}
	st := ifState.Counters

	if counters.InBytes.OK {
		reg.InterfaceCounters.InOctets.WithLabelValues(apiName, transceiverName).Set(float64(counters.InBytes.Value))
		reg.InterfaceCounters.InBits.WithLabelValues(apiName, transceiverName).Set(float64(counters.InBytes.Value * 8))
		st.InBits = counters.InBytes.Value * 8
	}
	if counters.InPkts.OK {
		reg.InterfaceCounters.InPkts.WithLabelValues(apiName, transceiverName).Set(float64(counters.InPkts.Value))
	}
	if counters.InDrops.OK {
		reg.InterfaceCounters.InDiscards.WithLabelValues(apiName, transceiverName).Set(float64(counters.InDrops.Value))
		st.InDiscards = counters.InDrops.Value
	}
	if counters.InErrors.OK {
		reg.InterfaceCounters.InErrors.WithLabelValues(apiName, transceiverName).Set(float64(counters.InErrors.Value))
		st.InErrors = counters.InErrors.Value
	}
	if counters.InUnicastPkts.OK {
		reg.InterfaceCounters.InUnicastPkts.WithLabelValues(apiName, transceiverName).Set(float64(counters.InUnicastPkts.Value))
	}
	if counters.InMulticastPkts.OK {
		reg.InterfaceCounters.InMulticastPkts.WithLabelValues(apiName, transceiverName).Set(float64(counters.InMulticastPkts.Value))
	}
	if counters.InBroadcastPkts.OK {
		reg.InterfaceCounters.InBroadcastPkts.WithLabelValues(apiName, transceiverName).Set(float64(counters.InBroadcastPkts.Value))
	}

	if counters.OutBytes.OK {
		reg.InterfaceCounters.OutOctets.WithLabelValues(apiName, transceiverName).Set(float64(counters.OutBytes.Value))
		reg.InterfaceCounters.OutBits.WithLabelValues(apiName, transceiverName).Set(float64(counters.OutBytes.Value * 8))
		st.OutBits = counters.OutBytes.Value * 8
	}
	if counters.OutPkts.OK {
		reg.InterfaceCounters.OutPkts.WithLabelValues(apiName, transceiverName).Set(float64(counters.OutPkts.Value))
	}
	if counters.OutDrops.OK {
		reg.InterfaceCounters.OutDiscards.WithLabelValues(apiName, transceiverName).Set(float64(counters.OutDrops.Value))
		st.OutDiscards = counters.OutDrops.Value
	}
	if counters.OutErrors.OK {
		reg.InterfaceCounters.OutErrors.WithLabelValues(apiName, transceiverName).Set(float64(counters.OutErrors.Value))
		st.OutErrors = counters.OutErrors.Value
	}
	if counters.OutUnicastPkts.OK {
		reg.InterfaceCounters.OutUnicastPkts.WithLabelValues(apiName, transceiverName).Set(float64(counters.OutUnicastPkts.Value))
	}
	if counters.OutMulticastPkts.OK {
		reg.InterfaceCounters.OutMulticastPkts.WithLabelValues(apiName, transceiverName).Set(float64(counters.OutMulticastPkts.Value))
	}
	if counters.OutBroadcastPkts.OK {
		reg.InterfaceCounters.OutBroadcastPkts.WithLabelValues(apiName, transceiverName).Set(float64(counters.OutBroadcastPkts.Value))
	}

	if counters.QoS == nil {
		return
	}

	// the egress stats are per traffic class and only cover unicast, hence the queue names
	queues := map[string]agentapi.SwitchStateInterfaceCountersQueue{}
	for trafficClass, stats := range counters.QoS.EgressQueueStats {
		qName := unicastQueuePrefix + trafficClass
		qCounters := agentapi.SwitchStateInterfaceCountersQueue{}
		nonZero := false

		if stats.TxBytes.OK {
			val := stats.TxBytes.Value
			nonZero = nonZero || val != 0
			reg.InterfaceCounters.QueueTransmitOctets.WithLabelValues(apiName, qName, transceiverName).Set(float64(val))
			reg.InterfaceCounters.QueueTransmitBits.WithLabelValues(apiName, qName, transceiverName).Set(float64(val * 8))
			qCounters.TransmitBits = val * 8
		}
		if stats.TxFrames.OK {
			val := stats.TxFrames.Value
			nonZero = nonZero || val != 0
			reg.InterfaceCounters.QueueTransmitPkts.WithLabelValues(apiName, qName, transceiverName).Set(float64(val))
			qCounters.TransmitPkts = val
		}
		if stats.ECNMarkedFrames.OK {
			val := stats.ECNMarkedFrames.Value
			nonZero = nonZero || val != 0
			reg.InterfaceCounters.QueueECNMarkedPkts.WithLabelValues(apiName, qName, transceiverName).Set(float64(val))
			qCounters.ECNMarkedPkts = val
		}
		if stats.TxUCBufferDiscards.OK {
			val := stats.TxUCBufferDiscards.Value
			nonZero = nonZero || val != 0
			reg.InterfaceCounters.QueueDroppedPkts.WithLabelValues(apiName, qName, transceiverName).Set(float64(val))
			qCounters.DroppedPkts = val
		}
		if stats.WREDDiscards.OK {
			val := stats.WREDDiscards.Value
			nonZero = nonZero || val != 0
			reg.InterfaceCounters.QueueWREDDroppedPkts.WithLabelValues(apiName, qName, transceiverName).Set(float64(val))
			qCounters.WREDDroppedPkts = val
		}

		if nonZero {
			queues[qName] = qCounters
		}
	}

	if len(queues) > 0 {
		st.Queues = queues
	}
}

func lldpNeighbors(raw map[string]nvue.LLDPNeighbor) []agentapi.SwitchStateLLDPNeighbor {
	neighbors := make([]agentapi.SwitchStateLLDPNeighbor, 0, len(raw))

	for id, neighbor := range raw {
		st := agentapi.SwitchStateLLDPNeighbor{
			ChassisID:         neighbor.Chassis.ChassisID,
			SystemName:        cmp.Or(neighbor.Chassis.SystemName, id),
			SystemDescription: neighbor.Chassis.SystemDescription,
			PortID:            neighbor.Port.Name,
			PortDescription:   neighbor.Port.Description,
			TTL:               uint16(min(neighbor.Port.TTL.Value, math.MaxUint16)),
			Manufacturer:      neighbor.MED.Inventory.Manufacturer,
			Model:             neighbor.MED.Inventory.Model,
			SerialNumber:      neighbor.MED.Inventory.SerialNumber,
		}

		// NVUE doesn't report the LLDP source MAC, the neighbor port is only known by MAC when it says so itself
		portIDIsMAC := neighbor.Port.Type == lldpPortTypeMAC || isMACAddress(st.PortID)
		switch {
		case portIDIsMAC:
			st.MAC = strings.ToLower(st.PortID)
		case isMACAddress(st.ChassisID):
			st.MAC = strings.ToLower(st.ChassisID)
		}

		st.Port = st.PortID
		if portIDIsMAC && st.PortDescription != "" {
			st.Port = st.PortDescription
		}

		neighbors = append(neighbors, st)
	}

	if len(neighbors) == 0 {
		return nil
	}

	slices.SortFunc(neighbors, compareLLDPNeighbors)

	return neighbors
}

// isMACAddress reports whether s is a 6-octet colon-separated MAC address
func isMACAddress(s string) bool {
	hw, err := net.ParseMAC(s)

	// ParseMAC also takes dash/dot separated, bare hex and longer EUI-64 forms
	return err == nil && len(hw) == 6 && strings.Count(s, ":") == 5
}

// compareLLDPNeighbors orders neighbors by (system name, port, MAC), the remaining fields are only tie breakers to
// keep the order stable as it comes from a map
func compareLLDPNeighbors(a, b agentapi.SwitchStateLLDPNeighbor) int {
	return cmp.Or(
		strings.Compare(a.SystemName, b.SystemName),
		strings.Compare(a.Port, b.Port),
		strings.Compare(a.MAC, b.MAC),
		strings.Compare(a.ChassisID, b.ChassisID),
		strings.Compare(a.PortID, b.PortID),
		strings.Compare(a.PortDescription, b.PortDescription),
		strings.Compare(a.SystemDescription, b.SystemDescription),
		strings.Compare(a.Manufacturer, b.Manufacturer),
		strings.Compare(a.Model, b.Model),
		strings.Compare(a.SerialNumber, b.SerialNumber),
		cmp.Compare(a.TTL, b.TTL),
	)
}

func (c *CumulusProcessor) updateTransceivers(ctx context.Context, reg *switchstate.Registry, swState *agentapi.SwitchState, portMap map[string]string) error {
	// the listing only has the inserted modules, and only their vendor fields
	inserted, err := c.client.Transceivers(ctx)
	if err != nil {
		return fmt.Errorf("getting transceivers: %w", err)
	}

	var mu sync.Mutex
	details := make(map[string]*nvue.Transceiver, len(inserted))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(collectConcurrency)

	for nosName := range inserted {
		group.Go(func() error {
			transceiver, err := c.client.Transceiver(groupCtx, nosName)
			if err != nil {
				// pulled out between the listing and now
				if errors.Is(err, nvue.ErrNotFound) {
					return nil
				}

				return fmt.Errorf("getting transceiver %s: %w", nosName, err)
			}

			mu.Lock()
			details[nosName] = transceiver
			mu.Unlock()

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return err //nolint:wrapcheck // it's our own error from the goroutine
	}

	for nosName, transceiver := range details {
		apiName, ok := portMap[nosName]
		if !ok {
			slog.Warn("Port mapping not found, ignoring for metrics", "transceiver", nosName)

			continue
		}

		name, ok := normBreakoutName(apiName)
		if !ok {
			continue
		}

		st := transceiverState(transceiver)

		if transceiver.Temperature != nil && transceiver.Temperature.Temperature.OK {
			reg.TransceiverMetrics.Temperature.WithLabelValues(name).Set(st.Temperature)
		}
		if transceiver.Voltage != nil && transceiver.Voltage.Voltage.OK {
			reg.TransceiverMetrics.Voltage.WithLabelValues(name).Set(st.Voltage)
		}
		for lane, channel := range st.Channels {
			if channel.In != nil {
				reg.TransceiverMetrics.InPower.WithLabelValues(name, lane).Set(*channel.In)
			}
			if channel.Out != nil {
				reg.TransceiverMetrics.OutPower.WithLabelValues(name, lane).Set(*channel.Out)
			}
			reg.TransceiverMetrics.Bias.WithLabelValues(name, lane).Set(channel.Bias)
		}

		swState.Transceivers[name] = st
	}

	// an empty cage is reported as such, the same way the SONiC agent does
	for nosName, apiName := range portMap {
		if !strings.HasPrefix(nosName, nosPortPrefix) {
			continue
		}

		name, ok := normBreakoutName(apiName)
		if !ok {
			continue
		}

		if _, exists := swState.Transceivers[name]; !exists {
			swState.Transceivers[name] = agentapi.SwitchStateTransceiver{Present: transceiverNotPresent}
		}
	}

	return nil
}

func transceiverState(transceiver *nvue.Transceiver) agentapi.SwitchStateTransceiver {
	present := transceiver.Status == nvue.TransceiverStatusInserted

	st := agentapi.SwitchStateTransceiver{
		Description:  transceiver.PMDType,
		CableClass:   transceiver.CableType,
		FormFactor:   transceiver.Identifier,
		Present:      transceiverNotPresent,
		SerialNumber: transceiver.VendorSN,
		Vendor:       transceiver.VendorName,
		VendorPart:   transceiver.VendorPN,
		VendorOUI:    transceiver.VendorOUI,
		VendorRev:    transceiver.VendorRev,
		CMISRev:      transceiver.RevisionCompliance,
	}

	if present {
		st.Present = transceiverPresent
		// NVUE has no oper status for a module, a fault is the only sign of one not working
		if transceiver.ErrorStatus == "" || transceiver.ErrorStatus == notAvailable {
			st.OperStatus = transceiverOperActive
		}
	}

	if transceiver.FWVersion != notAvailable {
		st.Firmware = transceiver.FWVersion
	}

	if transceiver.CableLength.OK {
		st.CableLength = cleanupFloat(transceiver.CableLength.Value)
	}

	if transceiver.Temperature != nil && transceiver.Temperature.Temperature.OK {
		st.Temperature = cleanupFloat(transceiver.Temperature.Temperature.Value)
	}

	if transceiver.Voltage != nil && transceiver.Voltage.Voltage.OK {
		st.Voltage = cleanupFloat(transceiver.Voltage.Voltage.Value)
	}

	if len(transceiver.Channel) > 0 {
		st.Channels = make(map[string]agentapi.SwitchStateTransceiverChannel, len(transceiver.Channel))
		for id, channel := range transceiver.Channel {
			lane := strings.TrimPrefix(id, "channel-")
			chSt := agentapi.SwitchStateTransceiverChannel{}

			if channel.RxPower != nil && channel.RxPower.Power.OK {
				chSt.In = normPower(channel.RxPower.Power.DBm)
			}
			if channel.TxPower != nil && channel.TxPower.Power.OK {
				chSt.Out = normPower(channel.TxPower.Power.DBm)
			}
			if channel.TxBiasCurrent != nil && channel.TxBiasCurrent.Current.OK {
				chSt.Bias = cleanupFloat(channel.TxBiasCurrent.Current.Value)
			}

			st.Channels[lane] = chSt
		}
	}

	return st
}

func updateTransceiverInfoMetrics(reg *switchstate.Registry, swState *agentapi.SwitchState) {
	// transceivers get swapped, the ones that are gone have to take their series with them
	reg.TransceiverMetrics.Active.Reset()
	reg.TransceiverMetrics.CMISReady.Reset()
	reg.TransceiverMetrics.Info.Reset()

	for name, st := range swState.Transceivers {
		present := st.Present == transceiverPresent
		reg.TransceiverMetrics.Present.WithLabelValues(name).Set(boolToFloat64(present))

		if !present {
			continue
		}

		reg.TransceiverMetrics.Active.WithLabelValues(name).Set(boolToFloat64(st.OperStatus == transceiverOperActive))

		length := ""
		if st.CableLength > 0 {
			length = strconv.FormatFloat(st.CableLength, 'f', -1, 64)
		}

		reg.TransceiverMetrics.Info.WithLabelValues(
			name, st.Description, st.Vendor, st.VendorPart, st.SerialNumber, st.VendorRev, st.VendorOUI,
			st.Firmware, st.FormFactor, st.ConnectorType, st.CableClass, length, st.CMISRev,
		).Set(1)
	}
}

func (c *CumulusProcessor) updateBGPNeighbors(ctx context.Context, reg *switchstate.Registry, swState *agentapi.SwitchState, agent *agentapi.Agent, portMap map[string]string) error {
	vrfs, err := c.client.VRFs(ctx)
	if err != nil {
		return fmt.Errorf("getting vrfs: %w", err)
	}

	now := time.Now()

	for _, vrf := range vrfs {
		if vrf == vrfMgmt {
			continue
		}

		instance, err := c.client.BGPInstance(ctx, vrf)
		if err != nil {
			return fmt.Errorf("getting bgp instance for vrf %s: %w", vrf, err)
		}
		if instance == nil {
			continue
		}

		// the instance only carries a summary of each neighbor, the rest is served one neighbor at a time
		neighbors, err := c.collectBGPNeighbors(ctx, vrf, instance)
		if err != nil {
			return err
		}

		localAS := agent.Spec.Switch.ASN
		if instance.AutonomousSystem.OK {
			localAS = uint32(instance.AutonomousSystem.Value)
		}

		vrfSt := make(map[string]agentapi.SwitchStateBGPNeighbor, len(neighbors))
		// a neighbor only reports its remote AS once the session is up, its peer group knows the type regardless
		groups := map[string]*nvue.BGPPeerGroup{}

		for nosKey, neighbor := range neighbors {
			// an unnumbered neighbor is keyed by the interface it runs over, which the device reports under its NOS name
			key := nosKey
			if neighbor.Type == nvue.BGPNeighborTypeUnnumbered {
				if apiName, ok := portMap[nosKey]; ok {
					key = apiName
				}
			}

			st := bgpNeighborState(neighbor, now, localAS)

			if st.PeerType == agentapi.BGPPeerTypeUnset && st.PeerGroup != "" {
				group, known := groups[st.PeerGroup]
				if !known {
					group, err = c.client.BGPPeerGroup(ctx, vrf, st.PeerGroup)
					if err != nil {
						return fmt.Errorf("getting bgp peer group %s in vrf %s: %w", st.PeerGroup, vrf, err)
					}
					groups[st.PeerGroup] = group
				}

				st.PeerType = bgpPeerTypeFromGroup(group, localAS)
			}

			peerTypeID, err := st.PeerType.ID()
			if err != nil {
				return fmt.Errorf("getting peer type ID: %w", err)
			}
			sessionStateID, err := st.SessionState.ID()
			if err != nil {
				return fmt.Errorf("getting session state ID: %w", err)
			}

			reg.BGPNeighborMetrics.Enabled.WithLabelValues(vrf, key).Set(boolToFloat64(st.Enabled))
			reg.BGPNeighborMetrics.ConnectionsDropped.WithLabelValues(vrf, key).Set(float64(st.ConnectionsDropped))
			reg.BGPNeighborMetrics.EstablishedTransitions.WithLabelValues(vrf, key).Set(float64(st.EstablishedTransitions))
			reg.BGPNeighborMetrics.PeerType.WithLabelValues(vrf, key).Set(float64(peerTypeID))
			reg.BGPNeighborMetrics.SessionState.WithLabelValues(vrf, key).Set(float64(sessionStateID))

			reg.BGPNeighborMetrics.Messages.Received.Open.WithLabelValues(vrf, key).Set(float64(st.Messages.Received.Open))
			reg.BGPNeighborMetrics.Messages.Received.Keepalive.WithLabelValues(vrf, key).Set(float64(st.Messages.Received.Keepalive))
			reg.BGPNeighborMetrics.Messages.Received.RouteRefresh.WithLabelValues(vrf, key).Set(float64(st.Messages.Received.RouteRefresh))
			reg.BGPNeighborMetrics.Messages.Sent.Open.WithLabelValues(vrf, key).Set(float64(st.Messages.Sent.Open))
			reg.BGPNeighborMetrics.Messages.Sent.Keepalive.WithLabelValues(vrf, key).Set(float64(st.Messages.Sent.Keepalive))
			reg.BGPNeighborMetrics.Messages.Sent.RouteRefresh.WithLabelValues(vrf, key).Set(float64(st.Messages.Sent.RouteRefresh))

			for afiSafi, prefixes := range st.Prefixes {
				reg.BGPNeighborMetrics.Prefixes.Received.WithLabelValues(vrf, key, afiSafi).Set(float64(prefixes.Received))
				reg.BGPNeighborMetrics.Prefixes.Sent.WithLabelValues(vrf, key, afiSafi).Set(float64(prefixes.Sent))
			}

			vrfSt[key] = st
		}

		swState.BGPNeighbors[vrf] = vrfSt
	}

	return nil
}

// collectBGPNeighbors fetches the details of every neighbor of the instance concurrently, the ones that are gone
// since the instance was read are skipped
func (c *CumulusProcessor) collectBGPNeighbors(ctx context.Context, vrf string, instance *nvue.BGPInstance) (map[string]*nvue.BGPNeighbor, error) {
	var mu sync.Mutex
	neighbors := make(map[string]*nvue.BGPNeighbor, len(instance.Neighbor))

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(collectConcurrency)

	for id := range instance.Neighbor {
		group.Go(func() error {
			neighbor, err := c.client.BGPNeighbor(ctx, vrf, id)
			if err != nil {
				if errors.Is(err, nvue.ErrNotFound) {
					return nil
				}

				return fmt.Errorf("getting bgp neighbor %s in vrf %s: %w", id, vrf, err)
			}

			mu.Lock()
			neighbors[id] = neighbor
			mu.Unlock()

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err //nolint:wrapcheck // it's our own error from the goroutine
	}

	return neighbors, nil
}

func bgpNeighborState(neighbor *nvue.BGPNeighbor, now time.Time, localAS uint32) agentapi.SwitchStateBGPNeighbor {
	st := agentapi.SwitchStateBGPNeighbor{
		Enabled:                neighbor.State != nvue.StateDisabled,
		ConnectionsDropped:     uint64(max(neighbor.ConnectionsDropped.Value, 0)),
		EstablishedTransitions: uint64(max(neighbor.ConnectionsEstablished.Value, 0)),
		LastResetReason:        neighbor.LastResetReason,
		LocalAS:                localAS,
		PeerPort:               uint16(min(max(neighbor.RemotePort.Value, 0), math.MaxUint16)),
		Prefixes:               map[string]agentapi.SwitchStateBGPNeighborPrefixes{},
	}

	if neighbor.PeerGroup != bgpNone {
		st.PeerGroup = neighbor.PeerGroup
	}

	// the placeholder until the peer has sent its OPEN
	if neighbor.RemoteRouterID != bgpRouterIDUnknown {
		st.RemoteRouterID = neighbor.RemoteRouterID
	}

	if neighbor.LocalAS != nil && neighbor.LocalAS.ASN.OK {
		st.LocalAS = uint32(neighbor.LocalAS.ASN.Value)
	}

	// the remote AS is a number or one of the keywords, which also settle the peer type
	switch {
	case neighbor.RemoteAS.OK:
		st.PeerAS = uint32(neighbor.RemoteAS.Value)
		st.PeerType = agentapi.BGPPeerTypeExternal
		if st.PeerAS == st.LocalAS {
			st.PeerType = agentapi.BGPPeerTypeInternal
		}
	case neighbor.RemoteAS.Raw == nvue.BGPRemoteASInternal:
		st.PeerType = agentapi.BGPPeerTypeInternal
		st.PeerAS = st.LocalAS
	case neighbor.RemoteAS.Raw == nvue.BGPRemoteASExternal:
		st.PeerType = agentapi.BGPPeerTypeExternal
	}

	switch neighbor.SessionState {
	case "":
		st.SessionState = agentapi.BGPNeighborSessionStateUnset
	case string(agentapi.BGPNeighborSessionStateIdle), bgpSessionClearing, bgpSessionDeleted:
		st.SessionState = agentapi.BGPNeighborSessionStateIdle
	case string(agentapi.BGPNeighborSessionStateConnect):
		st.SessionState = agentapi.BGPNeighborSessionStateConnect
	case string(agentapi.BGPNeighborSessionStateActive):
		st.SessionState = agentapi.BGPNeighborSessionStateActive
	case "opensent":
		st.SessionState = agentapi.BGPNeighborSessionStateOpenSent
	case "openconfirm":
		st.SessionState = agentapi.BGPNeighborSessionStateOpenConfirm
	case string(agentapi.BGPNeighborSessionStateEstablished):
		st.SessionState = agentapi.BGPNeighborSessionStateEstablished
	default:
		slog.Warn("Unknown bgp session state, ignoring", "sessionState", neighbor.SessionState)
	}

	// the timers are reported as milliseconds elapsed, not as timestamps
	if neighbor.Uptime.OK && neighbor.Uptime.Value > 0 && st.SessionState == agentapi.BGPNeighborSessionStateEstablished {
		st.LastEstablished = kmetav1.Time{Time: now.Add(-millis(neighbor.Uptime.Value))}
	}
	if neighbor.LastResetTime.OK && neighbor.LastResetTime.Value > 0 {
		st.LastResetTime = kmetav1.Time{Time: now.Add(-millis(neighbor.LastResetTime.Value))}
	}

	if msgs := neighbor.Messages; msgs != nil {
		st.Messages = agentapi.BGPMessages{
			Received: agentapi.BGPMessagesCounters{
				Open:         uint64(max(msgs.RxOpens.Value, 0)),
				Keepalive:    uint64(max(msgs.RxKeepalives.Value, 0)),
				RouteRefresh: uint64(max(msgs.RxRouteRefreshes.Value, 0)),
			},
			Sent: agentapi.BGPMessagesCounters{
				Open:         uint64(max(msgs.TxOpens.Value, 0)),
				Keepalive:    uint64(max(msgs.TxKeepalives.Value, 0)),
				RouteRefresh: uint64(max(msgs.TxRouteRefreshes.Value, 0)),
			},
		}
	}

	for af, afSt := range neighbor.AF {
		if !afSt.TxPrefix.OK && !afSt.RxPrefix.OK {
			continue
		}

		st.Prefixes[afiSafiName(af)] = agentapi.SwitchStateBGPNeighborPrefixes{
			Received: uint32(max(afSt.RxPrefix.Value, 0)),
			Sent:     uint32(max(afSt.TxPrefix.Value, 0)),
		}
	}

	return st
}

// bgpPeerTypeFromGroup derives the peer type from the group configuration, which says it outright or by remote AS
func bgpPeerTypeFromGroup(group *nvue.BGPPeerGroup, localAS uint32) agentapi.BGPPeerType {
	if group == nil {
		return agentapi.BGPPeerTypeUnset
	}

	switch {
	case group.Type == nvue.BGPRemoteASInternal, group.RemoteAS.Raw == nvue.BGPRemoteASInternal:
		return agentapi.BGPPeerTypeInternal
	case group.Type == nvue.BGPRemoteASExternal, group.RemoteAS.Raw == nvue.BGPRemoteASExternal:
		return agentapi.BGPPeerTypeExternal
	case group.RemoteAS.OK && uint32(group.RemoteAS.Value) == localAS:
		return agentapi.BGPPeerTypeInternal
	case group.RemoteAS.OK:
		return agentapi.BGPPeerTypeExternal
	default:
		return agentapi.BGPPeerTypeUnset
	}
}

// afiSafiName turns "ipv4-unicast" into "IPV4_UNICAST", the OpenConfig spelling the SONiC agent reports
func afiSafiName(af string) string {
	return strings.ToUpper(strings.ReplaceAll(af, "-", "_"))
}

func millis(ms float64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

func (c *CumulusProcessor) updatePlatform(ctx context.Context, reg *switchstate.Registry, swState *agentapi.SwitchState) error {
	fans, err := c.client.Fans(ctx)
	if err != nil {
		return fmt.Errorf("getting fans: %w", err)
	}

	for name, fan := range fans {
		st := agentapi.SwitchStatePlatformFan{
			Presence: fan.State != nvue.EnvironmentStateAbsent,
			Status:   fan.State == nvue.EnvironmentStateOK,
		}

		if fan.Direction != notAvailable {
			st.Direction = fan.Direction
		}

		if fan.CurrentSpeed.OK {
			reg.PlatformMetrics.Fan.Speed.WithLabelValues(name).Set(fan.CurrentSpeed.Value)
			st.Speed = cleanupFloat(fan.CurrentSpeed.Value)
		}

		reg.PlatformMetrics.Fan.Presence.WithLabelValues(name).Set(boolToFloat64(st.Presence))
		reg.PlatformMetrics.Fan.Status.WithLabelValues(name).Set(boolToFloat64(st.Status))

		swState.Platform.Fans[name] = st
	}

	psus, err := c.client.PSUs(ctx)
	if err != nil {
		return fmt.Errorf("getting psus: %w", err)
	}

	for name, psu := range psus {
		// a PSU without power reads "bad" and no values, which is all that tells it apart from a missing one
		st := agentapi.SwitchStatePlatformPSU{
			Presence: psu.State != nvue.EnvironmentStateAbsent,
			Status:   psu.State == nvue.EnvironmentStateOK,
		}

		reg.PlatformMetrics.PSU.Presence.WithLabelValues(name).Set(boolToFloat64(st.Presence))
		reg.PlatformMetrics.PSU.Status.WithLabelValues(name).Set(boolToFloat64(st.Status))

		// NVUE only reports the DC output side
		if psu.Current.OK {
			reg.PlatformMetrics.PSU.OutputCurrent.WithLabelValues(name).Set(psu.Current.Value)
			st.OutputCurrent = cleanupFloat(psu.Current.Value)
		}
		if psu.Power.OK {
			reg.PlatformMetrics.PSU.OutputPower.WithLabelValues(name).Set(psu.Power.Value)
			st.OutputPower = cleanupFloat(psu.Power.Value)
		}
		if psu.Voltage.OK {
			reg.PlatformMetrics.PSU.OutputVoltage.WithLabelValues(name).Set(psu.Voltage.Value)
			st.OutputVoltage = cleanupFloat(psu.Voltage.Value)
		}

		swState.Platform.PSUs[name] = st
	}

	temperatures, err := c.client.Temperatures(ctx)
	if err != nil {
		return fmt.Errorf("getting temperatures: %w", err)
	}

	for name, temperature := range temperatures {
		st := agentapi.SwitchStatePlatformTemperature{}

		if temperature.State != nvue.EnvironmentStateOK {
			st.Alarms = temperature.State
		}

		if temperature.Current.OK {
			reg.PlatformMetrics.Temperature.Temperature.WithLabelValues(name).Set(temperature.Current.Value)
			st.Temperature = cleanupFloat(temperature.Current.Value)
		}
		if temperature.Max.OK {
			reg.PlatformMetrics.Temperature.HighThreshold.WithLabelValues(name).Set(temperature.Max.Value)
			st.HighThreshold = cleanupFloat(temperature.Max.Value)
		}
		if temperature.Crit.OK {
			reg.PlatformMetrics.Temperature.CriticalHighThreshold.WithLabelValues(name).Set(temperature.Crit.Value)
			st.CriticalHighThreshold = cleanupFloat(temperature.Crit.Value)
		}
		if temperature.Min.OK {
			reg.PlatformMetrics.Temperature.LowThreshold.WithLabelValues(name).Set(temperature.Min.Value)
			st.LowThreshold = cleanupFloat(temperature.Min.Value)
		}

		swState.Platform.Temperatures[name] = st
	}

	return nil
}

func (c *CumulusProcessor) updateNOS(ctx context.Context, swState *agentapi.SwitchState, system *nvue.System) error {
	platform, err := c.client.Platform(ctx)
	if err != nil {
		return fmt.Errorf("getting platform: %w", err)
	}

	firmware, err := c.client.Firmware(ctx)
	if err != nil {
		return fmt.Errorf("getting firmware: %w", err)
	}

	swState.NOS = agentapi.SwitchStateNOS{
		AsicVersion:         platform.ASICModel,
		BuildDate:           system.Version.Image.BuildDate,
		DistributionVersion: system.Version.BaseOS,
		HwskuVersion:        platform.SystemType,
		KernelVersion:       system.Version.Kernel,
		MfgName:             platform.Manufacturer,
		PlatformName:        system.Platform,
		ProductDescription:  system.ProductName,
		ProductVersion:      system.Version.Image.BuildID,
		SerialNumber:        platform.SerialNumber,
		SoftwareVersion:     cmp.Or(system.Version.ProductRelease, system.ProductRelease),
	}

	if system.Uptime.OK {
		swState.NOS.Uptime = formatUptime(system.Uptime.Value)
	}

	for name, component := range firmware {
		if component.ActualFirmware == "" || component.ActualFirmware == notAvailable {
			continue
		}

		swState.Firmware[name] = component.ActualFirmware
	}

	return nil
}

// formatUptime renders seconds the way uptime(1) does, "up 16 days, 6:15"
func formatUptime(seconds float64) string {
	total := int64(seconds)
	days := total / secondsPerDay
	hours := total % secondsPerDay / secondsPerHour
	minutes := total % secondsPerHour / secondsPerMinute

	if days == 0 {
		return fmt.Sprintf("up %d:%02d", hours, minutes)
	}

	return fmt.Sprintf("up %d days, %d:%02d", days, hours, minutes)
}

func boolToFloat64(b bool) float64 {
	if b {
		return 1
	}

	return 0
}

func cleanupFloat(val float64) float64 {
	if math.IsInf(val, 0) || math.IsNaN(val) {
		return 0
	}

	return val
}

func normPower(power float64) *float64 {
	if math.IsInf(power, 0) || math.IsNaN(power) {
		return nil
	}

	return &power
}

// transceiverForPort returns the transceiver cage an interface lives in, e.g. E1/1 for both E1/1/1 and E1/1/2, and
// nothing for the interfaces that have no cage at all, such as the management port and the bonds
func transceiverForPort(port string) string {
	switch strings.Count(port, "/") {
	case 1:
		return port
	case 2:
		return port[:strings.LastIndex(port, "/")]
	default:
		return ""
	}
}

// normBreakoutName maps the first breakout of a port to its cage, E1/1/1 to E1/1, and drops the other breakouts
// which share it
func normBreakoutName(transceiverName string) (string, bool) {
	if strings.Count(transceiverName, "/") == 2 {
		return strings.CutSuffix(transceiverName, "/1")
	}

	return transceiverName, true
}
