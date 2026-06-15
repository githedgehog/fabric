// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

// Command agent-ctl manages the Hedgehog agent lifecycle on a running SONiC VM
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
	sshAddr  = flag.String("ssh", "localhost:2222", "SSH address of SONiC VM")
	username = flag.String("user", "admin", "SSH username")
	sshKey   = flag.String("key", "./sshkey", "Path to SSH private key")
	binary   = flag.String("binary", "../../../bin/agent", "Path to agent binary (install only)")
	verbose  = flag.Bool("v", false, "Verbose logging")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <command>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  install    Install agent on SONiC VM\n")
		fmt.Fprintf(os.Stderr, "  uninstall  Uninstall agent from SONiC VM\n")
		fmt.Fprintf(os.Stderr, "  status     Show agent installation status\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := flag.Arg(0)

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
		fmt.Fprintf(os.Stderr, "Specify the path to your SSH key with -key\n")
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

	// Execute command
	switch command {
	case "install":
		handleInstall(ctx, mgr)
	case "uninstall":
		handleUninstall(ctx, mgr)
	case "status":
		handleStatus(ctx, mgr)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

func handleInstall(ctx context.Context, mgr *agent.Manager) {
	// Verify binary exists
	if _, err := os.Stat(*binary); err != nil {
		slog.Error("Agent binary not found", "path", *binary, "err", err)
		fmt.Fprintf(os.Stderr, "\nERROR: Agent binary not found at %s\n", *binary)
		fmt.Fprintf(os.Stderr, "\nBuild the agent first:\n")
		fmt.Fprintf(os.Stderr, "  cd ../../../\n")
		fmt.Fprintf(os.Stderr, "  just build-agent\n")
		os.Exit(1)
	}

	slog.Info("Installing Hedgehog agent", "ssh_addr", *sshAddr, "binary", *binary)

	if err := mgr.Install(ctx, *binary); err != nil {
		slog.Error("Failed to install agent", "err", err)
		os.Exit(1)
	}

	// Verify
	status, err := mgr.GetStatus(ctx)
	if err != nil {
		slog.Error("Failed to verify installation", "err", err)
		os.Exit(1)
	}

	slog.Info("Installation complete", "final_status", status)
	fmt.Println("\nSUCCESS: Agent successfully installed on SONiC VM")
	fmt.Printf("\nAgent status: %s\n", status)
	fmt.Println("\nYou can now:")
	fmt.Printf("  - Test gNMI: gnmic -a localhost:8080 -u admin --skip-verify capabilities\n")
	fmt.Printf("  - Check logs: ssh -i %s %s@%s 'tail -f /var/log/agent.log'\n", *sshKey, *username, *sshAddr)
	fmt.Printf("  - Check status: ./bin/agent-ctl -ssh %s status\n", *sshAddr)
	fmt.Printf("  - Uninstall:  ./bin/agent-ctl -ssh %s uninstall\n", *sshAddr)
}

func handleUninstall(ctx context.Context, mgr *agent.Manager) {
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
	fmt.Printf("\nAgent status: %s\n", status)
}

func handleStatus(ctx context.Context, mgr *agent.Manager) {
	slog.Info("Checking agent status", "ssh_addr", *sshAddr)

	status, err := mgr.GetStatus(ctx)
	if err != nil {
		slog.Error("Failed to get agent status", "err", err)
		os.Exit(1)
	}

	fmt.Printf("Agent status: %s\n", status)
}
