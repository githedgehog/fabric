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
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	ni "go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	"golang.org/x/sync/singleflight"
	"oras.land/oras-go/v2/registry/remote/auth"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	ListenPort = 32000
	ConfigPath = "/etc/fabric-boot/config/config.yaml"
	CAPath     = "/etc/fabric-boot/ca/ca.crt"
	CredsPath  = "/etc/fabric-boot/creds/config.json" //nolint:gosec
	CacheDir   = "/var/lib/fabric-boot/cache"
)

type Config struct {
	ControlVIP           string                  `json:"controlVIP,omitempty"`
	NOSRepoPrefix        string                  `json:"nosRepoPrefix,omitempty"`
	NOSVersions          map[meta.NOSType]string `json:"nosVersions,omitempty"`
	ONIERepoPrefix       string                  `json:"onieRepoPrefix,omitempty"`
	ONIEPlatformVersions map[string]string       `json:"oniePlatformVersions,omitempty"`
}

type service struct {
	cfg          *Config
	kube         client.Reader
	cacheDir     string
	orasClient   *auth.Client
	downloadLock *sync.Mutex
	sf           *singleflight.Group
}

func Run(ctx context.Context) error {
	if err := os.RemoveAll(CacheDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "removing cache dir %s", CacheDir)
	}
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

	cfg := &Config{}
	if err := yaml.Unmarshal(configData, cfg); err != nil {
		return fmt.Errorf("unmarshalling config: %w", err)
	}
	if cfg.ControlVIP == "" {
		return errors.New("ControlVIP is required")
	}
	cfg.NOSRepoPrefix = strings.TrimSuffix(cfg.NOSRepoPrefix, "/")
	if cfg.NOSRepoPrefix == "" {
		return errors.New("NOSRepoPrefix is required")
	}
	cfg.ONIERepoPrefix = strings.TrimSuffix(cfg.ONIERepoPrefix, "/")
	if cfg.ONIERepoPrefix == "" {
		return errors.New("ONIERepoPrefix is required")
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

	if err := svc.preCache(ctx); err != nil {
		return fmt.Errorf("pre-caching: %w", err)
	}

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
		Addr:              fmt.Sprintf("%s:%d", cfg.ControlVIP, ListenPort),
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
