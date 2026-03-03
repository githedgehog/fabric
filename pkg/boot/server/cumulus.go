// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"text/template"

	_ "embed"

	"github.com/go-chi/chi/v5/middleware"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl"
	corev1 "k8s.io/api/core/v1"
)

func (svc *service) handleAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := slog.With("rid", middleware.GetReqID(ctx))

	if err := svc.streamAgentBinary(ctx, w); err != nil {
		l.Error("Streaming agent bin", "err", err)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}
}

func (svc *service) streamAgentBinary(ctx context.Context, w io.Writer) error {
	agentPath, err := svc.getCachedOrDownload(ctx, svc.cfg.AgentRef, svc.cfg.AgentVersion, false)
	if err != nil {
		return fmt.Errorf("getting agent bin path: %w", err)
	}

	agentF, err := os.Open(agentPath)
	if err != nil {
		return fmt.Errorf("opening agent bin file: %w", err)
	}
	defer agentF.Close()

	if _, err := io.Copy(w, agentF); err != nil {
		return fmt.Errorf("copying agent bin file: %w", err)
	}

	return nil
}

func (svc *service) handleCumulusZTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := slog.With("rid", middleware.GetReqID(ctx))

	if r.Header.Get("CUMULUS-ARCH") != "x86_64" {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	serial := strings.TrimSpace(r.Header.Get("CUMULUS-SERIAL"))
	mac := strings.TrimSpace(r.Header.Get("CUMULUS-MGMT-MAC"))

	if serial == "" && mac == "" {
		l.Info("Both serial and mac are empty")
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	l = l.With("serial", serial, "mac", mac)

	agent, secret, err := svc.getAgentAndSecret(ctx, serial, mac)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			l.Info("NOS not found", "error", err.Error())
			w.WriteHeader(http.StatusNotFound)

			return
		}

		l.Error("Failed to get switch", "error", err.Error())
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	l = l.With("switch", agent.Name)

	l.Info("Cumulus ZTP")

	if err := svc.writeCumulusZTP(w, agent, secret); err != nil {
		l.Error("Failed to write response", "error", err.Error())
		w.WriteHeader(http.StatusInternalServerError)

		return
	}
}

func (svc *service) writeCumulusZTP(w http.ResponseWriter, agent *agentapi.Agent, secret *corev1.Secret) error {
	kubeConfig, ok := secret.Data[ctrl.AgentKubeconfigKey]
	if !ok {
		return fmt.Errorf("kubeconfig not found") //nolint:err113
	}

	controlVIP, err := netip.ParsePrefix(svc.cfg.ControlVIP)
	if err != nil {
		return fmt.Errorf("parsing control VIP: %w", err)
	}

	users := []CmlsUser{}
	for _, user := range agent.Spec.Users {
		role := ""
		switch user.Role {
		case "admin":
			role = "system-admin"
		case "operator":
			role = "nvue-monitor"
		}

		if role == "" {
			return fmt.Errorf("invalid role: %s", user.Role) //nolint:err113
		}

		keys := []CmlsSSHKey{}
		for _, key := range user.SSHKeys {
			parts := strings.Split(key, " ")
			if len(parts) < 2 {
				return fmt.Errorf("invalid SSH key: %s", key) //nolint:err113
			}

			keys = append(keys, CmlsSSHKey{
				Key:  parts[1],
				Type: parts[0],
			})
		}

		users = append(users, CmlsUser{
			Name:           user.Name,
			HashedPassword: user.Password,
			Role:           role,
			SSHKeys:        keys,
		})
	}

	cfgIn := CmlsConfigIn{
		Hostname:     agent.Name,
		ManagementIP: agent.Spec.Switch.IP,
		Users:        users,
		NTPServer:    controlVIP.Addr().String(),
	}

	cfgTmpl, err := template.New("cumulus_config").Parse(cumulusConfigTemplate)
	if err != nil {
		return fmt.Errorf("parsing config template: %w", err)
	}

	cfgBuf := &bytes.Buffer{}
	if err := cfgTmpl.Execute(cfgBuf, cfgIn); err != nil {
		return fmt.Errorf("executing config template: %w", err)
	}

	ztpIn := CumulusZTPIn{
		ControlVIP:    controlVIP.Addr().String(),
		InitialConfig: cfgBuf.String(),
	}

	ztpTmpl, err := template.New("cumulus_ztp").Parse(cumulusZTPTemplate)
	if err != nil {
		return fmt.Errorf("parsing ztp template: %w", err)
	}

	ztpBuf := &bytes.Buffer{}
	if err := ztpTmpl.Execute(ztpBuf, ztpIn); err != nil {
		return fmt.Errorf("executing ztp template: %w", err)
	}

	if _, err := io.Copy(w, ztpBuf); err != nil {
		return fmt.Errorf("writing ztp script: %w", err)
	}

	_ = kubeConfig

	return nil
}

//go:embed cumulus_config.tmpl.yaml
var cumulusConfigTemplate string

type CmlsConfigIn struct {
	Hostname     string
	ManagementIP string
	Users        []CmlsUser
	NTPServer    string
}

type CmlsUser struct {
	Name           string
	HashedPassword string
	Role           string
	SSHKeys        []CmlsSSHKey
}

type CmlsSSHKey struct {
	Key  string
	Type string
}

//go:embed cumulus_ztp.tmpl.sh
var cumulusZTPTemplate string

type CumulusZTPIn struct {
	ControlVIP    string
	InitialConfig string
}
