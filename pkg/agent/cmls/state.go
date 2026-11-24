// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"strings"
	"sync"
	"time"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
)

const (
	notAvailable = "N/A"

	// vxLegacyRelease is the Cumulus Linux release of the Cumulus VX the CI runs, see isVXLegacy
	vxLegacyRelease = "5.16."
)

// isVXLegacy tells the Cumulus VX on 5.16 the CI runs, the only switches where the state falls back to what 5.16 and VX
// report differently: no traffic rates, BGP neighbors without their type, the BFD multihop as a JSON boolean and
// transceiver placeholders without a status. On anything else these are errors or warnings.
func isVXLegacy(profile *wiringapi.SwitchProfileSpec, system *nvue.System) bool {
	return profile.NOSType == meta.NOSTypeCumulusVX && strings.HasPrefix(system.Version.ProductRelease, vxLegacyRelease)
}

// staticState is the part of the switch state that doesn't change and takes seconds to read, so it's read once
type staticState struct {
	lock     sync.Mutex
	platform *nvue.Platform
	firmware map[string]string
}

func (c *CumulusProcessor) UpdateSwitchState(ctx context.Context, agent *agentapi.Agent, reg *switchstate.Registry) error {
	if agent.Spec.SwitchProfile == nil {
		return errors.New("switch profile not found") //nolint:err113
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

	roce, err := c.GetRoCE(ctx)
	if err != nil {
		return err
	}
	swState.RoCE = roce

	system, err := c.nvue.System(ctx)
	if err != nil {
		return fmt.Errorf("getting system: %w", err)
	}

	// the switch reports its times in its own timezone
	loc, err := system.Location()
	if err != nil {
		return err //nolint:wrapcheck // it says what it is
	}

	vxLegacy := isVXLegacy(agent.Spec.SwitchProfile, system)

	if err := c.updateNOS(ctx, swState, system); err != nil {
		return fmt.Errorf("updating nos: %w", err)
	}

	if err := c.updateInterfaces(ctx, reg, swState, agent.Spec.Switch.Profile, portMap, loc, vxLegacy); err != nil {
		return fmt.Errorf("updating interfaces: %w", err)
	}

	cages := cagesOf(agent.Spec.SwitchProfile)

	if err := c.updateTransceivers(ctx, reg, swState, cages, vxLegacy); err != nil {
		return fmt.Errorf("updating transceivers: %w", err)
	}

	if err := c.updateBreakouts(ctx, swState, cages); err != nil {
		return fmt.Errorf("updating breakouts: %w", err)
	}

	vrfs, err := c.routedVRFs(ctx)
	if err != nil {
		return err
	}

	if err := c.updateBGPNeighbors(ctx, reg, swState, vrfs, portMap, vxLegacy); err != nil {
		return fmt.Errorf("updating bgp neighbors: %w", err)
	}

	if err := c.updateBFDPeers(ctx, reg, swState, vrfs, portMap, vxLegacy); err != nil {
		return fmt.Errorf("updating bfd peers: %w", err)
	}

	if err := c.updatePlatform(ctx, reg, swState); err != nil {
		return fmt.Errorf("updating platform: %w", err)
	}

	reg.SaveSwitchState(swState)

	slog.Debug("Switch state updated", "took", time.Since(start))

	return nil
}

// static reads the static part of the switch state the first time, a failed read is retried the next time
func (c *CumulusProcessor) static(ctx context.Context) (*nvue.Platform, map[string]string, error) {
	c.state.lock.Lock()
	defer c.state.lock.Unlock()

	if c.state.platform == nil {
		platform, err := c.nvue.Platform(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("getting platform: %w", err)
		}
		c.state.platform = platform
	}

	if c.state.firmware == nil {
		components, err := c.nvue.Firmware(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("getting firmware: %w", err)
		}

		firmware := make(map[string]string, len(components))
		for name, component := range components {
			firmware[name] = component.ActualFirmware
		}
		c.state.firmware = firmware
	}

	return c.state.platform, c.state.firmware, nil
}

func (c *CumulusProcessor) updateNOS(ctx context.Context, swState *agentapi.SwitchState, system *nvue.System) error {
	platform, firmware, err := c.static(ctx)
	if err != nil {
		return err
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
		SoftwareVersion:     system.Version.ProductRelease,
	}
	if system.Uptime.OK {
		swState.NOS.Uptime = (time.Duration(system.Uptime.Value) * time.Second).String()
	}

	maps.Copy(swState.Firmware, firmware)

	return nil
}

func (c *CumulusProcessor) updatePlatform(ctx context.Context, reg *switchstate.Registry, swState *agentapi.SwitchState) error {
	fans, err := c.nvue.Fans(ctx)
	if err != nil {
		return fmt.Errorf("getting fans: %w", err)
	}

	for name, fan := range fans {
		st := agentapi.SwitchStatePlatformFan{
			Presence: fan.State != "absent",
			Status:   fan.State == nvue.EnvironmentStateOK,
		}
		if fan.Direction != notAvailable {
			st.Direction = fan.Direction
		}
		if fan.CurrentSpeed.OK {
			st.Speed = cleanupFloat(fan.CurrentSpeed.Value)
			reg.PlatformMetrics.Fan.Speed.WithLabelValues(name).Set(st.Speed)
		}
		reg.PlatformMetrics.Fan.Presence.WithLabelValues(name).Set(boolToFloat64(st.Presence))
		reg.PlatformMetrics.Fan.Status.WithLabelValues(name).Set(boolToFloat64(st.Status))

		swState.Platform.Fans[name] = st
	}

	psus, err := c.nvue.PSUs(ctx)
	if err != nil {
		return fmt.Errorf("getting psus: %w", err)
	}

	for name, psu := range psus {
		st := agentapi.SwitchStatePlatformPSU{
			Presence: psu.State != "absent",
			Status:   psu.State == nvue.EnvironmentStateOK,
		}
		// the readings are on the output side
		if psu.Power.OK {
			st.OutputPower = cleanupFloat(psu.Power.Value)
			reg.PlatformMetrics.PSU.OutputPower.WithLabelValues(name).Set(st.OutputPower)
		}
		if psu.Current.OK {
			st.OutputCurrent = cleanupFloat(psu.Current.Value)
			reg.PlatformMetrics.PSU.OutputCurrent.WithLabelValues(name).Set(st.OutputCurrent)
		}
		if psu.Voltage.OK {
			st.OutputVoltage = cleanupFloat(psu.Voltage.Value)
			reg.PlatformMetrics.PSU.OutputVoltage.WithLabelValues(name).Set(st.OutputVoltage)
		}
		reg.PlatformMetrics.PSU.Presence.WithLabelValues(name).Set(boolToFloat64(st.Presence))
		reg.PlatformMetrics.PSU.Status.WithLabelValues(name).Set(boolToFloat64(st.Status))

		swState.Platform.PSUs[name] = st
	}

	temperatures, err := c.nvue.Temperatures(ctx)
	if err != nil {
		return fmt.Errorf("getting temperatures: %w", err)
	}

	for name, temp := range temperatures {
		st := agentapi.SwitchStatePlatformTemperature{}
		if temp.Current.OK {
			st.Temperature = cleanupFloat(temp.Current.Value)
			reg.PlatformMetrics.Temperature.Temperature.WithLabelValues(name).Set(st.Temperature)
		}
		if temp.Max.OK {
			st.HighThreshold = cleanupFloat(temp.Max.Value)
			reg.PlatformMetrics.Temperature.HighThreshold.WithLabelValues(name).Set(st.HighThreshold)
		}
		if temp.Crit.OK {
			st.CriticalHighThreshold = cleanupFloat(temp.Crit.Value)
			reg.PlatformMetrics.Temperature.CriticalHighThreshold.WithLabelValues(name).Set(st.CriticalHighThreshold)
		}
		if temp.Min.OK {
			st.LowThreshold = cleanupFloat(temp.Min.Value)
			reg.PlatformMetrics.Temperature.LowThreshold.WithLabelValues(name).Set(st.LowThreshold)
		}

		swState.Platform.Temperatures[name] = st
	}

	return nil
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
