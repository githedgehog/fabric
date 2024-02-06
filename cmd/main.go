/*
Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	agentv1alpha2 "go.githedgehog.com/fabric/api/agent/v1alpha2"
	dhcpv1alpha2 "go.githedgehog.com/fabric/api/dhcp/v1alpha2"
	vpcv1alpha2 "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringv1alpha2 "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	agentcontroller "go.githedgehog.com/fabric/pkg/ctrl/agent"
	connectioncontroller "go.githedgehog.com/fabric/pkg/ctrl/connection"
	controlagentcontroller "go.githedgehog.com/fabric/pkg/ctrl/controlagent"
	vpccontroller "go.githedgehog.com/fabric/pkg/ctrl/vpc"
	"go.githedgehog.com/fabric/pkg/manager/config"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	connectionWebhook "go.githedgehog.com/fabric/pkg/webhook/connection"
	externalWebhool "go.githedgehog.com/fabric/pkg/webhook/external"
	externalAttachmentWebhook "go.githedgehog.com/fabric/pkg/webhook/externalattachment"
	externalPeeringWebhook "go.githedgehog.com/fabric/pkg/webhook/externalpeering"
	ipv4NamespaceWebhook "go.githedgehog.com/fabric/pkg/webhook/ipv4ns"
	serverWebhook "go.githedgehog.com/fabric/pkg/webhook/server"
	switchWebhook "go.githedgehog.com/fabric/pkg/webhook/switchh"
	vlanNamespaceWebook "go.githedgehog.com/fabric/pkg/webhook/vlanns"
	vpcWebhook "go.githedgehog.com/fabric/pkg/webhook/vpc"
	vpcAttachmentWebhook "go.githedgehog.com/fabric/pkg/webhook/vpcattachment"
	vpcPeeringWebhook "go.githedgehog.com/fabric/pkg/webhook/vpcpeering"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(agentv1alpha2.AddToScheme(scheme))
	utilruntime.Must(wiringv1alpha2.AddToScheme(scheme))
	utilruntime.Must(vpcv1alpha2.AddToScheme(scheme))
	utilruntime.Must(dhcpv1alpha2.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

var version = "(devel)"

//go:embed motd.txt
var motd []byte

func main() {
	_, err := os.Stdout.Write(motd)
	if err != nil {
		log.Fatal("failed to write motd:", err)
	}
	fmt.Println("Version:", version)

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	cfgBasedir := "/etc/hedgehog/fabric" // TODO config?
	cfg, err := config.Load(cfgBasedir)
	if err != nil {
		setupLog.Error(err, "unable to load config")
		os.Exit(1)
	}

	setupLog.Info("Config loaded", "config", cfg)

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
					&agentv1alpha2.Catalog{},
				},
				Unstructured: false,
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	libMngr := librarian.NewManager(cfg)

	if err = agentcontroller.SetupWithManager(cfgBasedir, mgr, cfg, libMngr, version); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Agent")
		os.Exit(1)
	}
	if err = controlagentcontroller.SetupWithManager(cfgBasedir, mgr, cfg, version); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ControlAgent")
		os.Exit(1)
	}
	if err = vpccontroller.SetupWithManager(cfgBasedir, mgr, cfg, libMngr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VPC")
		os.Exit(1)
	}
	if err = connectioncontroller.SetupWithManager(cfgBasedir, mgr, cfg, libMngr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Connection")
		os.Exit(1)
	}

	if err = connectionWebhook.SetupWithManager(cfgBasedir, mgr, cfg); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Connection")
		os.Exit(1)
	}
	if err = serverWebhook.SetupWithManager(cfgBasedir, mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Server")
		os.Exit(1)
	}
	if err = switchWebhook.SetupWithManager(cfgBasedir, mgr, cfg); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Switch")
		os.Exit(1)
	}
	if err = vpcWebhook.SetupWithManager(cfgBasedir, mgr, cfg); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VPC")
		os.Exit(1)
	}
	if err = vpcAttachmentWebhook.SetupWithManager(cfgBasedir, mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VPCAttachment")
		os.Exit(1)
	}
	if err = vpcPeeringWebhook.SetupWithManager(cfgBasedir, mgr, cfg); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VPCPeering")
		os.Exit(1)
	}
	if err = ipv4NamespaceWebhook.SetupWithManager(cfgBasedir, mgr, cfg); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "IPv4Namespace")
		os.Exit(1)
	}
	if err = vlanNamespaceWebook.SetupWithManager(cfgBasedir, mgr, cfg); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VLANNamespace")
		os.Exit(1)
	}
	if err = externalWebhool.SetupWithManager(cfgBasedir, mgr, cfg); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "External")
		os.Exit(1)
	}
	if err = externalAttachmentWebhook.SetupWithManager(cfgBasedir, mgr, cfg); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ExternalAttachment")
		os.Exit(1)
	}
	if err = externalPeeringWebhook.SetupWithManager(cfgBasedir, mgr, cfg); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ExternalPeering")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
