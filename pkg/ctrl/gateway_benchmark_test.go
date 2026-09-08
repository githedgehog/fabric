// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
)

// EXPERIMENT ONLY -- guards the benchmark-lab override. Must never merge.

func TestBenchmarkOverrideForcesPCIAndKeepsTheName(t *testing.T) {
	// The name is the key the IPs and MTU hang off; losing it detaches the addresses from the
	// interface silently, which is why this asserts on the key and not just on the PCI value.
	in := map[string]gwapi.GatewayInterface{
		"enp2s1": {Kernel: "enp2s1", IPs: []string{"172.30.128.4/31"}, MTU: 9036},
	}
	out, err := benchmarkInterfaceOverride("gateway-1", in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	iface, ok := out["enp2s1"]
	if !ok {
		t.Fatalf("interface name was not preserved, got keys %v", out)
	}
	if iface.PCI != "0000:02:01.0" {
		t.Errorf("PCI = %q, want 0000:02:01.0", iface.PCI)
	}
	if iface.Kernel != "" {
		t.Errorf("Kernel = %q, want it cleared (PCI and Kernel are mutually exclusive)", iface.Kernel)
	}
	if len(iface.IPs) != 1 || iface.IPs[0] != "172.30.128.4/31" {
		t.Errorf("IPs = %v, want them carried across", iface.IPs)
	}
	if iface.MTU != 9036 {
		t.Errorf("MTU = %d, want 9036 carried across", iface.MTU)
	}
}

func TestBenchmarkOverrideSelectsTheDPDKDriver(t *testing.T) {
	// The controller picks the driver by counting interfaces with a PCI address. If the override
	// failed to set one the run would quietly benchmark the kernel path instead -- a result that
	// looks like a result.
	out, err := benchmarkInterfaceOverride("gateway-2", map[string]gwapi.GatewayInterface{
		"enp2s1": {Kernel: "enp2s1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pcis, kernels := 0, 0
	for _, iface := range out {
		if iface.PCI != "" {
			pcis++
		}
		if iface.Kernel != "" {
			kernels++
		}
	}
	if pcis != 1 || kernels != 0 {
		t.Fatalf("pcis=%d kernels=%d; want 1 and 0 so the controller selects dpdk", pcis, kernels)
	}
}

func TestBenchmarkOverrideIsInertForOtherGateways(t *testing.T) {
	in := map[string]gwapi.GatewayInterface{"enp2s1": {Kernel: "enp2s1"}}
	out, err := benchmarkInterfaceOverride("gateway-3", in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["enp2s1"].PCI != "" || out["enp2s1"].Kernel != "enp2s1" {
		t.Errorf("a gateway outside the table was modified: %+v", out["enp2s1"])
	}
}

func TestBenchmarkOverrideRefusesMoreThanOneInterface(t *testing.T) {
	// One address cannot describe two ports; failing here beats an opaque EAL duplicate later.
	_, err := benchmarkInterfaceOverride("gateway-1", map[string]gwapi.GatewayInterface{
		"enp2s1": {}, "enp2s2": {},
	})
	if err == nil {
		t.Fatal("expected an error when the gateway has two interfaces")
	}
}
