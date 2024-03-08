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

package framework

import (
	"context"
	"log/slog"

	gg "github.com/onsi/ginkgo/v2"
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

type KubeClient struct {
	client client.WithWatch
}

func getKubeClient() (*KubeClient, error) {
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

	return &KubeClient{
		client: client,
	}, nil
}

func (c *KubeClient) Wait(ctx context.Context, objs ...client.Object) error {
	// TODO
	return nil
}

func (c *KubeClient) VPCCreate(ctx context.Context, name string, spec vpcapi.VPCSpec) (*vpcapi.VPC, error) {
	vpc := &vpcapi.VPC{
		ObjectMeta: ctrl.ObjectMeta{
			Name: name,
		},
		Spec: spec,
	}
	// err := c.client.Create(ctx, vpc)
	// if err != nil {
	// 	return nil, err
	// }

	// TODO

	gg.DeferCleanup(func(ctx context.Context) error {
		slog.Info("VPCCreate cleanup", "vpc", name)
		return nil
	})

	return vpc, nil
}

func (c *KubeClient) VPCDelete(ctx context.Context, vpc *vpcapi.VPC) error {
	// TODO
	return nil
}

func (c *KubeClient) VPCAttach(ctx context.Context, vpc *vpcapi.VPC, server string) (*vpcapi.VPCAttachment, error) {
	attach := &vpcapi.VPCAttachment{}

	gg.DeferCleanup(func(ctx context.Context) error {
		slog.Info("VPCAttach cleanup", "vpc", vpc.Name, "server", server)
		return nil
	})

	// TODO
	return attach, nil
}

func (c *KubeClient) VPCDetach(ctx context.Context, vpc *vpcapi.VPC, server string) error {
	// TODO
	return nil
}

func (c *KubeClient) VPCPeer(ctx context.Context, vpc1, vpc2 *vpcapi.VPC) (*vpcapi.VPCPeering, error) {
	peer := &vpcapi.VPCPeering{}

	gg.DeferCleanup(func(ctx context.Context) error {
		slog.Info("VPCPeer cleanup", "vpc1", vpc1.Name, "vpc2", vpc2.Name)
		return nil
	})

	// TODO
	return peer, nil
}

func (c *KubeClient) VPCUnpeer(ctx context.Context, vpc1, vpc2 *vpcapi.VPC) error {
	// TODO
	return nil
}
