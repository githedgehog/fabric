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
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/boot"
	ni "go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	"golang.org/x/sync/singleflight"
	corev1 "k8s.io/api/core/v1"
	"oras.land/oras-go/v2/registry/remote/auth"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	ListenPort = 32000
	ConfigPath = "/config/config.yaml"
	CAPath     = "/ca/ca.crt"
	CredsPath  = "/creds/" + corev1.DockerConfigJsonKey
	CacheDir   = "/cache/v1"
)

type service struct {
	cfg          *boot.ServerConfig
	kube         client.WithWatch
	cacheDir     string
	orasClient   *auth.Client
	downloadLock *sync.Mutex
	sf           *singleflight.Group
}

func Run(ctx context.Context) error {
	// TODO think about cache cleanup

	if err := os.MkdirAll(CacheDir, 0o755); err != nil {
		return errors.Wrapf(err, "creating cache dir %s", CacheDir)
	}

	// TODO we probably need to cache client? only for switches and secrets in default namespace?
	kube, err := kubeutil.NewClientWithCore(ctx, "", agentapi.SchemeBuilder, wiringapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	orasClient, err := newORASClient(CredsPath, CAPath)
	if err != nil {
		return fmt.Errorf("creating ORAS client: %w", err)
	}

	configData, err := os.ReadFile(ConfigPath)
	if err != nil {
		return fmt.Errorf("reading config file %s: %w", ConfigPath, err)
	}

	cfg := &boot.ServerConfig{}
	if err := yaml.UnmarshalStrict(configData, cfg); err != nil {
		return fmt.Errorf("unmarshalling config: %w", err)
	}
	if cfg.ControlVIP == "" {
		return errors.New("ControlVIP is required")
	}
	controlVIP, err := netip.ParsePrefix(cfg.ControlVIP)
	if err != nil {
		return fmt.Errorf("parsing ControlVIP: %w", err)
	}
	if cfg.NOSRepos == nil {
		cfg.NOSRepos = map[meta.NOSType]string{}
	}
	if cfg.ONIERepos == nil {
		cfg.ONIERepos = map[string]string{}
	}
	if cfg.NOSVersions == nil {
		cfg.NOSVersions = map[meta.NOSType]string{}
	}
	if cfg.ONIEPlatformVersions == nil {
		cfg.ONIEPlatformVersions = map[string]string{}
	}

	svc := &service{
		cfg:          cfg,
		kube:         kube,
		cacheDir:     CacheDir,
		orasClient:   orasClient,
		downloadLock: &sync.Mutex{},
		sf:           &singleflight.Group{},
	}

	go func() {
		if err := svc.preCacheBackground(ctx); err != nil {
			slog.Error("Failed to pre-cache", "error", err)
			os.Exit(1)
		}
	}()

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(RequestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(ResponseRequestID)
	r.Use(middleware.Heartbeat("/healthz"))
	r.Use(middleware.Timeout(300 * time.Second))

	// TODO what should be a correct number?
	r.With(middleware.Throttle(20)).Get(ni.OnieURLSuffix, svc.handleONIE)

	// TODO do we need to throttle it as well?
	r.Post(ni.LogURLSuffix, svc.handleLog)

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", controlVIP.Addr(), ListenPort),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       90 * time.Second,
		Handler:           r,
	}
	if err := srv.ListenAndServe(); err != nil {
		return errors.Wrapf(err, "error running server")
	}

	return nil
}
