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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/ctrl"
	corev1 "k8s.io/api/core/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kyaml "sigs.k8s.io/yaml"
)

var ErrNotFound = errors.New("not found")

func (svc *service) preCacheBackground(ctx context.Context) error {
	l := slog.With("background", "cacher")

	l.Info("Starting pre-caching")

	for nosType, nosVersion := range svc.cfg.NOSVersions {
		repo, ok := svc.cfg.NOSRepos[nosType]
		if !ok {
			return fmt.Errorf("NOS repo not found: %s", nosType) //nolint:err113
		}

		if _, err := svc.getCachedOrDownload(ctx, repo, nosVersion, true); err != nil {
			return fmt.Errorf("pre-caching NOS %s %s: %w", nosType, nosVersion, err)
		}
	}

	l.Info("NOS pre-caching done")

	for platform, version := range svc.cfg.ONIEPlatformVersions {
		repo, ok := svc.cfg.ONIERepos[platform]
		if !ok {
			return fmt.Errorf("ONIE repo not found: %s", platform) //nolint:err113
		}

		if _, err := svc.getCachedOrDownload(ctx, repo, version, true); err != nil {
			return fmt.Errorf("pre-caching ONIE %s %s: %w", platform, version, err)
		}
	}

	l.Info("ONIE pre-caching done")

retry:
	for ctx.Err() == nil {
		w, err := svc.kube.Watch(ctx, &agentapi.AgentList{})
		if err != nil {
			l.Error("Failed to watch agents", "error", err.Error())

			continue
		}
		defer w.Stop()

		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context done: %w", ctx.Err())
			case event, ok := <-w.ResultChan():
				if !ok || event.Object == nil {
					l.Warn("Watch channel closed, retrying")

					continue retry
				}

				if event.Type == watch.Error {
					l.Warn("Watch error, retrying", "error", event.Object)

					continue retry
				}

				if event.Type == watch.Added || event.Type == watch.Modified || event.Type == watch.Bookmark {
					agent, ok := event.Object.(*agentapi.Agent)
					if !ok {
						l.Warn("Failed to cast agent", "object", event.Object)

						continue
					}

					agentRepo := agent.Spec.Version.Repo
					agentVersion := agent.Spec.Version.Default
					if agent.Spec.Version.Override != "" {
						agentVersion = agent.Spec.Version.Override
					}

					if _, err := svc.getCachedOrDownload(ctx, agentRepo, agentVersion, true); err != nil {
						return fmt.Errorf("pre-caching agent %s %s: %w", agentRepo, agentVersion, err)
					}
				}
			}
		}
	}

	return fmt.Errorf("context done (pre-cache finished): %w", ctx.Err())
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
		serial := strings.TrimSpace(r.Header.Get("ONIE-SERIAL-NUMBER"))
		mac := strings.TrimSpace(r.Header.Get("ONIE-ETH-ADDR"))

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

		l.Info("NOS install")
		if err := svc.streamNOSInstaller(ctx, agent, secret, w); err != nil {
			l.Error("Failed to stream nos-install", "switch", agent.Name, "error", err.Error())
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

func (svc *service) getAgentAndSecret(ctx context.Context, serial, mac string) (*agentapi.Agent, *corev1.Secret, error) {
	serial = strings.TrimSpace(serial)
	mac = strings.TrimSpace(mac)

	if serial != "" || mac != "" {
		switches := &wiringapi.SwitchList{}
		if err := svc.kube.List(ctx, switches, kclient.InNamespace(kmetav1.NamespaceDefault)); err != nil {
			return nil, nil, fmt.Errorf("listing switches: %w", err)
		}

		for _, sw := range switches.Items {
			bootSerial := strings.TrimSpace(sw.Spec.Boot.Serial)
			bootMAC := strings.TrimSpace(sw.Spec.Boot.MAC)

			if serial != "" && strings.EqualFold(bootSerial, serial) || mac != "" && strings.EqualFold(bootMAC, mac) {
				agent := &agentapi.Agent{}
				if err := svc.kube.Get(ctx, kclient.ObjectKey{Name: sw.Name, Namespace: sw.Namespace}, agent); err != nil {
					if kapierrors.IsNotFound(err) {
						return nil, nil, fmt.Errorf("agent %s: %w", sw.Name, ErrNotFound)
					}

					return nil, nil, fmt.Errorf("agent %s: %w", sw.Name, err)
				}

				secretName := ctrl.AgentKubeconfigSecret(sw.Name)
				agentSecret := &corev1.Secret{}
				if err := svc.kube.Get(ctx, kclient.ObjectKey{Name: secretName, Namespace: agent.Namespace}, agentSecret); err != nil {
					if kapierrors.IsNotFound(err) {
						return nil, nil, fmt.Errorf("agent %s secret %s: %w", sw.Name, secretName, ErrNotFound)
					}

					return nil, nil, fmt.Errorf("agent %s secret %s: %w", sw.Name, secretName, err)
				}

				return agent, agentSecret, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("switch: %w", ErrNotFound)
}

func (svc *service) streamNOSInstaller(ctx context.Context, agent *agentapi.Agent, secret *corev1.Secret, w io.Writer) error {
	agent.SetGroupVersionKind(agentapi.GroupVersion.WithKind(agentapi.KindAgent))
	agentConfig, err := kyaml.Marshal(agent)
	if err != nil {
		return fmt.Errorf("marshaling agent: %w", err)
	}

	kubeConfig, ok := secret.Data[ctrl.AgentKubeconfigKey]
	if !ok {
		return fmt.Errorf("kubeconfig not found") //nolint:err113
	}

	if agent.Spec.SwitchProfile == nil {
		return fmt.Errorf("switch profile is missing") //nolint:err113
	}

	nosType := agent.Spec.SwitchProfile.NOSType
	if nosType == "" || !slices.Contains(meta.NOSTypes, nosType) {
		return fmt.Errorf("invalid NOS type") //nolint:err113
	}

	nosRepo, ok := svc.cfg.NOSRepos[nosType]
	if !ok {
		return fmt.Errorf("NOS repo not found") //nolint:err113
	}

	nosVersion, ok := svc.cfg.NOSVersions[nosType]
	if !ok {
		return fmt.Errorf("NOS version not found") //nolint:err113
	}

	nosPath, err := svc.getCachedOrDownload(ctx, nosRepo, nosVersion, false)
	if err != nil {
		return fmt.Errorf("getting NOS: %w", err)
	}

	agentRepo := agent.Spec.Version.Repo
	agentVersion := agent.Spec.Version.Default
	if agent.Spec.Version.Override != "" {
		agentVersion = agent.Spec.Version.Override
	}

	agentPath, err := svc.getCachedOrDownload(ctx, agentRepo, agentVersion, false)
	if err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}

	//nolint:wrapcheck
	return (&nosinstall.Builder{
		AgentConfig: agentConfig,
		KubeConfig:  kubeConfig,
		NOSPath:     nosPath,
		AgentPath:   agentPath,
	}).Build(w)
}

func (svc *service) streamONIEUpdater(ctx context.Context, platform string, w io.Writer) error {
	repo, ok := svc.cfg.ONIERepos[platform]
	if !ok {
		return fmt.Errorf("onie-updater repo not found") //nolint:err113
	}

	version, ok := svc.cfg.ONIEPlatformVersions[platform]
	if !ok {
		return fmt.Errorf("onie-updater version not found") //nolint:err113
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
