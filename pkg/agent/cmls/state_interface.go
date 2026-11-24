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
	"slices"
	"strings"
	"sync"
	"time"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"golang.org/x/sync/errgroup"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// the hardware counters are read one port at a time
	countersConcurrency = 4
	// unicastQueuePrefix names the egress queues by their traffic class like the Broadcom agent does
	unicastQueuePrefix = "UC"
	maxUtilizationPct  = 100
)

// unmanagedInterfaces are the interfaces of a switch its profile doesn't have, so they aren't reported, by switch profile
var unmanagedInterfaces = map[string][]string{
	// a second management port
	switchprofile.CumulusNvidiaSN2700.Name: {"eth1"},
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

func (c *CumulusProcessor) updateInterfaces(ctx context.Context, reg *switchstate.Registry, swState *agentapi.SwitchState, profile string, portMap map[string]string, loc *time.Location, vxLegacy bool) error {
	ifaces, err := c.nvue.Interfaces(ctx)
	if err != nil {
		return fmt.Errorf("getting interfaces: %w", err)
	}

	rates, err := c.nvue.InterfaceRates(ctx)
	// Cumulus VX doesn't collect the traffic rates, its nvued says the service collecting them isn't active
	if err != nil && (!vxLegacy || !errors.Is(err, nvue.ErrNotFound)) {
		return fmt.Errorf("getting interface rates: %w", err)
	}

	// the hardware counters of the ports up, keyed by their NOS names
	withCounters := map[string]string{}

	for nosName, iface := range ifaces {
		name, ok := reportedInterface(nosName, &iface, profile, portMap)
		if !ok {
			continue
		}

		transceiverName := transceiverForPort(name)
		st, err := interfaceState(nosName, iface.Link, loc)
		if err != nil {
			return err
		}

		adminStatusID, err := st.AdminStatus.ID()
		if err != nil {
			return fmt.Errorf("getting admin status ID of %s: %w", nosName, err)
		}
		operStatusID, err := st.OperStatus.ID()
		if err != nil {
			return fmt.Errorf("getting oper status ID of %s: %w", nosName, err)
		}

		reg.InterfaceMetrics.Enabled.WithLabelValues(name, transceiverName).Set(boolToFloat64(st.Enabled))
		reg.InterfaceMetrics.AdminStatus.WithLabelValues(name, transceiverName).Set(float64(adminStatusID))
		reg.InterfaceMetrics.OperStatus.WithLabelValues(name, transceiverName).Set(float64(operStatusID))
		if !st.LastChange.IsZero() {
			reg.InterfaceMetrics.LastChange.WithLabelValues(name, transceiverName).Set(float64(st.LastChange.Unix()))
		}

		if ifRates, ok := rates[nosName]; ok && st.Enabled {
			st.Counters = ratesCounters(reg, name, transceiverName, &ifRates)
		}

		// every switch and IPMI would show up on the management port
		if iface.Type != nvue.InterfaceTypeEth && iface.LLDP != nil {
			st.LLDPNeighbors = lldpNeighbors(iface.LLDP.Neighbor)
		}

		swState.Interfaces[name] = st

		if st.Enabled && iface.Type == nvue.InterfaceTypeSwp {
			withCounters[nosName] = name
		}
	}

	counters, err := c.interfaceCounters(ctx, withCounters)
	if err != nil {
		return err
	}
	for name, ifCounters := range counters {
		st := swState.Interfaces[name]
		applyCounters(reg, &st, name, transceiverForPort(name), ifCounters)
		swState.Interfaces[name] = st
	}

	updateLLDPMetrics(reg, swState, portMap)

	return nil
}

// reportedInterface returns the API name of an interface to report: the ports, the management port and the bonds, which
// keep their NOS names
func reportedInterface(nosName string, iface *nvue.Interface, profile string, portMap map[string]string) (string, bool) {
	if iface.Type != nvue.InterfaceTypeSwp && iface.Type != nvue.InterfaceTypeEth && iface.Type != nvue.InterfaceTypeBond {
		return "", false
	}
	if iface.Link == nil {
		slog.Warn("Interface without link state, ignoring", "interface", nosName)

		return "", false
	}
	if slices.Contains(unmanagedInterfaces[profile], nosName) {
		return "", false
	}
	if iface.Type == nvue.InterfaceTypeBond {
		return nosName, true
	}

	apiName, exists := portMap[nosName]
	if !exists {
		slog.Warn("Port mapping not found, ignoring for metrics", "interface", nosName)

		return "", false
	}

	return apiName, true
}

func interfaceState(nosName string, link *nvue.InterfaceLink, loc *time.Location) (agentapi.SwitchStateInterface, error) {
	st := agentapi.SwitchStateInterface{
		MAC:           link.MACAddress,
		Speed:         link.Speed,
		AutoNegotiate: link.AutoNegotiate == nvue.StateEnabled,
		FEC:           link.FEC,
		// protodown is how the protocols such as the link flap protection take a port down
		ErrDisabled: link.Protodown != "" && link.Protodown != nvue.StateDisabled,
	}

	switch link.AdminStatus {
	case nvue.StatusUp:
		st.AdminStatus = agentapi.AdminStatusUp
	case nvue.StatusDown:
		st.AdminStatus = agentapi.AdminStatusDown
	default:
		return st, fmt.Errorf("interface %s: unknown admin status %q", nosName, link.AdminStatus) //nolint:err113
	}
	st.Enabled = st.AdminStatus == agentapi.AdminStatusUp

	switch link.OperStatus {
	case nvue.StatusUp:
		st.OperStatus = agentapi.OperStatusUp
	case nvue.StatusDown:
		st.OperStatus = agentapi.OperStatusDown
	case string(agentapi.OperStatusDormant):
		st.OperStatus = agentapi.OperStatusDormant
	case string(agentapi.OperStatusTesting):
		st.OperStatus = agentapi.OperStatusTesting
	default:
		slog.Warn("Unknown oper status", "interface", nosName, "operStatus", link.OperStatus)
		st.OperStatus = agentapi.OperStatusUnknown
	}

	if link.OperStatusLastChange != "" {
		lastChange, err := link.LastChange(loc)
		if err != nil {
			return st, fmt.Errorf("interface %s: %w", nosName, err)
		}
		st.LastChange = kmetav1.Time{Time: lastChange}
	}

	return st, nil
}

func ratesCounters(reg *switchstate.Registry, name, transceiverName string, rates *nvue.InterfaceRates) *agentapi.SwitchStateInterfaceCounters {
	counters := &agentapi.SwitchStateInterfaceCounters{
		InBitsPerSecond:  cleanupFloat(rates.InBitsRate.Value),
		InPktsPerSecond:  cleanupFloat(rates.InPktsRate.Value),
		InUtilization:    utilizationPercent(rates.InUtilization),
		OutBitsPerSecond: cleanupFloat(rates.OutBitsRate.Value),
		OutPktsPerSecond: cleanupFloat(rates.OutPktsRate.Value),
		OutUtilization:   utilizationPercent(rates.OutUtilization),
	}

	if rates.LoadInterval.OK {
		reg.InterfaceMetrics.RateInterval.WithLabelValues(name, transceiverName).Set(rates.LoadInterval.Value)
	}

	reg.InterfaceCounters.InBitsPerSecond.WithLabelValues(name, transceiverName).Set(counters.InBitsPerSecond)
	reg.InterfaceCounters.InOctetsPerSecond.WithLabelValues(name, transceiverName).Set(counters.InBitsPerSecond / 8)
	reg.InterfaceCounters.InPktsPerSecond.WithLabelValues(name, transceiverName).Set(counters.InPktsPerSecond)
	reg.InterfaceCounters.InUtilization.WithLabelValues(name, transceiverName).Set(float64(counters.InUtilization))
	reg.InterfaceCounters.OutBitsPerSecond.WithLabelValues(name, transceiverName).Set(counters.OutBitsPerSecond)
	reg.InterfaceCounters.OutOctetsPerSecond.WithLabelValues(name, transceiverName).Set(counters.OutBitsPerSecond / 8)
	reg.InterfaceCounters.OutPktsPerSecond.WithLabelValues(name, transceiverName).Set(counters.OutPktsPerSecond)
	reg.InterfaceCounters.OutUtilization.WithLabelValues(name, transceiverName).Set(float64(counters.OutUtilization))

	return counters
}

// utilizationPercent rounds the percentage of the link speed to the whole percent the state has
func utilizationPercent(n nvue.Number) uint8 {
	if !n.OK || n.Value < 0 {
		return 0
	}

	return uint8(math.Round(min(n.Value, maxUtilizationPct)))
}

// interfaceCounters reads the hardware counters of the ports, NOS name to API name, a few at a time, they're only
// served one port at a time. A port gone since the listing has none.
func (c *CumulusProcessor) interfaceCounters(ctx context.Context, ports map[string]string) (map[string]*nvue.InterfaceCounters, error) {
	lock := sync.Mutex{}
	counters := make(map[string]*nvue.InterfaceCounters, len(ports))

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(countersConcurrency)

	for nosName, name := range ports {
		group.Go(func() error {
			ifCounters, err := c.nvue.InterfaceCounters(ctx, nosName)
			if errors.Is(err, nvue.ErrNotFound) {
				slog.Warn("Port without counters", "interface", nosName)

				return nil
			}
			if err != nil {
				return fmt.Errorf("getting counters of %s: %w", nosName, err)
			}

			lock.Lock()
			counters[name] = ifCounters
			lock.Unlock()

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err //nolint:wrapcheck // the error of a request above
	}

	return counters, nil
}

func applyCounters(reg *switchstate.Registry, st *agentapi.SwitchStateInterface, name, transceiverName string, counters *nvue.InterfaceCounters) {
	if st.Counters == nil {
		st.Counters = &agentapi.SwitchStateInterfaceCounters{}
	}
	cnt := st.Counters

	if counters.InBytes.OK {
		reg.InterfaceCounters.InOctets.WithLabelValues(name, transceiverName).Set(float64(counters.InBytes.Value))
		reg.InterfaceCounters.InBits.WithLabelValues(name, transceiverName).Set(float64(counters.InBytes.Value * 8))
		cnt.InBits = counters.InBytes.Value * 8
	}
	if counters.InPkts.OK {
		reg.InterfaceCounters.InPkts.WithLabelValues(name, transceiverName).Set(float64(counters.InPkts.Value))
	}
	if counters.InUnicastPkts.OK {
		reg.InterfaceCounters.InUnicastPkts.WithLabelValues(name, transceiverName).Set(float64(counters.InUnicastPkts.Value))
	}
	if counters.InMulticastPkts.OK {
		reg.InterfaceCounters.InMulticastPkts.WithLabelValues(name, transceiverName).Set(float64(counters.InMulticastPkts.Value))
	}
	if counters.InBroadcastPkts.OK {
		reg.InterfaceCounters.InBroadcastPkts.WithLabelValues(name, transceiverName).Set(float64(counters.InBroadcastPkts.Value))
	}
	if counters.InDrops.OK {
		reg.InterfaceCounters.InDiscards.WithLabelValues(name, transceiverName).Set(float64(counters.InDrops.Value))
		cnt.InDiscards = counters.InDrops.Value
	}
	if counters.InErrors.OK {
		reg.InterfaceCounters.InErrors.WithLabelValues(name, transceiverName).Set(float64(counters.InErrors.Value))
		cnt.InErrors = counters.InErrors.Value
	}

	if counters.OutBytes.OK {
		reg.InterfaceCounters.OutOctets.WithLabelValues(name, transceiverName).Set(float64(counters.OutBytes.Value))
		reg.InterfaceCounters.OutBits.WithLabelValues(name, transceiverName).Set(float64(counters.OutBytes.Value * 8))
		cnt.OutBits = counters.OutBytes.Value * 8
	}
	if counters.OutPkts.OK {
		reg.InterfaceCounters.OutPkts.WithLabelValues(name, transceiverName).Set(float64(counters.OutPkts.Value))
	}
	if counters.OutUnicastPkts.OK {
		reg.InterfaceCounters.OutUnicastPkts.WithLabelValues(name, transceiverName).Set(float64(counters.OutUnicastPkts.Value))
	}
	if counters.OutMulticastPkts.OK {
		reg.InterfaceCounters.OutMulticastPkts.WithLabelValues(name, transceiverName).Set(float64(counters.OutMulticastPkts.Value))
	}
	if counters.OutBroadcastPkts.OK {
		reg.InterfaceCounters.OutBroadcastPkts.WithLabelValues(name, transceiverName).Set(float64(counters.OutBroadcastPkts.Value))
	}
	if counters.OutDrops.OK {
		reg.InterfaceCounters.OutDiscards.WithLabelValues(name, transceiverName).Set(float64(counters.OutDrops.Value))
		cnt.OutDiscards = counters.OutDrops.Value
	}
	if counters.OutErrors.OK {
		reg.InterfaceCounters.OutErrors.WithLabelValues(name, transceiverName).Set(float64(counters.OutErrors.Value))
		cnt.OutErrors = counters.OutErrors.Value
	}

	if counters.QoS == nil {
		return
	}

	queues := map[string]agentapi.SwitchStateInterfaceCountersQueue{}
	for tc, stats := range counters.QoS.EgressQueueStats {
		queue := unicastQueuePrefix + tc
		qCounters := agentapi.SwitchStateInterfaceCountersQueue{
			TransmitBits:    stats.TxBytes.Value * 8,
			TransmitPkts:    stats.TxFrames.Value,
			DroppedPkts:     stats.TxUCBufferDiscards.Value,
			WREDDroppedPkts: stats.WREDDiscards.Value,
			ECNMarkedPkts:   stats.ECNMarkedFrames.Value,
		}

		reg.InterfaceCounters.QueueTransmitOctets.WithLabelValues(name, queue, transceiverName).Set(float64(stats.TxBytes.Value))
		reg.InterfaceCounters.QueueTransmitBits.WithLabelValues(name, queue, transceiverName).Set(float64(qCounters.TransmitBits))
		reg.InterfaceCounters.QueueTransmitPkts.WithLabelValues(name, queue, transceiverName).Set(float64(qCounters.TransmitPkts))
		reg.InterfaceCounters.QueueDroppedPkts.WithLabelValues(name, queue, transceiverName).Set(float64(qCounters.DroppedPkts))
		reg.InterfaceCounters.QueueWREDDroppedPkts.WithLabelValues(name, queue, transceiverName).Set(float64(qCounters.WREDDroppedPkts))
		reg.InterfaceCounters.QueueECNMarkedPkts.WithLabelValues(name, queue, transceiverName).Set(float64(qCounters.ECNMarkedPkts))

		// the state only has the queues in use, like the Broadcom agent reports them
		if qCounters != (agentapi.SwitchStateInterfaceCountersQueue{}) {
			queues[queue] = qCounters
		}
	}
	if len(queues) > 0 {
		cnt.Queues = queues
	}
}

const unknownLLDPChassisID = "00:00:00:00:00:00"

func lldpNeighbors(raw map[string]nvue.LLDPNeighbor) []agentapi.SwitchStateLLDPNeighbor {
	if len(raw) == 0 {
		return nil
	}

	neighbors := make([]agentapi.SwitchStateLLDPNeighbor, 0, len(raw))
	for _, neighbor := range raw {
		st := agentapi.SwitchStateLLDPNeighbor{
			ChassisID:         neighbor.Chassis.ChassisID,
			SystemName:        neighbor.Chassis.SystemName,
			SystemDescription: neighbor.Chassis.SystemDescription,
			PortID:            neighbor.Port.Name,
			PortDescription:   neighbor.Port.Description,
			Manufacturer:      neighbor.MED.Inventory.Manufacturer,
			Model:             neighbor.MED.Inventory.Model,
			SerialNumber:      neighbor.MED.Inventory.SerialNumber,
		}
		if neighbor.Port.TTL.OK {
			st.TTL = uint16(min(max(neighbor.Port.TTL.Value, 0), math.MaxUint16))
		}

		// a chassis id that isn't a MAC, such as the machine id hosts send, is reported as all zeroes
		if st.ChassisID == unknownLLDPChassisID {
			st.ChassisID = ""
		}

		// the port id of a host is often its MAC, the description is more telling then
		portIDIsMAC := isMACAddress(st.PortID)
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

	slices.SortFunc(neighbors, compareLLDPNeighbors)

	return neighbors
}

// isMACAddress reports whether s is a 6-octet colon-separated MAC address
func isMACAddress(s string) bool {
	hw, err := net.ParseMAC(s)

	return err == nil && len(hw) == 6 && strings.Count(s, ":") == 5
}

// compareLLDPNeighbors orders neighbors by (system name, port, MAC), the remaining fields are only tie breakers to
// keep the order stable as it comes from a map and the sort isn't stable either
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

func updateLLDPMetrics(reg *switchstate.Registry, swState *agentapi.SwitchState, portMap map[string]string) {
	// neighbors are labeled by their own identity, the ones that are gone have to take their series with them
	reg.LLDPMetrics.LastUpdate.Reset()
	reg.LLDPMetrics.TTL.Reset()
	reg.LLDPMetrics.Info.Reset()

	// iterating the ports and not the neighbors: a port without any has to report zero, nothing ever resets it
	for nosName, name := range portMap {
		if !strings.HasPrefix(nosName, nvue.InterfaceTypeSwp) {
			continue
		}

		transceiverName := transceiverForPort(name)
		neighbors := swState.Interfaces[name].LLDPNeighbors
		reg.LLDPMetrics.Neighbors.WithLabelValues(name, transceiverName).Set(float64(len(neighbors)))

		for _, neighbor := range neighbors {
			reg.LLDPMetrics.Info.WithLabelValues(
				name, neighbor.SystemName, neighbor.Port, neighbor.MAC, neighbor.ChassisID,
				neighbor.SystemDescription, neighbor.Manufacturer, neighbor.Model, neighbor.SerialNumber,
				transceiverName,
			).Set(1)

			if neighbor.TTL > 0 {
				reg.LLDPMetrics.TTL.
					WithLabelValues(name, neighbor.SystemName, neighbor.Port, neighbor.MAC, transceiverName).
					Set(float64(neighbor.TTL))
			}
		}
	}
}
