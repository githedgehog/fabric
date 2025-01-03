// Copyright 2024 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package apiutil

import (
	"context"
	"fmt"
	"strings"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LLDPNeighbor struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Port        string `json:"port,omitempty"`
}

type LLDPNeighborType string

const (
	LLDPNeighborTypeFabric   LLDPNeighborType = "fabric"
	LLDPNeighborTypeExternal LLDPNeighborType = "external"
	LLDPNeighborTypeServer   LLDPNeighborType = "server"
)

type LLDPNeighborStatus struct {
	ConnectionName string           `json:"connectionName,omitempty"`
	ConnectionType string           `json:"connectionType,omitempty"`
	Type           LLDPNeighborType `json:"type,omitempty"`
	Expected       LLDPNeighbor     `json:"expected,omitempty"`
	Actual         LLDPNeighbor     `json:"actual,omitempty"`
}

func GetLLDPNeighbors(ctx context.Context, kube client.Reader, sw *wiringapi.Switch) (map[string]LLDPNeighborStatus, error) {
	if sw == nil {
		return nil, fmt.Errorf("switch is nil") //nolint:goerr113
	}

	ag := &agentapi.Agent{}
	if err := kube.Get(ctx, client.ObjectKey{Name: sw.Name, Namespace: sw.Namespace}, ag); err != nil {
		return nil, fmt.Errorf("getting agent %s: %w", sw.Name, err)
	}

	out := map[string]LLDPNeighborStatus{}

	sps := map[string]*wiringapi.SwitchProfile{}
	swSP := map[string]*wiringapi.SwitchProfile{}
	swNOS2API := map[string]map[string]string{}
	spNOS2API := map[string]map[string]string{}

	swList := &wiringapi.SwitchList{}
	if err := kube.List(ctx, swList); err != nil {
		return nil, fmt.Errorf("listing switches: %w", err)
	}
	for _, sw := range swList.Items {
		if _, ok := spNOS2API[sw.Spec.Profile]; !ok {
			sp := &wiringapi.SwitchProfile{}
			if err := kube.Get(ctx, client.ObjectKey{Name: sw.Spec.Profile, Namespace: sw.Namespace}, sp); err != nil {
				return nil, fmt.Errorf("getting switch profile %s: %w", sw.Spec.Profile, err)
			}

			ports, err := sp.Spec.GetNOS2APIPortsFor(&sw.Spec)
			if err != nil {
				return nil, fmt.Errorf("getting NOS ports mapping for %s: %w", sw.Name, err)
			}

			sps[sp.Name] = sp
			spNOS2API[sp.Name] = ports
		}

		swSP[sw.Name] = sps[sw.Spec.Profile]
		swNOS2API[sw.Name] = spNOS2API[sw.Spec.Profile]
	}

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns, wiringapi.MatchingLabelsForListLabelSwitch(sw.Name)); err != nil {
		return nil, fmt.Errorf("listing connections: %w", err)
	}

	for _, conn := range conns.Items {
		_, _, _, links, err := conn.Spec.Endpoints()
		if err != nil {
			return nil, fmt.Errorf("getting endpoints for %s: %w", conn.Name, err)
		}

		for k, v := range links {
			links[v] = k
		}

		for k, v := range links {
			kParts := strings.SplitN(k, "/", 2)
			kDevice, kPort := kParts[0], kParts[1]

			vParts := strings.SplitN(v, "/", 2)
			vDevice, vPort := vParts[0], vParts[1]

			if kDevice != sw.Name {
				continue
			}

			var statusType LLDPNeighborType
			if conn.Spec.MCLAGDomain != nil || conn.Spec.Fabric != nil || conn.Spec.VPCLoopback != nil {
				statusType = LLDPNeighborTypeFabric
			} else if conn.Spec.External != nil {
				statusType = LLDPNeighborTypeExternal
			} else {
				statusType = LLDPNeighborTypeServer
			}

			if sp, exist := swSP[kDevice]; exist {
				port, err := sp.Spec.NormalizePortName(kPort)
				if err != nil {
					return nil, fmt.Errorf("normalizing port name %s: %w", kPort, err)
				}
				kPort = port
			} else {
				return nil, fmt.Errorf("switch profile not found for %s", kDevice) //nolint:goerr113
			}

			if statusType == LLDPNeighborTypeFabric {
				if sp, exist := swSP[vDevice]; exist {
					port, err := sp.Spec.NormalizePortName(vPort)
					if err != nil {
						return nil, fmt.Errorf("normalizing port name %s: %w", vPort, err)
					}
					vPort = port
				} else {
					return nil, fmt.Errorf("switch profile not found for %s", vDevice) //nolint:goerr113
				}
			}

			status, ok := out[kPort]
			if ok {
				return nil, fmt.Errorf("duplicate port %s", kPort) //nolint:goerr113
			}

			status.Type = statusType
			status.ConnectionName = conn.Name
			status.ConnectionType = conn.Spec.Type()
			status.Expected = LLDPNeighbor{
				Name: vDevice,
				Port: vPort,
			}

			out[kPort] = status
		}
	}

	for ifaceName, iface := range ag.Status.State.Interfaces {
		for _, neighbor := range iface.LLDPNeighbors {
			status := out[ifaceName]

			port := neighbor.PortID
			if status.Type == LLDPNeighborTypeFabric {
				if status.Expected.Name != "" {
					status.Expected.Description = wiringapi.SwitchLLDPDescription(ag.Spec.Config.DeploymentID)
				} else {
					return nil, fmt.Errorf("expected neighbor name not found for %s while type if fabric", ifaceName) //nolint:goerr113
				}

				ports, ok := swNOS2API[status.Expected.Name]
				if !ok {
					return nil, fmt.Errorf("NOS ports mapping for %s not found", status.Expected.Name) //nolint:goerr113
				}

				if apiPort, ok := ports[port]; ok {
					port = apiPort
				} else {
					return nil, fmt.Errorf("port mapping for %s not found in switch %s", port, status.Expected.Name) //nolint:goerr113
				}
			}

			status.Actual = LLDPNeighbor{
				Name:        neighbor.SystemName,
				Description: neighbor.SystemDescription,
				Port:        port,
			}

			out[ifaceName] = status
		}
	}

	return out, nil
}
