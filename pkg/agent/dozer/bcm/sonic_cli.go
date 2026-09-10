// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const (
	// sonic-cli refuses to run as root ("FATAL: root cannot launch CLI") and the agent runs as root,
	// so every invocation has to drop to a regular user.
	SonicCLIUser = "admin"

	sonicCLITimeout = 2 * time.Minute
)

// SonicCLI runs a sonic-cli command.
func SonicCLI(ctx context.Context, command string) error {
	ctx, cancel := context.WithTimeout(ctx, sonicCLITimeout)
	defer cancel()

	return runSonicCLI(ctx, command, exec.CommandContext(ctx,
		"sudo", "-u", SonicCLIUser, "sonic-cli", "-c", command))
}

// SonicCLIConfirm runs a sonic-cli command that prompts for confirmation, answering it with answer
// (such as "y\n"). The prompt is read from the controlling TTY, not stdin, so a plain stdin pipe is
// ignored and the command defaults to N. `script` allocates a pty and forwards its own stdin into
// it, so the answer written here reaches the prompt.
//
// Under a pty sonic-cli also paginates, so a command with long output stops at "--more--" instead of
// completing: append "| no-more" to the command to disable paging.
func SonicCLIConfirm(ctx context.Context, command, answer string) error {
	ctx, cancel := context.WithTimeout(ctx, sonicCLITimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "script", "-qec", //nolint:gosec
		fmt.Sprintf("sudo -u %s sonic-cli -c %q", SonicCLIUser, command), "/dev/null")
	cmd.Stdin = strings.NewReader(answer)

	return runSonicCLI(ctx, command, cmd)
}

// runSonicCLI returns the command output together with the error, so that a failure is diagnosable
// from the single log line the caller reports.
func runSonicCLI(ctx context.Context, command string, cmd *exec.Cmd) error {
	out := &bytes.Buffer{}
	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Run()
	output := strings.TrimSpace(out.String())

	if err == nil {
		if output != "" {
			slog.Debug("sonic-cli", "command", command, "output", output)
		}

		return nil
	}

	// on the deadline the process is killed, which on its own only surfaces as "signal: killed"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("%w (timed out after %s)", err, sonicCLITimeout)
	}

	if output == "" {
		return fmt.Errorf("running sonic-cli %q: %w", command, err)
	}

	return fmt.Errorf("running sonic-cli %q: %w: %s", command, err, output)
}
