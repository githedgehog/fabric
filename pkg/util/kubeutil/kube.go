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

package kubeutil

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

func NewClient(ctx context.Context, kubeconfigPath string, schemeBuilders ...*scheme.Builder) (client.WithWatch, error) {
	return newClient(ctx, kubeconfigPath, false, false, schemeBuilders...)
}

func NewClientWithCore(ctx context.Context, kubeconfigPath string, schemeBuilders ...*scheme.Builder) (client.WithWatch, error) {
	return newClient(ctx, kubeconfigPath, true, false, schemeBuilders...)
}

func NewClientWithCache(ctx context.Context, kubeconfigPath string, schemeBuilders ...*scheme.Builder) (client.WithWatch, error) {
	return newClient(ctx, kubeconfigPath, false, true, schemeBuilders...)
}

// TODO cached version is minimal naive implementation with hanging go routine, need to be improved
func newClient(ctx context.Context, kubeconfigPath string, core, cached bool, schemeBuilders ...*scheme.Builder) (client.WithWatch, error) {
	var cfg *rest.Config
	var err error

	if kubeconfigPath == "" {
		if cfg, err = ctrl.GetConfig(); err != nil {
			return nil, errors.Wrapf(err, "failed to get kubeconfig using default path or in-cluster config")
		}
	} else {
		if cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
			nil,
		).ClientConfig(); err != nil {
			return nil, errors.Wrapf(err, "failed to load kubeconfig from %s", kubeconfigPath)
		}
	}

	scheme := runtime.NewScheme()

	if core {
		if err := corev1.AddToScheme(scheme); err != nil {
			return nil, errors.Wrapf(err, "failed to add core scheme to runtime")
		}
	}

	for _, schemeBuilder := range schemeBuilders {
		if err := schemeBuilder.AddToScheme(scheme); err != nil {
			return nil, errors.Wrapf(err, "failed to add scheme %s to runtime", schemeBuilder.GroupVersion.String())
		}
	}

	var cacheOpts *client.CacheOptions
	if cached {
		clientCache, err := cache.New(cfg, cache.Options{
			Scheme: scheme,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create kube controller runtime cache")
		}

		go func() {
			if err := clientCache.Start(ctx); err != nil {
				slog.Error("failed to start kube controller runtime cache", "err", err)
				panic(err)
			}
		}()

		if !clientCache.WaitForCacheSync(ctx) {
			return nil, errors.New("failed to sync kube controller runtime cache")
		}

		cacheOpts = &client.CacheOptions{
			Reader: clientCache,
		}
	}

	kubeClient, err := client.NewWithWatch(cfg, client.Options{
		Scheme: scheme,
		Cache:  cacheOpts,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create kube controller runtime client")
	}

	return kubeClient, nil
}
