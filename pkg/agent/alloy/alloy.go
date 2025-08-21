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

package alloy

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fmeta "go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/agent/common"
	"go.githedgehog.com/libmeta/pkg/tmpl"
)

const (
	unitName    = "hedgehog-alloy.service"
	binDir      = "/opt/hedgehog/bin"
	binName     = "alloy"
	listenPort  = 7043
	storagePath = "/var/lib/alloy"
	configPath  = "/etc/sonic/hedgehog/config.alloy"
)

type unitTemplateConf struct {
	User    string
	Binary  string
	Listen  string
	Storage string
	Config  string
}

//go:embed alloy.service.tmpl
var unitTemplate string

func EnsureInstalled(ctx context.Context, agent *agentapi.Agent) error {
	if agent.Spec.Version.AlloyRepo == "" || agent.Spec.Version.AlloyVersion == "" {
		return nil
	}

	start := time.Now()
	slog.Debug("Ensuring alloy is installed and running")

	binPath := filepath.Join(binDir, binName)
	unitPath := filepath.Join("/etc/systemd/system/", unitName)

	if _, err := osuser.Lookup(fmeta.AlloyUser); err != nil {
		if errors.Is(err, osuser.UnknownUserError(fmeta.AlloyUser)) {
			if err := execCmd(ctx, "useradd", "--no-create-home", "--shell", "/bin/false", "--groups", "adm", fmeta.AlloyUser); err != nil {
				return errors.Wrapf(err, "error creating alloy user %s", fmeta.AlloyUser)
			}
		} else {
			return errors.Wrapf(err, "error check looking up alloy user %s", fmeta.AlloyUser)
		}
	}

	alloyUser, err := osuser.Lookup(fmeta.AlloyUser)
	if err != nil {
		return errors.Wrapf(err, "error looking up alloy user %s", fmeta.AlloyUser)
	}

	alloyUserUID, err := strconv.Atoi(alloyUser.Uid)
	if err != nil {
		return errors.Wrapf(err, "error parsing alloy user UID %s", alloyUser.Uid)
	}

	if err := execCmd(ctx, "usermod", "--append", "--groups", "adm", fmeta.AlloyUser); err != nil {
		return errors.Wrapf(err, "error adding alloy user to adm group")
	}

	restart := false

	desiredConfig, err := agent.Spec.Config.Alloy.Render()
	if err != nil {
		return errors.Wrapf(err, "error executing config template")
	}

	actualConfig, err := os.ReadFile(configPath)
	if err == nil && string(desiredConfig) != string(actualConfig) || err != nil && os.IsNotExist(err) {
		restart = true
		if err := os.WriteFile(configPath, desiredConfig, 0o600); err != nil {
			return errors.Wrapf(err, "error writing config file")
		}
	} else if err != nil {
		return errors.Wrapf(err, "error reading config file")
	}

	if err := os.Chown(configPath, alloyUserUID, os.Getgid()); err != nil {
		return errors.Wrapf(err, "error changing config file ownership")
	}

	if err := os.MkdirAll(storagePath, 0o700); err != nil {
		return errors.Wrapf(err, "error creating storage directory")
	}

	if err := os.Chown(storagePath, alloyUserUID, os.Getgid()); err != nil {
		return errors.Wrapf(err, "error changing storage directory ownership")
	}

	needsBinUpgrade := false
	if _, err := os.Stat(binPath); errors.Is(err, os.ErrNotExist) {
		needsBinUpgrade = true
	} else if err == nil {
		vOut, err := exec.CommandContext(ctx, binPath, "--version").CombinedOutput()
		if err != nil {
			return errors.Wrapf(err, "error running alloy binary to get version")
		}

		if !strings.Contains(string(vOut), fmt.Sprintf("alloy, version %s (branch", agent.Spec.Version.AlloyVersion)) {
			needsBinUpgrade = true
		}
	} else {
		return errors.Wrapf(err, "error checking for alloy binary")
	}

	if needsBinUpgrade {
		if err := common.UpgradeBin(ctx,
			agent.Spec.Version.AlloyRepo,
			agent.Spec.Version.AlloyVersion,
			agent.Spec.Version.CA,
			agent.Spec.Version.Username,
			agent.Spec.Version.Password,
			binDir,
			binName,
			func(_ context.Context, _ string) error { return nil }); err != nil {
			return errors.Wrapf(err, "error installing alloy binary")
		}
	}

	ip, _, err := net.ParseCIDR(agent.Spec.Switch.IP)
	if err != nil {
		return errors.Wrapf(err, "error parsing switch IP")
	}

	desiredUnit, err := tmpl.Render("systemd-unit", unitTemplate, unitTemplateConf{
		User:    fmeta.AlloyUser,
		Binary:  binPath,
		Listen:  fmt.Sprintf("%s:%d", ip.String(), listenPort),
		Storage: storagePath,
		Config:  configPath,
	})
	if err != nil {
		return errors.Wrapf(err, "error executing unit template")
	}

	actualUnit, err := os.ReadFile(unitPath)
	if err == nil && string(desiredUnit) != string(actualUnit) || err != nil && os.IsNotExist(err) {
		restart = true
		if err := os.WriteFile(unitPath, desiredUnit, 0o644); err != nil { //nolint:gosec
			return errors.Wrapf(err, "error writing unit file")
		}

		if err := execCmd(ctx, "systemctl", "daemon-reload"); err != nil {
			return errors.Wrapf(err, "error reloading systemd")
		}
	} else if err != nil {
		return errors.Wrapf(err, "error reading unit file")
	}

	if len(agent.Spec.Config.Alloy.Targets.Prometheus) > 0 || len(agent.Spec.Config.Alloy.Targets.Loki) > 0 {
		if err := execCmd(ctx, "systemctl", "enable", unitName); err != nil {
			return errors.Wrapf(err, "error enabling unit")
		}

		cmd := "start"
		if restart {
			cmd = "restart"
		}
		if err := execCmd(ctx, "systemctl", cmd, unitName); err != nil {
			return errors.Wrapf(err, "error starting unit")
		}
	} else {
		if err := execCmd(ctx, "systemctl", "disable", unitName); err != nil {
			return errors.Wrapf(err, "error disabling unit")
		}

		if err := execCmd(ctx, "systemctl", "stop", unitName); err != nil {
			return errors.Wrapf(err, "error stopping unit")
		}
	}

	slog.Debug("Alloy ensured", "took", time.Since(start))

	return nil
}

func execCmd(ctx context.Context, name string, arg ...string) error {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	return errors.Wrapf(cmd.Run(), "error running '%s %s'", name, strings.Join(arg, " "))
}
