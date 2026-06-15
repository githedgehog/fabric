// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

// Command agent-uninstall removes the Hedgehog agent from a running SONiC VM
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
	sshAddr    = flag.String("ssh", "localhost:2222", "SSH address of SONiC VM")
	username   = flag.String("user", "admin", "SSH username")
	sshKey     = flag.String("key", "./sshkey", "Path to SSH private key")
	showStatus = flag.Bool("status", false, "Show agent status instead of uninstalling")
	verbose    = flag.Bool("v", false, "Verbose logging")
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

	if *showStatus {
		// Show status
		status, err := mgr.GetStatus(ctx)
		if err != nil {
			slog.Error("Failed to get agent status", "err", err)
			os.Exit(1)
		}
		fmt.Printf("Agent status: %s\n", status)
		return
	}

	// Uninstall agent
	slog.Info("Uninstalling Hedgehog agent", "ssh_addr", *sshAddr)

	if err := mgr.Uninstall(ctx); err != nil {
		slog.Error("Failed to uninstall agent", "err", err)
		os.Exit(1)
	}

	// Verify
	status, err := mgr.GetStatus(ctx)
	if err != nil {
		slog.Error("Failed to verify uninstallation", "err", err)
		os.Exit(1)
	}

	slog.Info("Uninstallation complete", "final_status", status)
	fmt.Println("\nSUCCESS: Agent successfully uninstalled from SONiC VM")
	fmt.Println("\nTo persist these changes, you should shut down the VM cleanly:")
	fmt.Printf("  ssh -i %s -p 2222 %s@localhost 'sudo poweroff'\n", *sshKey, *username)
	fmt.Println("\nThen the image will be clean for future tests.")
}
