// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

// Command agent-install installs the Hedgehog agent on a running SONiC VM
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"go.githedgehog.com/fabric/test/integration/agent-gnmi/pkg/agent"
)

var (
	sshAddr = flag.String("ssh", "localhost:2222", "SSH address of SONiC VM")
	username = flag.String("user", "admin", "SSH username")
	sshKey   = flag.String("key", "./sshkey", "Path to SSH private key")
	binary   = flag.String("binary", "../../../bin/agent", "Path to agent binary")
	verbose  = flag.Bool("v", false, "Verbose logging")
)

func main() {
	flag.Parse()

	// Setup logging
	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))

	// Verify binary exists
	if _, err := os.Stat(*binary); err != nil {
		slog.Error("Agent binary not found", "path", *binary, "err", err)
		fmt.Fprintf(os.Stderr, "\nERROR: Agent binary not found at %s\n", *binary)
		fmt.Fprintf(os.Stderr, "\nBuild the agent first:\n")
		fmt.Fprintf(os.Stderr, "  cd ../../../\n")
		fmt.Fprintf(os.Stderr, "  go build -o test/integration/agent-gnmi/bin/agent ./cmd/agent\n")
		os.Exit(1)
	}

	// Load SSH key
	keyData, err := os.ReadFile(*sshKey)
	if err != nil {
		slog.Error("Failed to read SSH key", "path", *sshKey, "err", err)
		fmt.Fprintf(os.Stderr, "\nERROR: SSH key not found at %s\n", *sshKey)
		fmt.Fprintf(os.Stderr, "\nSpecify the path to your SSH key with -key\n")
		os.Exit(1)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		slog.Error("Failed to parse SSH key", "err", err)
		fmt.Fprintf(os.Stderr, "\nERROR: Invalid SSH key\n")
		os.Exit(1)
	}

	// Create SSH config
	sshConfig := &ssh.ClientConfig{
		User: *username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Create agent manager
	mgr := agent.NewManager(*sshAddr, sshConfig)

	ctx := context.Background()

	// Check current status
	slog.Info("Checking current agent status", "ssh_addr", *sshAddr)
	status, err := mgr.GetStatus(ctx)
	if err != nil {
		slog.Error("Failed to get agent status", "err", err)
		os.Exit(1)
	}
	slog.Info("Current status", "status", status)

	// Install agent
	slog.Info("Installing Hedgehog agent", "ssh_addr", *sshAddr, "binary", *binary)

	if err := mgr.Install(ctx, *binary); err != nil {
		slog.Error("Failed to install agent", "err", err)
		os.Exit(1)
	}

	// Verify
	status, err = mgr.GetStatus(ctx)
	if err != nil {
		slog.Error("Failed to verify installation", "err", err)
		os.Exit(1)
	}

	slog.Info("Installation complete", "final_status", status)
	fmt.Println("\nSUCCESS: Agent successfully installed on SONiC VM")
	fmt.Printf("\nAgent status: %s\n", status)
	fmt.Println("\nYou can now:")
	fmt.Printf("  - Test gNMI: gnmic -a localhost:8080 -u admin --skip-verify capabilities\n")
	fmt.Printf("  - Check logs: ssh -i %s admin@localhost 'tail -f /var/log/agent.log'\n", *sshKey)
	fmt.Printf("  - Check status: ssh -i %s admin@localhost 'systemctl status hedgehog-agent'\n", *sshKey)
	fmt.Println("  - Uninstall:  ./bin/agent-uninstall")
}
