// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agentgnmi_test

import (
	"context"
	"testing"
	"time"
)

// TestVMBoots verifies that the SONiC VS VM starts and becomes accessible
func TestVMBoots(t *testing.T) {
	if *targetType != "vs" {
		t.Skip("Test only applicable for VS target")
	}

	if testVM == nil {
		t.Fatal("Test VM not initialized")
	}

	// VM should already be running and ready from TestMain
	if !testVM.IsRunning() {
		t.Fatal("VM is not running")
	}

	t.Log("VM is running")
	t.Logf("SSH address: %s", testVM.SSHAddress())
	t.Logf("gNMI address: %s", testVM.GNMIAddress())
	t.Logf("Serial log: %s", testVM.SerialLogPath())
}

// TestSSHConnectivity verifies SSH connection to SONiC
func TestSSHConnectivity(t *testing.T) {
	if *targetType != "vs" {
		t.Skip("Test only applicable for VS target")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try SSH connection
	if err := testVM.CheckSSH(ctx); err != nil {
		t.Fatalf("SSH connection failed: %v", err)
	}

	t.Log("SSH connection successful")
}
