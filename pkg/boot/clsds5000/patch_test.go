// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package clsds5000

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPatchData(t *testing.T) {
	inData := []byte(`
            "description":"DS5000 pddf platform with BMC, ver v1.2",
		"PORT1-CTRL":
    {
        "dev_info": { "device_type":"", "device_name":"PORT1-CTRL", "device_parent":"MUX1", "virt_parent":"PORT1"},
        "i2c":
        {
            "topo_info": { "parent_bus":"0x4", "dev_addr":"0x42", "dev_type":"pddf_xcvr"},
            "attr_list":
            [
                { "attr_name":"xcvr_reset", "attr_devaddr":"0x42", "attr_devtype":"cpld", "attr_devname":"CPLD1", "attr_offset":"0x42", "attr_mask":"42", "attr_cmpval":"0x0", "attr_len":"1"},
                { "attr_name":"xcvr_lpmode", "attr_devaddr":"0x42", "attr_devtype":"cpld", "attr_devname":"CPLD1", "attr_offset":"0x42", "attr_mask":"42", "attr_cmpval":"0x1", "attr_len":"1"},
                { "attr_name":"xcvr_present", "attr_devaddr":"0x42", "attr_devtype":"cpld", "attr_devname":"CPLD1", "attr_offset":"0x42", "attr_mask":"42", "attr_cmpval":"0x0", "attr_len":"1"},
                { "attr_name":"xcvr_rxlos", "attr_devaddr":"0x42", "attr_devtype":"cpld", "attr_devname":"CPLD1", "attr_offset":"0x42", "attr_mask":"42", "attr_cmpval":"0x0", "attr_len":"1"}
            ]
        }
    },
`)
	expectedData := []byte(`
            "description":"DS5000 pddf platform with BMC, ver v1.2-hh1",
		"PORT1-CTRL":
    {
        "dev_info": { "device_type":"", "device_name":"PORT1-CTRL", "device_parent":"MUX1", "virt_parent":"PORT1"},
        "i2c":
        {
            "topo_info": { "parent_bus":"0x4", "dev_addr":"0x42", "dev_type":"pddf_xcvr"},
            "attr_list":
            [
                { "attr_name":"xcvr_reset", "attr_devaddr":"0x42", "attr_devtype":"cpld", "attr_devname":"CPLD1", "attr_offset":"0x42", "attr_mask":"42", "attr_cmpval":"0x1", "attr_len":"1"},
                { "attr_name":"xcvr_lpmode", "attr_devaddr":"0x42", "attr_devtype":"cpld", "attr_devname":"CPLD1", "attr_offset":"0x42", "attr_mask":"42", "attr_cmpval":"0x1", "attr_len":"1"},
                { "attr_name":"xcvr_present", "attr_devaddr":"0x42", "attr_devtype":"cpld", "attr_devname":"CPLD1", "attr_offset":"0x42", "attr_mask":"42", "attr_cmpval":"0x0", "attr_len":"1"},
                { "attr_name":"xcvr_rxlos", "attr_devaddr":"0x42", "attr_devtype":"cpld", "attr_devname":"CPLD1", "attr_offset":"0x42", "attr_mask":"42", "attr_cmpval":"0x0", "attr_len":"1"}
            ]
        }
    },
`)

	actualData := patchData(inData)
	require.Equal(t, string(expectedData), string(actualData))

	doublePatchData := patchData(actualData)
	require.Equal(t, string(expectedData), string(doublePatchData))
}
