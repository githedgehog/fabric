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

package inspect

import (
	"context"
	"net"
	"strings"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AccessIn struct {
	Source      string
	Destination string
}

type AccessOut struct {
	SourceSubnets           []string            `json:"sourceSubnets,omitempty"`
	DestinationSubnets      []string            `json:"destinationSubnets,omitempty"`
	DestinationExternal     string              `json:"destinationExternal,omitempty"`
	Reachable               map[string][]string `json:"reachable,omitempty"`
	ExternalReachable       map[string]bool     `json:"externalReachable,omitempty"`
	StaticExternalReachable map[string]bool     `json:"staticExternalReachable,omitempty"`
}

func (out *AccessOut) MarshalText() (string, error) {
	str := strings.Builder{}

	str.WriteString("Source VPCSubnets: " + strings.Join(out.SourceSubnets, ", ") + "\n")
	if out.DestinationExternal != "" {
		str.WriteString("Destination External IP: " + out.DestinationExternal + "\n\n")

		if len(out.ExternalReachable) == 0 && len(out.StaticExternalReachable) == 0 {
			str.WriteString("Destination External IP not reachable from any source subnet\n")
		} else {
			if len(out.ExternalReachable) > 0 {
				str.WriteString("Destination External IP is potentilly reachable from (using ExternalPeering, if actual route received from External): \n")
				for subnet, reachable := range out.ExternalReachable {
					if !reachable {
						continue
					}

					str.WriteString("  " + subnet + "\n")
				}
			}

			if len(out.StaticExternalReachable) > 0 {
				str.WriteString("Destination External IP is reachable from (using StaticExternal): \n")
				for subnet, reachable := range out.StaticExternalReachable {
					if !reachable {
						continue
					}

					str.WriteString("  " + subnet + "\n")
				}
			}
		}
	} else {
		str.WriteString("Destination VPCSubnets: " + strings.Join(out.DestinationSubnets, ", ") + "\n\n")

		if len(out.Reachable) == 0 {
			str.WriteString("No Destination VPCSubnets reachable from any Source VPCSubnet\n")
		} else {
			str.WriteString("Reachable VPCSubnets: \n")
			for subnet, destSubnets := range out.Reachable {
				str.WriteString("  " + subnet + ": " + strings.Join(destSubnets, ", ") + "\n")
			}
		}
	}

	return str.String(), nil
}

var _ Func[AccessIn, *AccessOut] = Access

func Access(ctx context.Context, kube client.Reader, in AccessIn) (*AccessOut, error) {
	out := &AccessOut{
		Reachable:               map[string][]string{},
		ExternalReachable:       map[string]bool{},
		StaticExternalReachable: map[string]bool{},
	}

	if in.Source == "" {
		return nil, errors.New("source must be specified")
	}

	if in.Source == in.Destination {
		return nil, errors.New("source and destination must be different")
	}

	sourceSubnets, err := asVPCSubnets(ctx, kube, in.Source, "source")
	if err != nil {
		return nil, errors.Wrap(err, "failed to determine source subnet")
	}
	if len(sourceSubnets) == 0 {
		return nil, errors.New("source must be non-empty server name, full VPC subnet name (<vpc-name>/<subnet-name>) or valid IPv4 address from VPC subnets")
	}

	out.SourceSubnets = sourceSubnets

	destSubnets, err := asVPCSubnets(ctx, kube, in.Destination, "destination")
	if err != nil {
		return nil, errors.Wrap(err, "failed to determine destination subnet")
	}

	out.DestinationSubnets = destSubnets

	ext := false
	if extIP := net.ParseIP(in.Destination); len(destSubnets) == 0 && extIP != nil && extIP.To4() != nil {
		ext = true
		out.DestinationExternal = in.Destination
	}

	if !ext && len(destSubnets) == 0 {
		return nil, errors.New("destination must be non-empty server name, full VPC subnet name (<vpc-name>/<subnet-name>) or valid IPv4 address from VPC subnets")
	}

	if ext {
		for _, sourceSubnet := range sourceSubnets {
			reachable, err := apiutil.IsExternalIPReachable(ctx, kube, sourceSubnet, in.Destination)
			if err != nil {
				return nil, errors.Wrap(err, "failed to determine if destination external IP is reachable for subnet "+sourceSubnet+" using externals")
			}

			if reachable {
				out.ExternalReachable[sourceSubnet] = reachable
			}

			reachable, err = apiutil.IsStaticExternalIPReachable(ctx, kube, sourceSubnet, in.Destination)
			if err != nil {
				return nil, errors.Wrap(err, "failed to determine if destination external IP is reachable for subnet "+sourceSubnet+" using static externals")
			}

			if reachable {
				out.StaticExternalReachable[sourceSubnet] = true
			}
		}
	} else {
		for _, sourceSubnet := range sourceSubnets {
			for _, destSubnet := range destSubnets {
				reachable, err := apiutil.IsSubnetReachable(ctx, kube, sourceSubnet, destSubnet)
				if err != nil {
					return nil, errors.Wrap(err, "failed to determine if destination subnet is reachable from source subnet using externals")
				}

				if reachable {
					out.Reachable[sourceSubnet] = append(out.Reachable[sourceSubnet], destSubnet)
				}
			}
		}
	}

	return out, nil
}

func asVPCSubnets(ctx context.Context, kube client.Reader, in string, t string) ([]string, error) {
	if in == "" {
		return nil, nil
	}

	if strings.Contains(in, "/") {
		return []string{in}, nil
	}

	ip := net.ParseIP(in)
	if ip != nil {
		if ip.To4() == nil {
			return nil, errors.New(t + " must be server name, full VPC subnet name (<vpc-name>/<subnet-name>) or valid IPv4 address")
		}

		ipnsList := &vpcapi.IPv4NamespaceList{}
		err := kube.List(ctx, ipnsList)
		if err != nil {
			return nil, errors.Wrap(err, "cannot list IPv4Namespace")
		}

		for _, ipns := range ipnsList.Items {
			for _, subnetStr := range ipns.Spec.Subnets {
				_, subnetNet, err := net.ParseCIDR(subnetStr)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse ipns %s subnet %q", ipns.Name, subnetStr)
				}

				if subnetNet.Contains(ip) {
					vpcs := &vpcapi.VPCList{}
					err = kube.List(ctx, vpcs, client.MatchingLabels{
						vpcapi.LabelIPv4NS: ipns.Name,
					})
					if err != nil {
						return nil, errors.Wrap(err, "cannot list VPC")
					}

					for _, vpc := range vpcs.Items {
						for subnetName, subnet := range vpc.Spec.Subnets {
							_, subnetNet, err := net.ParseCIDR(subnet.Subnet)
							if err != nil {
								return nil, errors.Wrapf(err, "failed to parse vpc %s subnet %q", vpc.Name, subnet.Subnet)
							}

							if subnetNet.Contains(ip) {
								return []string{vpc.Name + "/" + subnetName}, nil
							}
						}
					}
				}
			}
		}
	} else {
		server := &wiringapi.Server{}
		err := kube.Get(ctx, types.NamespacedName{Name: in, Namespace: metav1.NamespaceDefault}, server)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get server")
		}

		subnets, err := apiutil.GetAttachedSubnets(ctx, kube, in)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get attached subnets for "+t+" "+in)
		}

		return maps.Keys(subnets), nil
	}

	return nil, nil
}
