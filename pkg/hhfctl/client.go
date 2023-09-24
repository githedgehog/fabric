package hhfctl

import (
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(vpcapi.AddToScheme(scheme))
	utilruntime.Must(wiringapi.AddToScheme(scheme))
	utilruntime.Must(agentapi.AddToScheme(scheme))
}

func kubeClient() (client.WithWatch, error) {
	k8scfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	client, err := client.NewWithWatch(k8scfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}
