// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agentgnmi_test

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.githedgehog.com/fabric/test/integration/agent-gnmi/pkg/agent"
	"go.githedgehog.com/fabric/test/integration/agent-gnmi/pkg/sonicvm"
)

var (
	targetType    = flag.String("target", "vs", "Target type: vs or hardware")
	agentBinary   = flag.String("agent-binary", "../../../bin/agent", "Path to agent binary")
	cacheDir      = flag.String("cache-dir", "./images", "Cache directory for VM images")
	keepOnFailure = flag.Bool("keep-on-failure", false, "Keep VM running on test failure")
	buildAgent    = flag.Bool("build-agent", true, "Build agent binary before tests")
)

// Global test VM (for VS testing)
var testVM *sonicvm.VM
var testAgentMgr *agent.Manager

func TestMain(m *testing.M) {
	flag.Parse()

	// Setup logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if *targetType != "vs" {
		slog.Info("Skipping VM setup - not VS target", "target", *targetType)
		os.Exit(m.Run())
	}

	// Setup SONiC VS
	ctx := context.Background()

	slog.Info("Setting up SONiC VS for integration tests",
		"cache_dir", *cacheDir,
		"agent_binary", *agentBinary,
	)

	// Build agent binary if requested
	if *buildAgent {
		slog.Info("Building agent binary using justfile...")

		// Build agent - find fabric root by going up from current directory
		fabricRoot, err := filepath.Abs("../../..")
		if err != nil {
			slog.Error("Failed to get fabric root path", "err", err)
			os.Exit(1)
		}

		// Use the justfile build-agent target (handles CGO_ENABLED=0, GOOS=linux, ldflags, etc.)
		cmd := exec.Command("just", "build-agent")
		cmd.Dir = fabricRoot
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			slog.Error("Failed to build agent", "err", err)
			os.Exit(1)
		}
		slog.Info("Agent binary built successfully", "path", *agentBinary)
	}

	// Create VM
	vm, err := sonicvm.New(sonicvm.Config{
		Name:    "sonic-vs-test",
		WorkDir: *cacheDir,
		Memory:  "4096M",
		CPUs:    4, // SONiC containers need >2 CPUs
	})
	if err != nil {
		slog.Error("Failed to create VM", "err", err)
		os.Exit(1)
	}
	testVM = vm

	// Start VM
	if err := vm.Start(ctx); err != nil {
		slog.Error("Failed to start VM", "err", err)
		os.Exit(1)
	}

	// Wait for VM to be ready
	slog.Info("Waiting for SONiC VS to boot and be ready...")
	if err := vm.WaitReady(ctx, 5*time.Minute); err != nil {
		slog.Error("VM not ready", "err", err)
		slog.Info("Serial log available at", "path", vm.SerialLogPath())
		vm.Stop(context.Background())
		os.Exit(1)
	}

	slog.Info("SONiC VS is ready",
		"ssh", vm.SSHAddress(),
		"gnmi", vm.GNMIAddress(),
	)

	// Create agent manager
	testAgentMgr = agent.NewManager(vm.SSHAddress(), vm.SSHConfig())

	// Prepare test environment (fix services for virtualized testing)
	slog.Info("Preparing test environment...")
	if err := testAgentMgr.PrepareTestEnvironment(ctx); err != nil {
		slog.Error("Failed to prepare test environment", "err", err)
		slog.Info("Serial log available at", "path", vm.SerialLogPath())
		vm.Stop(context.Background())
		os.Exit(1)
	}

	// Uninstall any existing agent (clean slate for tests)
	slog.Info("Ensuring clean slate - uninstalling any existing agent...")
	if err := testAgentMgr.Uninstall(ctx); err != nil {
		slog.Warn("Failed to uninstall existing agent", "err", err)
		// Continue anyway - might not have been installed
	}

	// Install agent for testing
	slog.Info("Installing agent on SONiC VS...")
	if err := testAgentMgr.Install(ctx, *agentBinary); err != nil {
		slog.Error("Failed to install agent", "err", err)
		slog.Info("Serial log available at", "path", vm.SerialLogPath())
		vm.Stop(context.Background())
		os.Exit(1)
	}

	// Verify agent is running
	status, err := testAgentMgr.GetStatus(ctx)
	if err != nil {
		slog.Error("Failed to get agent status", "err", err)
	} else {
		slog.Info("Agent status", "status", status)
	}

	slog.Info("Test environment ready - running tests...")

	// Run tests
	code := m.Run()

	// Cleanup
	if code != 0 && *keepOnFailure {
		slog.Warn("Tests failed - keeping VM running for debugging",
			"ssh", vm.SSHAddress(),
			"serial_log", vm.SerialLogPath(),
		)
		slog.Info("To connect: ssh -i ./sshkey -p 2222 admin@localhost")
		slog.Info("Press Ctrl+C to stop VM and exit")
		select {} // Block forever
	}

	// Uninstall agent before stopping VM
	slog.Info("Cleaning up - uninstalling agent...")
	uninstallCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := testAgentMgr.Uninstall(uninstallCtx); err != nil {
		slog.Warn("Failed to uninstall agent during cleanup", "err", err)
	}
	cancel()

	slog.Info("Stopping SONiC VS...")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()
	if err := vm.Stop(stopCtx); err != nil {
		slog.Error("Failed to stop VM", "err", err)
	}

	os.Exit(code)
}
