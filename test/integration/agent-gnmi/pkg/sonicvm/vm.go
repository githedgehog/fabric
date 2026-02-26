// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package sonicvm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	// File names
	OSImageFile  = "sonic-vs.qcow2"
	EFICodeFile  = "efi_code.fd"
	EFIVarsFile  = "efi_vars.fd"
	SerialLog    = "serial.log"
	SerialSock   = "serial.sock"
	MonSock      = "mon.sock"

	// Default values
	DefaultMemory   = "4096M"
	DefaultCPUs     = 4  // SONiC containers need >2 CPUs for their CPU limits
	DefaultSSHPort  = 2222
	DefaultGNMIPort = 8080

	// SONiC defaults
	DefaultUsername = "admin"
	DefaultSSHKey   = "./sshkey" // SSH key in test directory
)

// Config holds VM configuration
type Config struct {
	Name      string
	WorkDir   string // Directory containing VM images
	Memory    string
	CPUs      int
	SSHPort   int    // Host port for SSH
	GNMIPort  int    // Host port for gNMI
	SSHKeyPath string // Path to SSH private key (optional, uses default if empty)
}

// VM represents a running SONiC VS instance
type VM struct {
	config  Config
	process *exec.Cmd
	cancel  context.CancelFunc
}

// New creates a new VM instance (not started)
func New(cfg Config) (*VM, error) {
	// Validate config
	if cfg.Name == "" {
		return nil, fmt.Errorf("VM name is required")
	}
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("work directory is required")
	}

	// Apply defaults
	if cfg.Memory == "" {
		cfg.Memory = DefaultMemory
	}
	if cfg.CPUs == 0 {
		cfg.CPUs = DefaultCPUs
	}
	if cfg.SSHPort == 0 {
		cfg.SSHPort = DefaultSSHPort
	}
	if cfg.GNMIPort == 0 {
		cfg.GNMIPort = DefaultGNMIPort
	}

	// Check required files exist
	requiredFiles := []string{OSImageFile, EFICodeFile, EFIVarsFile}
	for _, file := range requiredFiles {
		path := filepath.Join(cfg.WorkDir, file)
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("required file %s not found: %w", file, err)
		}
	}

	return &VM{config: cfg}, nil
}

// Start starts the VM
func (vm *VM) Start(ctx context.Context) error {
	if vm.process != nil {
		return fmt.Errorf("VM already running")
	}

	// Create cancelable context for VM process
	vmCtx, cancel := context.WithCancel(ctx)
	vm.cancel = cancel

	// Check image format for EFI files
	efiFormat, err := getImageFormat(filepath.Join(vm.config.WorkDir, EFICodeFile))
	if err != nil {
		return fmt.Errorf("getting EFI image format: %w", err)
	}

	// Build QEMU arguments
	args := []string{
		"-name", vm.config.Name,
		"-m", vm.config.Memory,
		"-machine", "q35,accel=kvm,smm=on",
		"-cpu", "host",
		"-smp", fmt.Sprintf("%d", vm.config.CPUs),
		"-object", "rng-random,filename=/dev/urandom,id=rng0",
		"-device", "virtio-rng-pci,rng=rng0",

		// Disk
		"-drive", fmt.Sprintf("if=none,file=%s,id=disk1", OSImageFile),
		"-device", "virtio-blk-pci,drive=disk1,bootindex=1",

		// EFI firmware
		"-drive", fmt.Sprintf("if=pflash,file=%s,format=%s,readonly=on", EFICodeFile, efiFormat),
		"-drive", fmt.Sprintf("if=pflash,file=%s,format=%s", EFIVarsFile, efiFormat),

		// Serial console
		"-nographic",
		"-chardev", fmt.Sprintf("socket,id=serial,path=%s,server=on,wait=off,signal=off,logfile=%s",
			SerialSock, SerialLog),
		"-serial", "chardev:serial",

		// Monitor and QMP
		"-monitor", fmt.Sprintf("unix:%s,server,nowait", MonSock),

		// Networking: user mode with port forwards
		// Use e1000 to match SONiC expectations (not virtio-net)
		"-netdev", fmt.Sprintf("user,id=mgmt,hostfwd=tcp::%d-:22,hostfwd=tcp::%d-:8080",
			vm.config.SSHPort, vm.config.GNMIPort),
		"-device", "e1000,netdev=mgmt",

		// Disable S3 sleep
		"-global", "ICH9-LPC.disable_s3=1",
	}

	slog.Info("Starting SONiC VS",
		"name", vm.config.Name,
		"memory", vm.config.Memory,
		"cpus", vm.config.CPUs,
		"ssh_port", vm.config.SSHPort,
		"gnmi_port", vm.config.GNMIPort,
	)
	slog.Debug("QEMU command", "cmd", "qemu-system-x86_64 "+strings.Join(args, " "))

	// Start QEMU
	cmd := exec.CommandContext(vmCtx, "qemu-system-x86_64", args...)
	cmd.Dir = vm.config.WorkDir
	cmd.Stdout = &logWriter{prefix: fmt.Sprintf("[%s] ", vm.config.Name), level: slog.LevelDebug}
	cmd.Stderr = &logWriter{prefix: fmt.Sprintf("[%s] ", vm.config.Name), level: slog.LevelWarn}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("starting QEMU: %w", err)
	}

	vm.process = cmd

	// Monitor process in background
	go func() {
		err := cmd.Wait()
		if err != nil && vmCtx.Err() == nil {
			// Process exited unexpectedly
			slog.Error("QEMU process exited unexpectedly", "name", vm.config.Name, "err", err)
		}
	}()

	slog.Info("SONiC VS started", "name", vm.config.Name, "pid", cmd.Process.Pid)
	return nil
}

// Stop gracefully stops the VM
func (vm *VM) Stop(ctx context.Context) error {
	if vm.process == nil {
		return nil // Already stopped
	}

	slog.Info("Stopping SONiC VS", "name", vm.config.Name)

	// Cancel VM context
	if vm.cancel != nil {
		vm.cancel()
	}

	// Try graceful shutdown via QEMU monitor
	// For now, just send SIGTERM and wait
	if err := vm.process.Process.Signal(os.Interrupt); err != nil {
		slog.Warn("Failed to send SIGINT to QEMU", "name", vm.config.Name, "err", err)
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- vm.process.Wait()
	}()

	select {
	case <-ctx.Done():
		// Context timeout, force kill
		slog.Warn("VM shutdown timeout, force killing", "name", vm.config.Name)
		if err := vm.process.Process.Kill(); err != nil {
			return fmt.Errorf("killing VM: %w", err)
		}
		<-done // Wait for actual exit
	case err := <-done:
		if err != nil && err.Error() != "signal: interrupt" {
			slog.Warn("VM exited with error", "name", vm.config.Name, "err", err)
		}
	}

	vm.process = nil
	slog.Info("SONiC VS stopped", "name", vm.config.Name)
	return nil
}

// WaitReady waits for SONiC to boot and be accessible via SSH
func (vm *VM) WaitReady(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	slog.Info("Waiting for SONiC VS to be ready", "name", vm.config.Name, "timeout", timeout)

	// Poll SSH until it's available
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for SONiC VS to be ready: %w", ctx.Err())
		case <-ticker.C:
			attempt++
			slog.Debug("Checking SSH availability", "name", vm.config.Name, "attempt", attempt)

			if err := vm.CheckSSH(ctx); err != nil {
				slog.Debug("SSH not ready yet", "name", vm.config.Name, "err", err)
				continue
			}

			slog.Info("SONiC VS is ready", "name", vm.config.Name, "attempts", attempt)
			return nil
		}
	}
}

// CheckSSH attempts to connect to SSH and run a simple command
func (vm *VM) CheckSSH(ctx context.Context) error {
	config := vm.SSHConfig()
	config.Timeout = 5 * time.Second

	addr := fmt.Sprintf("localhost:%d", vm.config.SSHPort)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return err
	}
	defer client.Close()

	// Run simple command to verify SSH is fully working
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	output, err := session.CombinedOutput("echo ready")
	if err != nil {
		return err
	}

	if !strings.Contains(string(output), "ready") {
		return fmt.Errorf("unexpected output from SSH: %s", output)
	}

	return nil
}

// SSHConfig returns SSH connection configuration
func (vm *VM) SSHConfig() *ssh.ClientConfig {
	authMethods := []ssh.AuthMethod{}

	// Try SSH key first
	keyPath := vm.config.SSHKeyPath
	if keyPath == "" {
		keyPath = DefaultSSHKey
	}

	if signer, err := loadSSHKey(keyPath); err == nil {
		authMethods = append(authMethods, ssh.PublicKeys(signer))
		slog.Debug("Loaded SSH key for authentication", "path", keyPath)
	} else {
		slog.Debug("SSH key not available, will try without it", "path", keyPath, "err", err)
	}

	return &ssh.ClientConfig{
		User:            DefaultUsername,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
}

// loadSSHKey loads an SSH private key from a file
func loadSSHKey(keyPath string) (ssh.Signer, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("reading SSH key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parsing SSH key: %w", err)
	}

	return signer, nil
}

// SSHAddress returns the SSH address
func (vm *VM) SSHAddress() string {
	return fmt.Sprintf("localhost:%d", vm.config.SSHPort)
}

// GNMIAddress returns the gNMI address
func (vm *VM) GNMIAddress() string {
	return fmt.Sprintf("localhost:%d", vm.config.GNMIPort)
}

// RunSSHCommand executes a command on the VM via SSH
func (vm *VM) RunSSHCommand(ctx context.Context, command string) (string, error) {
	client, err := ssh.Dial("tcp", vm.SSHAddress(), vm.SSHConfig())
	if err != nil {
		return "", fmt.Errorf("connecting to SSH: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	return string(output), err
}

// SerialLogPath returns the path to the serial console log
func (vm *VM) SerialLogPath() string {
	return filepath.Join(vm.config.WorkDir, SerialLog)
}

// IsRunning returns true if the VM process is running
func (vm *VM) IsRunning() bool {
	return vm.process != nil && vm.process.Process != nil
}

// getImageFormat detects if a file is qcow2 or raw format
func getImageFormat(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	header := make([]byte, 4)
	if _, err := f.Read(header); err != nil {
		return "", fmt.Errorf("reading file header: %w", err)
	}

	// qcow2 magic: "QFI\xfb"
	if string(header) == "QFI\xfb" {
		return "qcow2", nil
	}

	return "raw", nil
}

// logWriter implements io.Writer and logs to slog
type logWriter struct {
	prefix string
	level  slog.Level
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		slog.Log(context.Background(), w.level, w.prefix+msg)
	}
	return len(p), nil
}
