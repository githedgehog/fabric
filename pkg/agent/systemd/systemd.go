package systemd

import (
	"bytes"
	"os"
	"text/template"
)

var unitTmpl = `
[Unit]
Description=Hedgehog Agent

[Service]
User={{ .User }}
ExecStart={{ .BinPath }} start
Environment=KUBECONFIG=/etc/sonic/hedgehog/agent-kubeconfig

Restart=always
RestartSec=2

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
}

func Generate(cfg UnitConfig) (string, error) {
	t, err := template.New("unit").Parse(unitTmpl[1 : len(unitTmpl)-1])
	if err != nil {
		return "", err
	}

	var unit bytes.Buffer
	err = t.Execute(os.Stdout, cfg)
	if err != nil {
		return "", err
	}

	return unit.String(), nil
}
