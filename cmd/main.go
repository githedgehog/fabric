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

package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/go-logr/logr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	agentctrl "go.githedgehog.com/fabric/pkg/ctrl/agent"
	connectionctrl "go.githedgehog.com/fabric/pkg/ctrl/connection"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	vpcctrl "go.githedgehog.com/fabric/pkg/ctrl/vpc"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	"go.githedgehog.com/fabric/pkg/version"
	connectionwh "go.githedgehog.com/fabric/pkg/webhook/connection"
	externalwh "go.githedgehog.com/fabric/pkg/webhook/external"
	externalattachmentwh "go.githedgehog.com/fabric/pkg/webhook/externalattachment"
	externalpeeringwh "go.githedgehog.com/fabric/pkg/webhook/externalpeering"
	ipv4namespacewh "go.githedgehog.com/fabric/pkg/webhook/ipv4ns"
	serverwh "go.githedgehog.com/fabric/pkg/webhook/server"
	switchwh "go.githedgehog.com/fabric/pkg/webhook/switchh"
	switchprofilewh "go.githedgehog.com/fabric/pkg/webhook/switchprofile"
	vlannamespacewh "go.githedgehog.com/fabric/pkg/webhook/vlanns"
	vpcwh "go.githedgehog.com/fabric/pkg/webhook/vpc"
	vpcattachmentwh "go.githedgehog.com/fabric/pkg/webhook/vpcattachment"
	vpcpeeringwh "go.githedgehog.com/fabric/pkg/webhook/vpcpeering"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/yaml"
	//+kubebuilder:scaffold:imports
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(agentapi.AddToScheme(scheme))
	utilruntime.Must(wiringapi.AddToScheme(scheme))
	utilruntime.Must(vpcapi.AddToScheme(scheme))
	utilruntime.Must(dhcpapi.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	// TODO make it configurable
	logLevel := slog.LevelDebug

	logW := os.Stderr
	handler := tint.NewHandler(logW, &tint.Options{
		Level:      logLevel,
		TimeFormat: time.StampMilli,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)
	ctrl.SetLogger(logr.FromSlogHandler(handler))
	klog.SetSlogLogger(logger)

	if err := run(); err != nil {
		slog.Error("Failed to run", "error", err)
		os.Exit(1)
	}
}

func run() error {
	slog.Info("Starting fabric-ctrl", "version", version.Version)

	cfgBasedir := "/etc/hedgehog/fabric"
	cfg, err := meta.LoadFabricConfig(cfgBasedir)
	if err != nil {
		return fmt.Errorf("loading fabric config: %w", err)
	}

	ca, err := os.ReadFile("/etc/hedgehog/ca/ca.crt")
	if err != nil {
		return fmt.Errorf("reading CA: %w", err)
	}

	username, err := os.ReadFile("/creds/" + corev1.BasicAuthUsernameKey)
	if err != nil {
		return fmt.Errorf("reading registry username: %w", err)
	}

	password, err := os.ReadFile("/creds/" + corev1.BasicAuthPasswordKey)
	if err != nil {
		return fmt.Errorf("reading registry password: %w", err)
	}

	loadedCfgData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling loaded config: %w", err)
	}

	slog.Info("Config loaded", "config", string(loadedCfgData))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":8080",
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: ":8081",
		LeaderElection:         true,
		LeaderElectionID:       "fabric.githedgehog.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,

		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&agentapi.Catalog{},
				},
				Unstructured: false,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("starting manager: %w", err)
	}

	libMngr := librarian.NewManager(cfg)

	profiles := switchprofile.NewDefaultSwitchProfiles()
	if err := profiles.RegisterAll(context.TODO(), mgr.GetClient(), cfg); err != nil {
		return fmt.Errorf("registering default switch profiles: %w", err)
	}

	if err = agentctrl.SetupWithManager(mgr, cfg, libMngr, string(ca), string(username), string(password)); err != nil {
		return fmt.Errorf("setting up agent controller: %w", err)
	}
	if err = vpcctrl.SetupWithManager(mgr, cfg, libMngr); err != nil {
		return fmt.Errorf("setting up vpc controller: %w", err)
	}
	if err = connectionctrl.SetupWithManager(mgr, cfg, libMngr); err != nil {
		return fmt.Errorf("setting up connection controller: %w", err)
	}
	if err = switchprofile.SetupWithManager(mgr, cfg, libMngr, profiles); err != nil {
		return fmt.Errorf("setting up switch profile controller: %w", err)
	}

	if err = connectionwh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up connection webhook: %w", err)
	}
	if err = serverwh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up server webhook: %w", err)
	}
	if err = switchwh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up switch webhook: %w", err)
	}
	if err = vpcwh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up vpc webhook: %w", err)
	}
	if err = vpcattachmentwh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up vpc attachment webhook: %w", err)
	}
	if err = vpcpeeringwh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up vpc peering webhook: %w", err)
	}
	if err = ipv4namespacewh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up ipv4 namespace webhook: %w", err)
	}
	if err = vlannamespacewh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up vlan namespace webhook: %w", err)
	}
	if err = externalwh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up external webhook: %w", err)
	}
	if err = externalattachmentwh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up external attachment webhook: %w", err)
	}
	if err = externalpeeringwh.SetupWithManager(mgr, cfg); err != nil {
		return fmt.Errorf("setting up external peering webhook: %w", err)
	}
	if err = switchprofilewh.SetupWithManager(mgr, cfg, profiles); err != nil {
		return fmt.Errorf("setting up switch profile webhook: %w", err)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up ready check: %w", err)
	}

	slog.Info("Starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("running manager: %w", err)
	}

	return nil
}
