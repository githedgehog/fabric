// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const (
	// Agent paths on SONiC
	AgentBinaryPath      = "/opt/hedgehog/bin/agent"
	AgentServiceFile     = "/etc/systemd/system/hedgehog-agent.service"
	AgentConfigDir       = "/etc/hedgehog"
	AgentInstallDir      = "/opt/hedgehog"
	AgentServiceName     = "hedgehog-agent"

	// Build paths
	DefaultAgentBinary   = "../../../bin/agent"
)

// Manager handles agent lifecycle on SONiC VM
type Manager struct {
	sshConfig *ssh.ClientConfig
	sshAddr   string
}

// PrepareTestEnvironment configures the SONiC VM for testing
// This fixes services that may not work correctly in virtualized test environments
func (m *Manager) PrepareTestEnvironment(ctx context.Context) error {
	slog.Info("Preparing SONiC VM test environment")

	// Fix eth0-dhcp service: change from Type=simple to Type=oneshot with RemainAfterExit=yes
	// This makes the service report as "active" after dhclient successfully gets a lease
	slog.Debug("Fixing eth0-dhcp service for test environment")
	serviceContent := `[Unit]
Description=DHCP client for eth0
After=network.target

[Service]
Type=oneshot
ExecStart=/sbin/dhclient -v eth0
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
	cmd := fmt.Sprintf("sudo tee /etc/systemd/system/eth0-dhcp.service > /dev/null <<'EOF'\n%sEOF", serviceContent)
	if _, err := m.runSSHCommand(ctx, cmd); err != nil {
		return fmt.Errorf("updating eth0-dhcp service: %w", err)
	}

	// Reload systemd and restart service
	if _, err := m.runSSHCommand(ctx, "sudo systemctl daemon-reload"); err != nil {
		return fmt.Errorf("reloading systemd: %w", err)
	}

	if _, err := m.runSSHCommand(ctx, "sudo systemctl restart eth0-dhcp"); err != nil {
		slog.Warn("Failed to restart eth0-dhcp (may be OK if already configured)", "err", err)
	}

	// Install Grafana Alloy for observability
	// The agent config from VLAB references alloy with a registry that doesn't exist in standalone VMs
	// We install alloy from GitHub releases (downloaded on host, then copied to VM)
	slog.Debug("Installing Grafana Alloy for observability")

	// Check if alloy is already installed on VM
	_, err := m.runSSHCommand(ctx, "test -f /usr/local/bin/alloy")
	if err == nil {
		slog.Debug("Alloy already installed on VM, verifying version")
		output, verErr := m.runSSHCommand(ctx, "/usr/local/bin/alloy --version")
		if verErr == nil {
			slog.Debug("Alloy version", "output", strings.TrimSpace(output))
		}
	} else {
		// Download alloy on host (VM doesn't have internet access)
		alloyVersion := "v1.11.2"
		alloyURL := fmt.Sprintf("https://github.com/grafana/alloy/releases/download/%s/alloy-linux-amd64.zip", alloyVersion)
		alloyCachePath := "/tmp/alloy-" + alloyVersion

		// Check if already cached on host
		if _, err := os.Stat(alloyCachePath); err != nil {
			slog.Info("Downloading Grafana Alloy on host", "version", alloyVersion, "url", alloyURL)
			// Download zip file on host using http.Get
			resp, err := http.Get(alloyURL)
			if err != nil {
				return fmt.Errorf("downloading alloy from GitHub: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				return fmt.Errorf("downloading alloy: HTTP %d", resp.StatusCode)
			}

			// Save zip to temp file
			zipPath := "/tmp/alloy-" + alloyVersion + ".zip"
			zipFile, err := os.Create(zipPath)
			if err != nil {
				return fmt.Errorf("creating alloy zip file: %w", err)
			}

			if _, err := io.Copy(zipFile, resp.Body); err != nil {
				zipFile.Close()
				return fmt.Errorf("saving alloy zip: %w", err)
			}
			zipFile.Close()

			// Extract binary from zip
			slog.Debug("Extracting alloy binary from zip")
			zipReader, err := zip.OpenReader(zipPath)
			if err != nil {
				return fmt.Errorf("opening alloy zip: %w", err)
			}
			defer zipReader.Close()

			// Find and extract the alloy binary (should be alloy-linux-amd64)
			var extracted bool
			for _, file := range zipReader.File {
				if strings.Contains(file.Name, "alloy-linux-amd64") || file.Name == "alloy" {
					slog.Debug("Extracting file from zip", "name", file.Name)
					rc, err := file.Open()
					if err != nil {
						return fmt.Errorf("opening file in zip: %w", err)
					}

					outFile, err := os.Create(alloyCachePath)
					if err != nil {
						rc.Close()
						return fmt.Errorf("creating alloy binary file: %w", err)
					}

					_, err = io.Copy(outFile, rc)
					rc.Close()
					outFile.Close()
					if err != nil {
						return fmt.Errorf("extracting alloy binary: %w", err)
					}

					extracted = true
					break
				}
			}

			if !extracted {
				return fmt.Errorf("alloy binary not found in zip")
			}

			// Clean up zip file
			os.Remove(zipPath)

			// Make it executable
			if err := os.Chmod(alloyCachePath, 0755); err != nil {
				slog.Warn("Could not chmod alloy cache file", "err", err)
			}

			slog.Info("Alloy downloaded and extracted to host cache", "path", alloyCachePath)
		} else {
			slog.Debug("Using cached alloy from host", "path", alloyCachePath)
		}

		// Copy alloy binary to VM
		slog.Debug("Copying alloy binary to VM")
		if err := m.copyFileToVM(ctx, alloyCachePath, "/tmp/alloy"); err != nil {
			return fmt.Errorf("copying alloy to VM: %w", err)
		}

		// Install on VM
		slog.Debug("Installing alloy on VM")
		installCmd := "sudo mv /tmp/alloy /usr/local/bin/alloy && sudo chmod +x /usr/local/bin/alloy"
		if _, err := m.runSSHCommand(ctx, installCmd); err != nil {
			return fmt.Errorf("installing alloy on VM: %w", err)
		}

		// Verify installation
		output, err := m.runSSHCommand(ctx, "/usr/local/bin/alloy --version")
		if err != nil {
			slog.Warn("Could not verify alloy installation", "err", err)
		} else {
			slog.Info("Alloy installed successfully on VM", "version", strings.TrimSpace(output))
		}
	}

	slog.Info("Test environment prepared successfully")
	return nil
}

// NewManager creates a new agent manager
func NewManager(sshAddr string, sshConfig *ssh.ClientConfig) *Manager {
	return &Manager{
		sshAddr:   sshAddr,
		sshConfig: sshConfig,
	}
}

// IsInstalled checks if the agent is installed on the VM
func (m *Manager) IsInstalled(ctx context.Context) (bool, error) {
	output, err := m.runSSHCommand(ctx, "test -f "+AgentBinaryPath+" && echo installed || echo not-installed")
	if err != nil {
		return false, fmt.Errorf("checking if agent is installed: %w", err)
	}
	return strings.TrimSpace(output) == "installed", nil
}

// IsRunning checks if the agent service is running
func (m *Manager) IsRunning(ctx context.Context) (bool, error) {
	output, err := m.runSSHCommand(ctx, "systemctl is-active "+AgentServiceName+" 2>/dev/null || echo inactive")
	if err != nil {
		return false, fmt.Errorf("checking if agent is running: %w", err)
	}
	return strings.TrimSpace(output) == "active", nil
}

// Uninstall removes the agent from the VM
func (m *Manager) Uninstall(ctx context.Context) error {
	slog.Info("Uninstalling Hedgehog agent from SONiC VM")

	// Check if installed
	installed, err := m.IsInstalled(ctx)
	if err != nil {
		return fmt.Errorf("checking if agent is installed: %w", err)
	}

	if !installed {
		slog.Info("Agent not installed, nothing to uninstall")
		return nil
	}

	// Stop the service
	slog.Debug("Stopping agent service")
	_, err = m.runSSHCommand(ctx, "sudo systemctl stop "+AgentServiceName+" 2>/dev/null || true")
	if err != nil {
		slog.Warn("Failed to stop agent service (continuing anyway)", "err", err)
	}

	// Disable the service
	slog.Debug("Disabling agent service")
	_, err = m.runSSHCommand(ctx, "sudo systemctl disable "+AgentServiceName+" 2>/dev/null || true")
	if err != nil {
		slog.Warn("Failed to disable agent service (continuing anyway)", "err", err)
	}

	// Remove systemd service file
	slog.Debug("Removing systemd service file")
	_, err = m.runSSHCommand(ctx, "sudo rm -f "+AgentServiceFile)
	if err != nil {
		return fmt.Errorf("removing service file: %w", err)
	}

	// Reload systemd
	slog.Debug("Reloading systemd")
	_, err = m.runSSHCommand(ctx, "sudo systemctl daemon-reload")
	if err != nil {
		slog.Warn("Failed to reload systemd (continuing anyway)", "err", err)
	}

	// Remove agent binaries and directories
	slog.Debug("Removing agent installation directory")
	_, err = m.runSSHCommand(ctx, "sudo rm -rf "+AgentInstallDir)
	if err != nil {
		return fmt.Errorf("removing agent install directory: %w", err)
	}

	// Remove config directory
	slog.Debug("Removing agent config directory")
	_, err = m.runSSHCommand(ctx, "sudo rm -rf "+AgentConfigDir)
	if err != nil {
		return fmt.Errorf("removing agent config directory: %w", err)
	}

	// Clean up any rc.d scripts
	slog.Debug("Cleaning up startup scripts")
	_, err = m.runSSHCommand(ctx, "sudo find /etc/rc*.d -name '*hedgehog*' -delete 2>/dev/null || true")
	if err != nil {
		slog.Warn("Failed to clean up rc scripts (continuing anyway)", "err", err)
	}

	// Verify uninstallation
	stillInstalled, err := m.IsInstalled(ctx)
	if err != nil {
		return fmt.Errorf("verifying uninstallation: %w", err)
	}
	if stillInstalled {
		return fmt.Errorf("agent still appears to be installed after uninstallation")
	}

	slog.Info("Agent successfully uninstalled from SONiC VM")
	return nil
}

// Install installs the agent on the VM
func (m *Manager) Install(ctx context.Context, agentBinaryPath string) error {
	slog.Info("Installing Hedgehog agent on SONiC VM", "binary", agentBinaryPath)

	// Check if already installed
	installed, err := m.IsInstalled(ctx)
	if err != nil {
		return fmt.Errorf("checking if agent is installed: %w", err)
	}

	if installed {
		slog.Warn("Agent already installed, uninstalling first")
		if err := m.Uninstall(ctx); err != nil {
			return fmt.Errorf("uninstalling existing agent: %w", err)
		}
	}

	// Verify binary exists
	if _, err := os.Stat(agentBinaryPath); err != nil {
		return fmt.Errorf("agent binary not found at %s: %w", agentBinaryPath, err)
	}

	// Create installation directories
	slog.Debug("Creating installation directories")
	_, err = m.runSSHCommand(ctx, "sudo mkdir -p "+filepath.Dir(AgentBinaryPath))
	if err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}

	_, err = m.runSSHCommand(ctx, "sudo mkdir -p "+AgentConfigDir)
	if err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Clean old agent logs for fresh start
	slog.Debug("Cleaning old agent logs")
	_, err = m.runSSHCommand(ctx, "sudo rm -f /var/log/agent.log")
	if err != nil {
		slog.Warn("Failed to clean old agent logs (continuing anyway)", "err", err)
	}

	// Copy agent binary to VM
	slog.Debug("Copying agent binary to VM")
	tmpPath := "/tmp/agent"
	if err := m.copyFileToVM(ctx, agentBinaryPath, tmpPath); err != nil {
		return fmt.Errorf("copying agent binary: %w", err)
	}

	// Move to final location with sudo and set permissions
	slog.Debug("Installing agent binary")
	_, err = m.runSSHCommand(ctx, fmt.Sprintf("sudo mv %s %s && sudo chmod +x %s", tmpPath, AgentBinaryPath, AgentBinaryPath))
	if err != nil {
		return fmt.Errorf("installing agent binary: %w", err)
	}

	// Create systemd service file
	slog.Debug("Creating systemd service file")
	serviceContent := `[Unit]
Description=Hedgehog Fabric Agent
After=network.target

[Service]
Type=simple
ExecStart=/opt/hedgehog/bin/agent start
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`
	// Write service file via heredoc
	cmd := fmt.Sprintf("sudo tee %s > /dev/null <<'EOF'\n%sEOF", AgentServiceFile, serviceContent)
	_, err = m.runSSHCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("creating service file: %w", err)
	}

	// Reload systemd
	slog.Debug("Reloading systemd")
	_, err = m.runSSHCommand(ctx, "sudo systemctl daemon-reload")
	if err != nil {
		return fmt.Errorf("reloading systemd: %w", err)
	}

	// Enable service
	slog.Debug("Enabling agent service")
	_, err = m.runSSHCommand(ctx, "sudo systemctl enable "+AgentServiceName)
	if err != nil {
		return fmt.Errorf("enabling service: %w", err)
	}

	// Start service
	slog.Debug("Starting agent service")
	_, err = m.runSSHCommand(ctx, "sudo systemctl start "+AgentServiceName)
	if err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	// Verify installation
	installed, err = m.IsInstalled(ctx)
	if err != nil {
		return fmt.Errorf("verifying installation: %w", err)
	}
	if !installed {
		return fmt.Errorf("agent not found after installation")
	}

	// Check if service is enabled (not necessarily running yet)
	output, err := m.runSSHCommand(ctx, "systemctl is-enabled "+AgentServiceName+" 2>/dev/null || echo disabled")
	if err != nil {
		slog.Warn("Could not verify service is enabled", "err", err)
	}
	enabled := strings.TrimSpace(output) == "enabled"
	if !enabled {
		return fmt.Errorf("agent service not enabled after installation")
	}

	// Check if running, but don't fail if it's not (agent may be waiting for SONiC ready)
	running, err := m.IsRunning(ctx)
	if err != nil {
		slog.Warn("Could not check if agent is running", "err", err)
	}

	if running {
		slog.Info("Agent successfully installed and started on SONiC VM")
	} else {
		slog.Info("Agent successfully installed on SONiC VM", "note", "service will start when SONiC system is ready")
	}
	return nil
}

// GetStatus returns detailed status information about the agent
func (m *Manager) GetStatus(ctx context.Context) (string, error) {
	installed, err := m.IsInstalled(ctx)
	if err != nil {
		return "", err
	}

	if !installed {
		return "not installed", nil
	}

	running, err := m.IsRunning(ctx)
	if err != nil {
		return "", err
	}

	if running {
		// Get process info
		output, err := m.runSSHCommand(ctx, "ps aux | grep '/opt/hedgehog/bin/agent' | grep -v grep || echo ''")
		if err != nil {
			return "installed, running (details unavailable)", nil
		}
		return fmt.Sprintf("installed, running\n%s", strings.TrimSpace(output)), nil
	}

	return "installed, not running", nil
}

// runSSHCommand executes a command on the VM via SSH
func (m *Manager) runSSHCommand(ctx context.Context, command string) (string, error) {
	// Connect to SSH with the provided config
	client, err := ssh.Dial("tcp", m.sshAddr, m.sshConfig)
	if err != nil {
		return "", fmt.Errorf("connecting to SSH: %w", err)
	}
	defer client.Close()

	// Create session
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	// Run command
	slog.Debug("Running SSH command", "command", command)
	output, err := session.CombinedOutput(command)
	if err != nil {
		// Check if it's just a non-zero exit code
		if exitErr, ok := err.(*ssh.ExitError); ok {
			// Return output even on non-zero exit for commands that use || true
			return string(output), fmt.Errorf("command exited with code %d: %w", exitErr.ExitStatus(), err)
		}
		return string(output), fmt.Errorf("running command: %w", err)
	}

	return string(output), nil
}

// copyFileToVM copies a local file to the VM using SFTP
func (m *Manager) copyFileToVM(ctx context.Context, localPath, remotePath string) error {
	// Create SSH client
	client, err := ssh.Dial("tcp", m.sshAddr, m.sshConfig)
	if err != nil {
		return fmt.Errorf("connecting to SSH: %w", err)
	}
	defer client.Close()

	// Create SFTP client
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("creating SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer localFile.Close()

	// Get file info for size
	fileInfo, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("getting file info: %w", err)
	}

	// Create remote file
	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("creating remote file: %w", err)
	}
	defer remoteFile.Close()

	// Copy file
	slog.Debug("Copying file via SFTP", "local", localPath, "remote", remotePath, "size", fileInfo.Size())
	written, err := io.Copy(remoteFile, localFile)
	if err != nil {
		return fmt.Errorf("copying file: %w", err)
	}

	if written != fileInfo.Size() {
		return fmt.Errorf("incomplete copy: wrote %d bytes, expected %d", written, fileInfo.Size())
	}

	slog.Debug("File copied successfully", "bytes", written)
	return nil
}
