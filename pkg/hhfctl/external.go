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

package hhfctl

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type ExternalCreateOptions struct {
	Name              string
	IPv4Namespace     string
	InboundCommunity  string
	OutboundCommunity string
}

func ExternalCreate(ctx context.Context, printYaml bool, options *ExternalCreateOptions) error {
	ext := &vpcapi.External{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: vpcapi.ExternalSpec{
			IPv4Namespace:     options.IPv4Namespace,
			InboundCommunity:  options.InboundCommunity,
			OutboundCommunity: options.OutboundCommunity,
		},
	}

	kube, err := kubeutil.NewClient(ctx, "", vpcapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	ext.Default()
	warnings, err := ext.Validate(ctx /* validation.WithCtrlRuntime(kube) */, nil, nil)
	if err != nil {
		slog.Warn("Validation", "error", err)

		return errors.Errorf("validation failed")
	}
	if warnings != nil {
		slog.Warn("Validation", "warnings", warnings)
	}

	err = kube.Create(ctx, ext)
	if err != nil {
		return errors.Wrap(err, "cannot create external")
	}

	slog.Info("External created", "name", ext.Name)

	if printYaml {
		ext.ObjectMeta.ManagedFields = nil
		ext.ObjectMeta.Generation = 0
		ext.ObjectMeta.ResourceVersion = ""

		out, err := yaml.Marshal(ext)
		if err != nil {
			return errors.Wrap(err, "cannot marshal ext")
		}

		fmt.Println(string(out))
	}

	return nil
}

type ExternalPeeringOptions struct {
	VPC              string
	VPCSubnets       []string
	External         string
	ExternalPrefixes []string
}

func ExternalPeering(ctx context.Context, printYaml bool, options *ExternalPeeringOptions) error {
	extPeering := &vpcapi.ExternalPeering{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", options.VPC, options.External),
			Namespace: metav1.NamespaceDefault,
		},
		Spec: vpcapi.ExternalPeeringSpec{
			Permit: vpcapi.ExternalPeeringSpecPermit{
				VPC: vpcapi.ExternalPeeringSpecVPC{
					Name:    options.VPC,
					Subnets: options.VPCSubnets,
				},
				External: vpcapi.ExternalPeeringSpecExternal{
					Name:     options.External,
					Prefixes: []vpcapi.ExternalPeeringSpecPrefix{},
				},
			},
		},
	}

	for _, prefix := range options.ExternalPrefixes {
		// Temporarty remove le/ge
		/*le, ge := 0, 0

		prefixParts := strings.Split(rawPrefix, "_")
		if len(prefixParts) > 3 {
			return errors.Errorf("invalid external peering format %s, external prefix should be in format prefix_leXX_geYY", rawPrefix)
		}

		prefix := prefixParts[0]

		if len(prefixParts) > 1 {
			var err error
			for _, prefixPart := range prefixParts[1:] {
				if strings.HasPrefix(prefixPart, "le") {
					le, err = strconv.Atoi(strings.TrimPrefix(prefixPart, "le"))
					if err != nil {
						return errors.Errorf("invalid external peering %s, external prefix should be in format prefix_leXX_geYY", rawPrefix)
					}
				} else if strings.HasPrefix(prefixPart, "ge") {
					ge, err = strconv.Atoi(strings.TrimPrefix(prefixPart, "ge"))
					if err != nil {
						return errors.Errorf("invalid external peering %s, external prefix should be in format prefix_leXX_geYY", rawPrefix)
					}
				} else {
					return errors.Errorf("invalid external peering %s, external prefix should be in format prefix_leXX_geYY", rawPrefix)
				}
			}
		}

		extPeering.Spec.Permit.External.Prefixes = append(extPeering.Spec.Permit.External.Prefixes, vpcapi.ExternalPeeringSpecPrefix{
			Prefix: prefix,
			Le:     uint8(le),
			Ge:     uint8(ge),
		})*/
		extPeering.Spec.Permit.External.Prefixes = append(extPeering.Spec.Permit.External.Prefixes, vpcapi.ExternalPeeringSpecPrefix{
			Prefix: prefix,
		})
	}

	kube, err := kubeutil.NewClient(ctx, "", vpcapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	extPeering.Default()
	warnings, err := extPeering.Validate(ctx /* validation.WithCtrlRuntime(kube) */, nil, nil)
	if err != nil {
		slog.Warn("Validation", "error", err)

		return errors.Errorf("validation failed")
	}
	if warnings != nil {
		slog.Warn("Validation", "warnings", warnings)
	}

	err = kube.Create(ctx, extPeering)
	if err != nil {
		return errors.Wrap(err, "cannot create external peering")
	}

	slog.Info("ExternalPeering created", "name", extPeering.Name)

	if printYaml {
		extPeering.ObjectMeta.ManagedFields = nil
		extPeering.ObjectMeta.Generation = 0
		extPeering.ObjectMeta.ResourceVersion = ""

		out, err := yaml.Marshal(extPeering)
		if err != nil {
			return errors.Wrap(err, "cannot marshal ExternalPeering")
		}

		fmt.Println(string(out))
	}

	return nil
}
