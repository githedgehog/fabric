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
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/boot"
	ni "go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	"golang.org/x/sync/singleflight"
	corev1 "k8s.io/api/core/v1"
	"oras.land/oras-go/v2/registry/remote/auth"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kyaml "sigs.k8s.io/yaml"
)

const (
	ListenPort = 32000
	ConfigPath = "/config/config.yaml"
	CAPath     = "/ca/ca.crt"
	CredsPath  = "/creds/" + corev1.DockerConfigJsonKey
	CacheDir   = "/cache/v1"

	AgentURLSuffix      = "/agent"
	CumulusZTPURLSuffix = "/cumulus/ztp"
)

type service struct {
	cfg          *boot.ServerConfig
	kube         kclient.Client // uncached, to get the agent and its secret for a switch being installed and record its boot attempts
	boots        kclient.Reader // cached switch boots, see startSwitchBootCache
	cacheDir     string
	orasClient   *auth.Client
	downloadLock *sync.Mutex
	sf           *singleflight.Group
	bootAttempts chan func() // boot attempts to record, see recordBootAttempt
}

func Run(ctx context.Context) error {
	// TODO think about cache cleanup

	if err := os.MkdirAll(CacheDir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir %s: %w", CacheDir, err)
	}

	restCfg, err := kubeutil.NewClientConfig(ctx, "")
	if err != nil {
		return fmt.Errorf("creating kube config: %w", err)
	}

	scheme, err := kubeutil.NewScheme(corev1.AddToScheme, agentapi.AddToScheme, fcintapi.AddToScheme)
	if err != nil {
		return fmt.Errorf("creating scheme: %w", err)
	}

	kube, err := kclient.New(restCfg, kclient.Options{Scheme: scheme})
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
	if err := kyaml.UnmarshalStrict(configData, cfg); err != nil {
		return fmt.Errorf("unmarshalling config: %w", err)
	}
	if cfg.ControlVIP == "" {
		return errors.New("ControlVIP is required") //nolint:err113
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

	if cfg.AgentRef == "" {
		return errors.New("AgentRef is required") //nolint:err113
	}

	svc := &service{
		cfg:          cfg,
		kube:         kube,
		cacheDir:     CacheDir,
		orasClient:   orasClient,
		downloadLock: &sync.Mutex{},
		sf:           &singleflight.Group{},
		bootAttempts: make(chan func(), bootAttemptQueue),
	}

	for range bootAttemptWriters {
		go func() {
			for write := range svc.bootAttempts {
				write()
			}
		}()
	}

	boots, err := svc.startSwitchBootCache(ctx, restCfg, scheme)
	if err != nil {
		return fmt.Errorf("starting switch boot cache: %w", err)
	}
	svc.boots = boots

	go func() {
		if err := svc.preCacheBackground(ctx); err != nil {
			slog.Error("Failed to pre-cache", "error", err)
			os.Exit(1)
		}
	}()

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.ClientIPFromRemoteAddr)
	r.Use(RequestLogger)
	r.Use(middleware.Recoverer)
	r.Use(ResponseRequestID)
	r.Use(middleware.Heartbeat("/healthz"))
	r.Use(middleware.Timeout(300 * time.Second))

	// TODO what should be a correct number?
	r.With(middleware.Throttle(20)).Get(ni.OnieURLSuffix, svc.handleONIE)

	// TODO what should be a correct number?
	r.With(middleware.Throttle(20)).Get(AgentURLSuffix, svc.handleAgent)

	// TODO what should be a correct number?
	r.With(middleware.Throttle(20)).Get(CumulusZTPURLSuffix, svc.handleCumulusZTP)

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
		return fmt.Errorf("running server: %w", err)
	}

	return nil
}
