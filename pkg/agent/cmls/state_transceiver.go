// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
)

const (
	transceiverInserted        = "Inserted"
	transceiverPresent         = "PRESENT"
	transceiverOperActive      = "active"
	transceiverOperInactive    = "inactive"
	transceiverCMISRevPrefix   = "Rev. "
	transceiverChannelPrefix   = "channel-"
	transceiverPMDPrefix       = "ETH_"
	transceiverCablePassive    = "Passive copper cable"
	transceiverCableOptical    = "Optical module"
	transceiverCableClassDAC   = "DAC"
	transceiverCableClassFiber = "FIBER"
	breakoutSingle             = "1x"
)

// cage is the transceiver cage of a port
type cage struct {
	name string
	// the ports that can be broken out have their subports named E1/N/M, even when they aren't
	breakout bool
}

// cagesOf maps the NOS names of the ports to their cages, such as swp17 to E1/17 whatever its breakout is, which is how
// NVUE names the transceivers and the breakouts. The ports disabled by the breakout of their neighbor are cages too.
func cagesOf(profile *wiringapi.SwitchProfileSpec) map[string]cage {
	cages := map[string]cage{}
	for name, port := range profile.Ports {
		if port.Management {
			continue
		}

		portProfile, exists := profile.PortProfiles[port.Profile]
		cages[port.NOSName] = cage{name: name, breakout: exists && portProfile.Breakout != nil}
	}

	return cages
}

func (c *CumulusProcessor) updateTransceivers(ctx context.Context, reg *switchstate.Registry, swState *agentapi.SwitchState, cages map[string]cage, vxLegacy bool) error {
	transceivers, err := c.nvue.Transceivers(ctx)
	if err != nil {
		return fmt.Errorf("getting transceivers: %w", err)
	}

	// NVUE only lists the modules inserted, the empty cages are reported as well so they're visible as such
	for _, cage := range cages {
		swState.Transceivers[cage.name] = agentapi.SwitchStateTransceiver{}
	}

	for nosName, transceiver := range transceivers {
		// Cumulus VX has no modules, it lists placeholders without a status, even for subinterfaces
		if vxLegacy && transceiver.Status == "" {
			continue
		}

		cage, exists := cages[nosName]
		if !exists {
			slog.Warn("Port mapping not found, ignoring for metrics", "transceiver", nosName)

			continue
		}

		st, err := transceiverState(reg, cage.name, &transceiver)
		if err != nil {
			return fmt.Errorf("transceiver %s: %w", nosName, err)
		}
		swState.Transceivers[cage.name] = st
	}

	updateTransceiverInfoMetrics(reg, swState)

	return nil
}

func transceiverState(reg *switchstate.Registry, name string, transceiver *nvue.Transceiver) (agentapi.SwitchStateTransceiver, error) {
	if transceiver.Status != transceiverInserted {
		slog.Warn("Unknown transceiver status, considering it not present", "transceiver", name, "status", transceiver.Status)

		return agentapi.SwitchStateTransceiver{}, nil
	}

	st := agentapi.SwitchStateTransceiver{
		Present:      transceiverPresent,
		OperStatus:   transceiverOperActive,
		FormFactor:   formFactor(transceiver.Identifier),
		SerialNumber: transceiver.VendorSN,
		Vendor:       transceiver.VendorName,
		VendorPart:   transceiver.VendorPN,
		VendorOUI:    transceiver.VendorOUI,
		VendorRev:    transceiver.VendorRev,
	}

	if transceiver.ErrorStatus != notAvailable && transceiver.ErrorStatus != "" {
		st.OperStatus = transceiverOperInactive
	}

	st.Description = st.FormFactor
	if transceiver.PMDType != "" {
		pmd := strings.ReplaceAll(strings.TrimPrefix(transceiver.PMDType, transceiverPMDPrefix), "_", "-")
		st.Description = strings.TrimSpace(st.Description + " " + pmd)
	}

	switch transceiver.CableType {
	case transceiverCablePassive:
		st.CableClass = transceiverCableClassDAC
	case transceiverCableOptical:
		st.CableClass = transceiverCableClassFiber
	default:
		st.CableClass = transceiver.CableType
	}

	if transceiver.CableLength.OK {
		st.CableLength = cleanupFloat(transceiver.CableLength.Value)
	}

	// only the CMIS modules report a revision like that, SFF-8636 ones have the standard in it instead
	if rev, ok := strings.CutPrefix(transceiver.RevisionCompliance, transceiverCMISRevPrefix); ok {
		st.CMISRev = rev
	}

	// the modules without diagnostics, such as most DACs, report zeroes as their readings
	if transceiver.DiagnosticsStatus == notAvailable || transceiver.DiagnosticsStatus == "" {
		return st, nil
	}

	if transceiver.Temperature != nil && transceiver.Temperature.Temperature.OK {
		st.Temperature = cleanupFloat(transceiver.Temperature.Temperature.Value)
		reg.TransceiverMetrics.Temperature.WithLabelValues(name).Set(st.Temperature)
	}

	if transceiver.Voltage != nil && transceiver.Voltage.Voltage.OK {
		st.Voltage = cleanupFloat(transceiver.Voltage.Voltage.Value)
		reg.TransceiverMetrics.Voltage.WithLabelValues(name).Set(st.Voltage)
	}

	if len(transceiver.Channel) == 0 {
		return st, nil
	}

	st.Channels = map[string]agentapi.SwitchStateTransceiverChannel{}
	for chName, channel := range transceiver.Channel {
		ch, ok := strings.CutPrefix(chName, transceiverChannelPrefix)
		if !ok {
			return st, fmt.Errorf("unknown channel %q", chName) //nolint:err113
		}

		chSt := agentapi.SwitchStateTransceiverChannel{}
		if channel.RxPower != nil && channel.RxPower.Power.OK {
			power := channel.RxPower.Power.DBm
			chSt.In = &power
			reg.TransceiverMetrics.InPower.WithLabelValues(name, ch).Set(power)
		}
		if channel.TxPower != nil && channel.TxPower.Power.OK {
			power := channel.TxPower.Power.DBm
			chSt.Out = &power
			reg.TransceiverMetrics.OutPower.WithLabelValues(name, ch).Set(power)
		}
		if channel.TxBiasCurrent != nil && channel.TxBiasCurrent.Current.OK {
			chSt.Bias = cleanupFloat(channel.TxBiasCurrent.Current.Value)
			reg.TransceiverMetrics.Bias.WithLabelValues(name, ch).Set(chSt.Bias)
		}

		st.Channels[ch] = chSt
	}

	return st, nil
}

// formFactor is the name of the SFF identifier, such as QSFP28 for "0x11 (QSFP28)" and OSFP for
// "0x19 (OSFP 8X Pluggable Transceiver)"
func formFactor(identifier string) string {
	_, name, ok := strings.Cut(identifier, "(")
	if !ok {
		return identifier
	}

	name = strings.TrimSuffix(name, ")")
	if first, _, ok := strings.Cut(name, " "); ok {
		return first
	}

	return name
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

func (c *CumulusProcessor) updateBreakouts(ctx context.Context, swState *agentapi.SwitchState, cages map[string]cage) error {
	breakouts, err := c.nvue.Breakouts(ctx)
	if err != nil {
		return fmt.Errorf("getting breakouts: %w", err)
	}

	for nosName, breakout := range breakouts {
		cage, exists := cages[nosName]
		if !exists {
			slog.Warn("Breakout mapping not found, ignoring for metrics", "breakout", nosName)

			continue
		}
		// NVUE has the breakout config for the ports the switch profile has without any breakouts as well
		if !cage.breakout {
			continue
		}

		// the modes are named like in the switch profiles, such as 4x25G
		mode := breakout.Mode
		switch mode {
		case nvue.BreakoutDisabled:
		case "":
			mode = breakoutSingle + breakout.Speed
		default:
			mode += breakout.Speed
		}

		swState.Breakouts[cage.name] = agentapi.SwitchStateBreakout{Mode: mode}
	}

	return nil
}
