// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package clsds5000

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

const (
	cfgPath     = "/usr/share/sonic/platform/pddf/pddf-device.json" // symlink -> /usr/share/sonic/device/x86_64-cls_ds5000-r0/pddf/pddf-device.json
	description = `DS5000 pddf platform with BMC, ver v1.2`
	checkVal    = `"attr_name":"xcvr_reset"`
	oldVal      = `"attr_cmpval":"0x0"`
	newVal      = `"attr_cmpval":"0x1"`
)

func Patch() (bool, error) {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			// not a pddf device
			return false, nil
		}

		return false, fmt.Errorf("reading pddf-device.json: %w", err)
	}

	if !strings.Contains(string(data), description) {
		// not a DS5000
		return false, nil
	}

	newData := patchData(data)
	changed := !slices.Equal(data, newData)
	if changed {
		if err := os.WriteFile(cfgPath, newData, 0o644); err != nil { //nolint:gosec
			return false, fmt.Errorf("writing pddf-device.json: %w", err)
		}
	}

	return changed, nil
}

func patchData(data []byte) []byte {
	patched := strings.Builder{}
	for line := range strings.Lines(string(data)) {
		if strings.Contains(line, checkVal) && strings.Contains(line, oldVal) {
			line = strings.ReplaceAll(line, oldVal, newVal)
		} else if strings.Contains(line, description) {
			line = line[:strings.LastIndex(line, description)+len(description)] + "-hh1\",\n"
		}
		patched.WriteString(line)
	}
	res := []byte(patched.String())

	return res
}
