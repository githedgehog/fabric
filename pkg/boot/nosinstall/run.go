// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nosinstall

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"go.githedgehog.com/fabric/pkg/boot/clsds5000"
	"go.githedgehog.com/fabric/pkg/util/logutil"
	"go.githedgehog.com/fabric/pkg/util/uefiutil"
)

const (
	WebhookLog    = true
	OnieURLSuffix = "/onie"
	LogURLSuffix  = "/log"
	KeySerial     = "serial"
	KeyEthAddr    = "ethaddr"
)

var AllowedBootReasons = []string{"install", "rescue"}

type Env struct {
	ExecURL    string
	BootReason string
	Serial     string
	EthAddr    string
	Platform   string
	DiscoIP    string
}

func ReadEnv(ctx context.Context) Env {
	env := Env{
		ExecURL:    os.Getenv("onie_exec_url"),
		BootReason: os.Getenv("onie_boot_reason"),
		Serial:     os.Getenv("onie_serial_num"),
		EthAddr:    os.Getenv("onie_eth_addr"),
		Platform:   os.Getenv("onie_platform"),
		DiscoIP:    os.Getenv("onie_disco_ip"),
	}

	if env.Serial == "" {
		env.Serial = runSysInfo(ctx, "-s")
	}
	if env.EthAddr == "" {
		env.EthAddr = runSysInfo(ctx, "-e")
	}
	if env.Platform == "" {
		env.Platform = runSysInfo(ctx, "-p")
	}

	return env
}

func runSysInfo(ctx context.Context, flag string) string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	stdOut, stdErr := &bytes.Buffer{}, &bytes.Buffer{}

	cmd := exec.CommandContext(ctx, "/bin/onie-sysinfo", flag)
	cmd.Stdout = stdOut
	cmd.Stderr = stdErr

	if err := cmd.Run(); err != nil {
		return ""
	}

	if len(strings.TrimSpace(stdErr.String())) > 0 {
		return ""
	}

	out := strings.TrimSpace(stdOut.String())
	if len(out) > 64 {
		return ""
	}

	return out
}

func Run(ctx context.Context, env Env, dryRun bool) (funcErr error) { //nolint:nonamedreturns
	args := []any{}
	if env.BootReason != "" {
		args = append(args, "reason", env.BootReason)
	}
	if env.Platform != "" {
		args = append(args, "platform", env.Platform)
	}
	if env.Serial != "" {
		args = append(args, "serial", env.Serial)
	}
	if env.EthAddr != "" {
		args = append(args, "mac", env.EthAddr)
	}
	if env.DiscoIP != "" {
		args = append(args, "ip", env.DiscoIP)
	}
	if len(args) > 0 {
		slog.Info("ONIE env", args...)
	}

	if env.BootReason != "" && !slices.Contains(AllowedBootReasons, env.BootReason) {
		slog.Error("Not allowed ONIE boot reason, aborting", "reason", env.BootReason, "allowed", AllowedBootReasons)

		return fmt.Errorf("invalid ONIE boot reason") //nolint:goerr113
	}

	tmpDir := os.TempDir()
	tmpDirEntries, err := os.ReadDir(tmpDir)
	if err != nil {
		slog.Warn("Cannot read temp dir to cleanup", "dir", tmpDir, "error", err.Error())
	} else {
		for _, dirEntry := range tmpDirEntries {
			if strings.HasPrefix(dirEntry.Name(), "hh-install-") {
				dir := filepath.Join(tmpDir, dirEntry.Name())
				if err := os.RemoveAll(dir); err != nil {
					slog.Warn("Cannot remove temp dir", "dir", dir, "error", err.Error())
				}
			}
		}
	}

	tmp, err := os.MkdirTemp("", "hh-install-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}

	if !dryRun {
		defer func() {
			if err := os.RemoveAll(tmp); err != nil {
				slog.Warn("Cannot remove temp dir", "dir", tmp, "error", err.Error())
			}
		}()
	}

	if err := extractFiles(tmp); err != nil {
		return fmt.Errorf("extracting embedded files: %w", err)
	}

	if dryRun {
		slog.Info("Dry run, embedded files extracted, not actually running", "dir", tmp)

		return nil
	}

	defer func() {
		if funcErr != nil {
			slog.Error("Error during installation", "error", funcErr.Error())
			slog.Warn("Enforcing ONIE default boot entry")
			if err := uefiutil.MakeONIEDefaultBootEntryAndCleanup(); err != nil {
				slog.Error("Failed to enforce ONIE default boot entry", "error", err.Error())
			}
		}
	}()

	if err := EnsureONIEBootPartition(ctx); err != nil {
		return fmt.Errorf("ensuring ONIE boot partition: %w", err)
	}

	if err := runNOSInstaller(ctx, tmp); err != nil {
		return fmt.Errorf("running NOS installer: %w", err)
	}

	if err := installAgent(ctx, env, tmp); err != nil {
		return fmt.Errorf("installing agent: %w", err)
	}

	slog.Info("Installation complete")

	return nil
}

func extractFiles(dest string) error {
	slog.Info("Extracting embedded files", "dest", dest)

	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	binFile, err := os.Open(binPath)
	if err != nil {
		return fmt.Errorf("opening executable: %w", err)
	}
	defer binFile.Close()

	binStat, err := binFile.Stat()
	if err != nil {
		return fmt.Errorf("statting executable: %w", err)
	}

	magicBytes := make([]byte, len(Magic))
	if _, err := binFile.ReadAt(magicBytes, binStat.Size()-int64(len(Magic))); err != nil {
		return fmt.Errorf("reading magic: %w", err)
	}

	if string(magicBytes) != Magic {
		return fmt.Errorf("magic mismatch") //nolint:goerr113
	}

	payloadBytes := make([]byte, 8)
	if _, err := binFile.ReadAt(payloadBytes, binStat.Size()-int64(len(Magic))-8); err != nil {
		return fmt.Errorf("reading payload size: %w", err)
	}

	payloadSize := binary.BigEndian.Uint64(payloadBytes)
	if _, err := binFile.Seek(binStat.Size()-int64(len(Magic))-8-int64(payloadSize), 0); err != nil { //nolint:gosec
		return fmt.Errorf("seeking to payload start: %w", err)
	}

	tr := tar.NewReader(binFile)
	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return fmt.Errorf("reading tar header: %w", err)
		}

		if header == nil || header.Typeflag != tar.TypeReg {
			continue
		}

		if err := extractFile(dest, header, tr, os.FileMode(header.Mode)); err != nil { //nolint:gosec
			return fmt.Errorf("extracting file: %s: %w", header.Name, err)
		}
	}

	slog.Info("Embedded files extracted", "size", humanize.IBytes(payloadSize))

	return nil
}

func extractFile(dest string, header *tar.Header, r io.Reader, mode os.FileMode) error {
	target := filepath.Join(dest, header.Name) //nolint:gosec

	slog.Debug("Extracting file", "name", header.Name, "target", target, "size", humanize.IBytes(uint64(header.Size))) //nolint:gosec

	// path traversal check: https://security.snyk.io/research/zip-slip-vulnerability
	if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
		return fmt.Errorf("illegal file path %s", header.Name) //nolint:goerr113
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, mode)
	if err != nil {
		return fmt.Errorf("opening file %s: %w", target, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("writing file %s: %w", target, err)
	}

	return nil
}

func runNOSInstaller(ctx context.Context, tmp string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	nosPath := filepath.Join(tmp, NOSInstallerName)

	slog.Info("Executing NOS installer now...")

	nosCmd := exec.CommandContext(ctx, nosPath)
	nosCmd.Env = append(nosCmd.Environ(), "ZTP=n")
	nosCmd.Stderr = logutil.NewSink(ctx, slog.Info, "NOS: ")
	nosCmd.Stdout = logutil.NewSink(ctx, slog.Info, "NOS: ")
	if err := nosCmd.Run(); err != nil {
		return fmt.Errorf("NOS installer execution failed: %w", err)
	}

	slog.Info("NOS installer completed")

	return nil
}

func EnsureONIEBootPartition(ctx context.Context) error {
	_, err := os.Stat("/mnt/onie-boot/onie/grub.d")
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("statting /mnt/onie-boot/onie/grub.d: %w", err)
	}

	slog.Warn("ONIE boot partition seems to be not mounted, trying to mount now")

	cmd := exec.CommandContext(ctx, "mount", "LABEL=ONIE-BOOT", "-t", "ext4", "/mnt/onie-boot")
	cmd.Stderr = logutil.NewSink(ctx, slog.Info, "Mount ONIE-BOOT: ")
	cmd.Stdout = logutil.NewSink(ctx, slog.Info, "Mount ONIE-BOOT: ")

	if err := cmd.Run(); err != nil {
		slog.Warn("Mounting ONIE-BOOT failed", "error", err.Error())
	}

	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			slog.Info("Waiting for ONIE boot partition to be mounted")
			time.Sleep(1 * time.Second)
		}

		if _, err := os.Stat("/mnt/onie-boot"); err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("statting /mnt/onie-boot/onie/grub.d: %w", err)
		}
	}

	return fmt.Errorf("ONIE boot partition not mounted") //nolint:goerr113
}

func mountSONiCPartition(origCtx context.Context) (string, func(), error) {
	ctx, cancel := context.WithCancel(origCtx)
	defer cancel()

	slog.Info("Mounting SONiC partition")

	sonicRoot, err := os.MkdirTemp("", "hh-sonic-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir to mount sonic: %w", err)
	}

	slog.Debug("SONiC partition mount point", "path", sonicRoot)

	cmd := exec.CommandContext(ctx, "mount", "LABEL=SONiC-OS", "-t", "ext4", sonicRoot)
	cmd.Stderr = logutil.NewSink(ctx, slog.Info, "Mount SONiC-OS: ")
	cmd.Stdout = logutil.NewSink(ctx, slog.Info, "Mount SONiC-OS: ")

	if err := cmd.Run(); err != nil {
		return "", nil, fmt.Errorf("mounting SONiC partition: %w", err)
	}

	return sonicRoot, func() {
		ctx, cancel := context.WithCancel(origCtx)
		defer cancel()

		slog.Info("Unmounting SONiC partition")

		cmd := exec.CommandContext(ctx, "umount", sonicRoot)
		cmd.Stdout = logutil.NewSink(ctx, slog.Info, "Unmount: ")
		cmd.Stderr = logutil.NewSink(ctx, slog.Info, "Unmount: ")

		if err := cmd.Run(); err != nil {
			slog.Warn("SONiC partition unmount failed", "error", err.Error())
		} else {
			if err := os.RemoveAll(sonicRoot); err != nil {
				slog.Warn("Cannot remove SONiC partition mount point", "path", sonicRoot, "error", err.Error())
			}
		}
	}, nil
}

func installAgent(ctx context.Context, env Env, tmp string) error {
	sonicRoot, unmountSONiC, err := mountSONiCPartition(ctx)
	if err != nil {
		return fmt.Errorf("mounting SONiC partition: %w", err)
	}
	defer unmountSONiC()

	dirEntries, err := os.ReadDir(sonicRoot)
	if err != nil {
		return fmt.Errorf("reading sonic root dir: %w", err)
	}

	ok := false
	for _, dirEntry := range dirEntries {
		if strings.HasPrefix(dirEntry.Name(), "image-") {
			sonicRoot = filepath.Join(sonicRoot, dirEntry.Name(), "rw")
			ok = true

			break
		}
	}
	if !ok {
		return fmt.Errorf("finding SONiC image dir") //nolint:goerr113
	}

	if env.Platform == "x86_64-cls_ds5000-r0" {
		slog.Info("Checking for PDDF file patch")
		if changed, err := clsds5000.Patch(filepath.Join(sonicRoot, clsds5000.CfgPath)); err != nil {
			slog.Error("Failed to patch Celestica DS5000 switch pddf-device.json", "err", err)

			return fmt.Errorf("patching clsds5000: %w", err)
		} else if changed {
			slog.Info("Successfully patched Celestica DS5000 switch pddf-device.json, power cycle is required to apply the fix")
		}
	}

	slog.Info("Installing Fabric Agent binary")
	binDir := filepath.Join(sonicRoot, "/opt/hedgehog/bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("creating bin dir: %w", err)
	}
	if err := installFile(tmp, binDir, AgentBinaryName, 0o755); err != nil {
		return fmt.Errorf("installing agent binary: %w", err)
	}

	slog.Info("Installing Fabric Agent configs")
	confDir := filepath.Join(sonicRoot, "/etc/sonic/hedgehog")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return fmt.Errorf("creating agent conf dir: %w", err)
	}
	if err := installFile(tmp, confDir, AgentKubeConfigName, 0o600); err != nil {
		return fmt.Errorf("installing agent kubeconfig: %w", err)
	}
	if err := installFile(tmp, confDir, AgentConfigName, 0o600); err != nil {
		return fmt.Errorf("installing agent config: %w", err)
	}

	slog.Info("Installing Fabric Agent systemd unit")
	systemdPath := filepath.Join(sonicRoot, "/etc/systemd/system")
	if err := os.MkdirAll(systemdPath, 0o755); err != nil {
		return fmt.Errorf("creating systemd dir: %w", err)
	}
	if err := installFile(tmp, systemdPath, AgentUnitName, 0o644); err != nil {
		return fmt.Errorf("installing agent systemd unit: %w", err)
	}

	wantsPath := filepath.Join(sonicRoot, "/etc/systemd/system/multi-user.target.wants")
	if err := os.MkdirAll(wantsPath, 0o755); err != nil {
		return fmt.Errorf("creating systemd wants dir: %w", err)
	}
	if err := os.Symlink(filepath.Join(systemdPath, AgentUnitName), filepath.Join(wantsPath, AgentUnitName)); err != nil {
		return fmt.Errorf("symlinking agent systemd unit: %w", err)
	}

	return nil
}

func installFile(from, to, name string, mode os.FileMode) error {
	fromPath := filepath.Join(from, name)
	fromFile, err := os.Open(fromPath)
	if err != nil {
		return fmt.Errorf("opening source file %s: %w", fromPath, err)
	}
	defer fromFile.Close()

	toPath := filepath.Join(to, name)
	toFile, err := os.OpenFile(toPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("opening destination file %s: %w", toPath, err)
	}
	defer toFile.Close()

	if _, err := io.Copy(toFile, fromFile); err != nil {
		return fmt.Errorf("copying file: %w", err)
	}

	return nil
}
