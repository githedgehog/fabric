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
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"
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
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/agent/common"
)

const (
	userName    = "alloy"
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

type alloyConfigTemplateConf struct {
	meta.AlloyConfig

	AgentExporterPort uint16
	Hostname          string
	PrometheusEnabled bool
	LokiEnabled       bool
	ProxyURL          string
}

//go:embed config.alloy.tmpl
var alloyConfigTemplate string

func EnsureInstalled(ctx context.Context, agent *agentapi.Agent, agentExporterPort uint16) error {
	if agent.Spec.Version.AlloyRepo == "" || agent.Spec.Version.AlloyVersion == "" {
		return nil
	}

	start := time.Now()
	slog.Debug("Ensuring alloy is installed and running")

	agent.Spec.Alloy.Default()
	binPath := filepath.Join(binDir, binName)
	unitPath := filepath.Join("/etc/systemd/system/", unitName)

	if _, err := osuser.Lookup(userName); err != nil {
		if errors.Is(err, osuser.UnknownUserError(userName)) {
			if err := execCmd(ctx, "useradd", "--no-create-home", "--shell", "/bin/false", userName); err != nil {
				return errors.Wrapf(err, "error creating alloy user %s", userName)
			}
		} else {
			return errors.Wrapf(err, "error check looking up alloy user %s", userName)
		}
	}

	alloyUser, err := osuser.Lookup(userName)
	if err != nil {
		return errors.Wrapf(err, "error looking up alloy user %s", userName)
	}

	alloyUserUID, err := strconv.Atoi(alloyUser.Uid)
	if err != nil {
		return errors.Wrapf(err, "error parsing alloy user UID %s", alloyUser.Uid)
	}

	restart := false

	desiredConfig, err := executeTemplate(alloyConfigTemplate, alloyConfigTemplateConf{
		AlloyConfig:       agent.Spec.Alloy,
		AgentExporterPort: agentExporterPort,
		Hostname:          agent.Name,
		PrometheusEnabled: len(agent.Spec.Alloy.PrometheusTargets) > 0,
		LokiEnabled:       len(agent.Spec.Alloy.LokiTargets) > 0,
		ProxyURL:          agent.Spec.Alloy.ControlProxyURL,
	})
	if err != nil {
		return errors.Wrapf(err, "error executing config template")
	}

	actualConfig, err := os.ReadFile(configPath)
	if err == nil && desiredConfig != string(actualConfig) || err != nil && os.IsNotExist(err) {
		restart = true
		if err := os.WriteFile(configPath, []byte(desiredConfig), 0o600); err != nil {
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
		if err := common.UpgradeBin(ctx, agent.Spec.Version.AlloyRepo, agent.Spec.Version.AlloyVersion, agent.Spec.Version.CA, binDir, binName,
			func(_ context.Context, _ string) error { return nil }); err != nil {
			return errors.Wrapf(err, "error installing alloy binary")
		}
	}

	ip, _, err := net.ParseCIDR(agent.Spec.Switch.IP)
	if err != nil {
		return errors.Wrapf(err, "error parsing switch IP")
	}

	desiredUnit, err := executeTemplate(unitTemplate, unitTemplateConf{
		User:    userName,
		Binary:  binPath,
		Listen:  fmt.Sprintf("%s:%d", ip.String(), listenPort),
		Storage: storagePath,
		Config:  configPath,
	})
	if err != nil {
		return errors.Wrapf(err, "error executing unit template")
	}

	actualUnit, err := os.ReadFile(unitPath)
	if err == nil && desiredUnit != string(actualUnit) || err != nil && os.IsNotExist(err) {
		restart = true
		if err := os.WriteFile(unitPath, []byte(desiredUnit), 0o644); err != nil { //nolint:gosec
			return errors.Wrapf(err, "error writing unit file")
		}

		if err := execCmd(ctx, "systemctl", "daemon-reload"); err != nil {
			return errors.Wrapf(err, "error reloading systemd")
		}
	} else if err != nil {
		return errors.Wrapf(err, "error reading unit file")
	}

	if len(agent.Spec.Alloy.PrometheusTargets) > 0 || len(agent.Spec.Alloy.LokiTargets) > 0 {
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

func executeTemplate(tmplText string, data any) (string, error) {
	tmplText = strings.TrimPrefix(tmplText, "\n")
	tmplText = strings.TrimSpace(tmplText)

	tmpl, err := template.New("tmpl").Parse(tmplText)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing template")
	}

	buf := bytes.NewBuffer(nil)
	err = tmpl.Execute(buf, data)
	if err != nil {
		return "", errors.Wrapf(err, "error executing template")
	}

	return buf.String(), nil
}

func execCmd(ctx context.Context, name string, arg ...string) error {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	return errors.Wrapf(cmd.Run(), "error running '%s %s'", name, strings.Join(arg, " "))
}
