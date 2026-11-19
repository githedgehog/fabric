// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/pkg/agent/cmls"
	"go.githedgehog.com/fabric/pkg/ctrl"
	corev1 "k8s.io/api/core/v1"
)

func (svc *service) handleAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := slog.With("rid", middleware.GetReqID(ctx))

	version := r.URL.Query().Get("version")
	// it's a part of the cache path
	if version == "" || strings.Contains(version, "..") || strings.ContainsAny(version, `/\`) {
		l.Info("Agent version is missing or invalid", "version", version)
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	l = l.With("version", version)

	if err := svc.streamAgentBinary(ctx, version, w); err != nil {
		l.Error("Streaming agent bin", "err", err)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}
}

func (svc *service) streamAgentBinary(ctx context.Context, version string, w io.Writer) error {
	agentPath, err := svc.getCachedOrDownload(ctx, svc.cfg.AgentRef, version, false)
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

	if r.Header.Get("Cumulus-Arch") != "x86_64" {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	serial, mac := normalizeBootIDs(l, r.Header.Get("Cumulus-Serial"), r.Header.Get("Cumulus-Mgmt-Mac"))
	if serial == "" && mac == "" {
		l.Info("Both serial and mac are empty or invalid")
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	l = l.With("serial", serial, "mac", mac)

	sb, err := findSwitchBoot(ctx, svc.boots, serial, mac)
	if err != nil {
		writeSwitchError(l, w, err)

		return
	}

	l = l.With("switch", sb.Name)

	attempt := newBootAttempt(ctx, fcintapi.SwitchBootAttemptTypeCumulusZTP)
	defer func() { svc.recordBootAttempt(ctx, l, sb, attempt) }()

	agent, secret, err := svc.getAgentAndSecret(ctx, sb)
	if err != nil {
		writeSwitchError(l, w, err)

		return
	}

	// the script downloads the agent right away, so it's only served once the agent is pre-cached
	if _, err := svc.getCachedOrDownload(ctx, svc.cfg.AgentRef, sb.Spec.AgentVersion, false); err != nil {
		l.Info("Agent not pre-cached yet", "version", sb.Spec.AgentVersion, "error", err.Error())
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusServiceUnavailable)
		attempt.Result = fcintapi.SwitchBootAttemptResultAgentNotCached

		return
	}

	l.Info("Cumulus ZTP")

	if err := svc.writeCumulusZTP(w, sb, agent, secret); err != nil {
		l.Error("Failed to write response", "error", err.Error())
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	attempt.Result = fcintapi.SwitchBootAttemptResultServed
}

func (svc *service) writeCumulusZTP(w http.ResponseWriter, sb *fcintapi.SwitchBoot, agent *agentapi.Agent, secret *corev1.Secret) error {
	kubeConfig, ok := secret.Data[ctrl.AgentKubeconfigKey]
	if !ok {
		return fmt.Errorf("kubeconfig not found") //nolint:err113
	}

	ztpBuf, err := cmls.BuildZTPFor(agent, kubeConfig, sb.Spec.AgentVersion)
	if err != nil {
		return fmt.Errorf("building ztp: %w", err)
	}

	if _, err := io.Copy(w, ztpBuf); err != nil {
		return fmt.Errorf("writing ztp script: %w", err)
	}

	return nil
}
