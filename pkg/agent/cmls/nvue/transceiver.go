// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"net/url"
)

// Transceiver is a module in a port cage, the diagnostics are only there for modules that have them
type Transceiver struct {
	// Identifier is the SFF identifier with its name, such as "0x19 (OSFP 8X Pluggable Transceiver)"
	Identifier         string `json:"identifier"`
	Status             string `json:"status"`
	ErrorStatus        string `json:"error-status"`
	DiagnosticsStatus  string `json:"diagnostics-status"`
	CableType          string `json:"cable-type"`
	CableLength        Number `json:"cable-length"`
	PMDType            string `json:"pmd-type"`
	RevisionCompliance string `json:"revision-compliance"`

	VendorName     string `json:"vendor-name"`
	VendorOUI      string `json:"vendor-oui"`
	VendorPN       string `json:"vendor-pn"`
	VendorRev      string `json:"vendor-rev"`
	VendorSN       string `json:"vendor-sn"`
	VendorDateCode string `json:"vendor-date-code"`

	Temperature *TransceiverTemperature       `json:"temperature"`
	Voltage     *TransceiverVoltage           `json:"voltage"`
	Channel     map[string]TransceiverChannel `json:"channel"`
}

type TransceiverTemperature struct {
	// Temperature is in Celsius
	Temperature Number `json:"temperature"`
}

type TransceiverVoltage struct {
	Voltage Number `json:"voltage"`
}

type TransceiverChannel struct {
	RxPower       *TransceiverPower `json:"rx-power"`
	TxPower       *TransceiverPower `json:"tx-power"`
	TxBiasCurrent *TransceiverBias  `json:"tx-bias-current"`
	RxLOS         Flag              `json:"rx-los"`
	TxLOS         Flag              `json:"tx-los"`
	TxFault       Flag              `json:"tx-fault"`
	RxCDRLOL      Flag              `json:"rx-cdr-lol"`
	TxCDRLOL      Flag              `json:"tx-cdr-lol"`
}

type TransceiverPower struct {
	Power Power `json:"power"`
}

type TransceiverBias struct {
	// Current is in mA
	Current Number `json:"current"`
}

// Transceivers returns every inserted module by the name of its port, the base port for the broken out ones, with all
// details in one request
func (c *Client) Transceivers(ctx context.Context) (map[string]Transceiver, error) {
	transceivers := map[string]Transceiver{}
	if err := c.get(ctx, pathOf("platform", "transceiver"), url.Values{"view": {"detail"}}, &transceivers); err != nil {
		return nil, err
	}

	return transceivers, nil
}
