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

package systemd

import (
	"bytes"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/pkg/errors"
)

var switchUnitTmpl = `
[Unit]
Description=Hedgehog Agent

[Service]
User={{ .User }}
ExecStart={{ .BinPath }} start

Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

var agentUnitTmpl = `
[Unit]
Description=Hedgehog Control Agent
Wants=k3s.service
After=k3s.service

[Service]
User={{ .User }}
ExecStart={{ .BinPath }} control start

Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

// TODO identify better deps and wantedby if available
// Requires=database.service and After=database.service potentially makes sense as it probably doesn't make any sense
// to start agent without database available. On the other hand, for some recovery scenarious it could be helpful.
// Should we use WantedBy=sonic.target instead of multi-user.target? Agent is potentially closest to the
// database.service which is using multi-user.target.
// TODO think about RestartSec and StartLimitIntervalSec=1200 StartLimitBurst=3, we probably shouldn't limit agent and
// it should just restart every couple seconds

type UnitConfig struct {
	BinPath string
	User    string
	Control bool
}

func Generate(cfg UnitConfig) (string, error) {
	tmpl := switchUnitTmpl[1 : len(switchUnitTmpl)-1]
	if cfg.Control {
		tmpl = agentUnitTmpl[1 : len(agentUnitTmpl)-1]
	}

	t, err := template.New("unit").Parse(tmpl)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse template")
	}

	unit := bytes.NewBuffer(nil)
	err = t.Execute(unit, cfg)
	if err != nil {
		return "", errors.Wrapf(err, "failed to execute template")
	}

	return unit.String(), nil
}

func Install(cfg UnitConfig) error {
	unit := "hedgehog-agent.service"

	slog.Info("Installing", "unit", unit, "config", cfg)

	unitContent, err := Generate(cfg)
	if err != nil {
		return errors.Wrapf(err, "failed to generate %s", unit)
	}

	err = os.WriteFile("/etc/systemd/system/"+unit, []byte(unitContent), 0o644) //nolint:gosec
	if err != nil {
		return errors.Wrapf(err, "failed to write unit %s", unit)
	}

	err = run("systemctl", "daemon-reload")
	if err != nil {
		return errors.Wrapf(err, "failed to reload systemd")
	}

	err = run("systemctl", "enable", unit)
	if err != nil {
		return errors.Wrapf(err, "failed to enable unit %s", unit)
	}

	err = run("systemctl", "start", unit)
	if err != nil {
		return errors.Wrapf(err, "failed to start %s", unit)
	}

	return nil
}

func run(command string, args ...string) error {
	slog.Debug("Running", "command", command, "args", strings.Join(args, " "))

	cmd := exec.Command(command, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return errors.Wrapf(cmd.Run(), "failed to run %s %s", command, strings.Join(args, " "))
}
