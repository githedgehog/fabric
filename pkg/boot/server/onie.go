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

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/go-chi/chi/v5/middleware"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/ctrl"
	corev1 "k8s.io/api/core/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrNotFound = errors.New("not found")

func (svc *service) preCacheBackground(ctx context.Context) error {
	l := slog.With("background", "cacher")

	l.Info("Starting pre-caching")

	for nosType, nosVersion := range svc.cfg.NOSVersions {
		repo, ok := svc.cfg.NOSRepos[nosType]
		if !ok {
			return fmt.Errorf("NOS repo not found: %s", nosType) //nolint:goerr113
		}

		if _, err := svc.getCachedOrDownload(ctx, repo, nosVersion, true); err != nil {
			return fmt.Errorf("pre-caching NOS %s %s: %w", nosType, nosVersion, err)
		}
	}

	l.Info("NOS pre-caching done")

	for platform, version := range svc.cfg.ONIEPlatformVersions {
		repo, ok := svc.cfg.ONIERepos[platform]
		if !ok {
			return fmt.Errorf("ONIE repo not found: %s", platform) //nolint:goerr113
		}

		if _, err := svc.getCachedOrDownload(ctx, repo, version, true); err != nil {
			return fmt.Errorf("pre-caching ONIE %s %s: %w", platform, version, err)
		}
	}

	l.Info("ONIE pre-caching done")

	return nil
}

func (svc *service) handleONIE(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := slog.With("rid", middleware.GetReqID(ctx))

	if r.Header.Get("ONIE-ARCH") != "x86_64" {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	platform := r.Header.Get("ONIE-ARCH") + "-" + r.Header.Get("ONIE-MACHINE") + "-r" + r.Header.Get("ONIE-MACHINE-REV")

	l = l.With("platform", platform)

	op := r.Header.Get("ONIE-OPERATION")
	switch op {
	case "os-install":
		serial := wiringapi.NormalizeBootSerial(r.Header.Get("ONIE-SERIAL-NUMBER"))
		mac, err := wiringapi.NormalizeBootMAC(r.Header.Get("ONIE-ETH-ADDR"))
		if err != nil {
			l.Info("Invalid MAC", "error", err.Error())
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		if serial == "" && mac == "" {
			l.Info("Both serial and mac are empty")
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		l = l.With("serial", serial, "mac", mac)

		sb, agent, secret, err := svc.getAgentAndSecret(ctx, serial, mac)
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

		l.Info("NOS install", "switch", sb.Name)
		if err := svc.streamNOSInstaller(ctx, sb, agent, secret, w); err != nil {
			l.Error("Failed to stream nos-install", "switch", sb.Name, "error", err.Error())
			w.WriteHeader(http.StatusInternalServerError)

			return
		}
	case "onie-update":
		if platform == "" {
			l.Info("Platform is missing")
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		if svc.cfg.ONIERepos[platform] == "" || svc.cfg.ONIEPlatformVersions[platform] == "" {
			l.Info("ONIE not found")
			w.WriteHeader(http.StatusNotFound)
		}

		l.Info("ONIE update")
		if err := svc.streamONIEUpdater(ctx, platform, w); err != nil {
			l.Error("Failed to stream onie-updater", "platform", platform, "error", err.Error())
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusNotImplemented)
	default:
		w.WriteHeader(http.StatusBadRequest)

		return
	}
}

func (svc *service) getAgentAndSecret(ctx context.Context, serial, mac string) (*fcintapi.SwitchBoot, *agentapi.Agent, *corev1.Secret, error) {
	sb, err := findSwitchBoot(ctx, svc.boots, serial, mac)
	if err != nil {
		return nil, nil, nil, err
	}

	agent := &agentapi.Agent{}
	if err := svc.kube.Get(ctx, kclient.ObjectKey{Name: sb.Name, Namespace: sb.Namespace}, agent); err != nil {
		if kapierrors.IsNotFound(err) {
			return nil, nil, nil, fmt.Errorf("agent %s: %w", sb.Name, ErrNotFound)
		}

		return nil, nil, nil, fmt.Errorf("agent %s: %w", sb.Name, err)
	}

	secretName := ctrl.AgentKubeconfigSecret(sb.Name)
	agentSecret := &corev1.Secret{}
	if err := svc.kube.Get(ctx, kclient.ObjectKey{Name: secretName, Namespace: sb.Namespace}, agentSecret); err != nil {
		if kapierrors.IsNotFound(err) {
			return nil, nil, nil, fmt.Errorf("agent %s secret %s: %w", sb.Name, secretName, ErrNotFound)
		}

		return nil, nil, nil, fmt.Errorf("agent %s secret %s: %w", sb.Name, secretName, err)
	}

	return sb, agent, agentSecret, nil
}

func (svc *service) streamNOSInstaller(ctx context.Context, sb *fcintapi.SwitchBoot, agent *agentapi.Agent, secret *corev1.Secret, w io.Writer) error {
	agentConfig, err := agent.MarshalInstallConfig()
	if err != nil {
		return fmt.Errorf("marshaling agent: %w", err)
	}

	kubeConfig, ok := secret.Data[ctrl.AgentKubeconfigKey]
	if !ok {
		return fmt.Errorf("kubeconfig not found") //nolint:goerr113
	}

	nosType := sb.Spec.NOSType
	if nosType == "" || !slices.Contains(meta.NOSTypes, nosType) {
		return fmt.Errorf("invalid NOS type %q", nosType) //nolint:goerr113
	}

	nosRepo, ok := svc.cfg.NOSRepos[nosType]
	if !ok {
		return fmt.Errorf("NOS repo not found") //nolint:goerr113
	}

	nosVersion, ok := svc.cfg.NOSVersions[nosType]
	if !ok {
		return fmt.Errorf("NOS version not found") //nolint:goerr113
	}

	nosPath, err := svc.getCachedOrDownload(ctx, nosRepo, nosVersion, false)
	if err != nil {
		return fmt.Errorf("getting NOS: %w", err)
	}

	agentPath, err := svc.getCachedOrDownload(ctx, svc.cfg.AgentRef, sb.Spec.AgentVersion, false)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}

	//nolint:wrapcheck
	return (&nosinstall.Builder{
		NOSType:     nosType,
		AgentConfig: agentConfig,
		KubeConfig:  kubeConfig,
		NOSPath:     nosPath,
		AgentPath:   agentPath,
	}).Build(w)
}

func (svc *service) streamONIEUpdater(ctx context.Context, platform string, w io.Writer) error {
	repo, ok := svc.cfg.ONIERepos[platform]
	if !ok {
		return fmt.Errorf("onie-updater repo not found") //nolint:goerr113
	}

	version, ok := svc.cfg.ONIEPlatformVersions[platform]
	if !ok {
		return fmt.Errorf("onie-updater version not found") //nolint:goerr113
	}

	oniePath, err := svc.getCachedOrDownload(ctx, repo, version, false)
	if err != nil {
		return fmt.Errorf("getting onie-updater: %w", err)
	}

	onieF, err := os.Open(oniePath)
	if err != nil {
		return fmt.Errorf("opening onie-updater: %w", err)
	}
	defer onieF.Close()

	if _, err := io.Copy(w, onieF); err != nil {
		return fmt.Errorf("copying onie-updater: %w", err)
	}

	return nil
}
