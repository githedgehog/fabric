// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls"
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

	ztpBuf, err := cmls.BuildZTPFor(agent, kubeConfig)
	if err != nil {
		return fmt.Errorf("building ztp: %w", err)
	}

	if _, err := io.Copy(w, ztpBuf); err != nil {
		return fmt.Errorf("writing ztp script: %w", err)
	}

	return nil
}
