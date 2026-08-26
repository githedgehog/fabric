// Copyright 2024 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package apiutil

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type LLDPNeighbor struct {
	// Name is reported as the neighbor advertises it, see IgnoredPrefix/IgnoredSuffix
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Port        string `json:"port,omitempty"`

	// Only reported for the actual neighbors, the wiring has nothing to expect them from

	MAC        string        `json:"mac,omitempty"`
	TTL        uint16        `json:"ttl,omitempty"`
	LastUpdate *kmetav1.Time `json:"updated,omitempty"`

	// Parts of the Name ignored to match the wiring, only set when they made it match
	IgnoredPrefix string `json:"ignoredPrefix,omitempty"`
	IgnoredSuffix string `json:"ignoredSuffix,omitempty"`
}

// MatchedName is the Name without the ignored parts, the one compared against the wiring.
func (n LLDPNeighbor) MatchedName() string {
	return strings.TrimSuffix(strings.TrimPrefix(n.Name, n.IgnoredPrefix), n.IgnoredSuffix)
}

// Matches reports whether the neighbor is the one the wiring expects, ignoring case as host and port names do.
// A port with nothing expected on it never matches, there is nothing to be right about.
func (n LLDPNeighbor) Matches(expected LLDPNeighbor) bool {
	if expected.Name == "" {
		return false
	}

	if !strings.EqualFold(n.MatchedName(), expected.Name) || !strings.EqualFold(n.Port, expected.Port) {
		return false
	}

	return expected.Description == "" || strings.EqualFold(n.Description, expected.Description)
}

type LLDPNeighborType string

const (
	LLDPNeighborTypeFabric   LLDPNeighborType = "fabric"
	LLDPNeighborTypeExternal LLDPNeighborType = "external"
	LLDPNeighborTypeServer   LLDPNeighborType = "server"
	LLDPNeighborTypeGateway  LLDPNeighborType = "gateway"
)

type LLDPNeighborStatus struct {
	ConnectionName string           `json:"connectionName,omitempty"`
	ConnectionType string           `json:"connectionType,omitempty"`
	Type           LLDPNeighborType `json:"type,omitempty"`
	Expected       LLDPNeighbor     `json:"expected,omitempty"`
	Actual         []LLDPNeighbor   `json:"actual,omitempty"`
}

// DPUs name themselves after their host and hosts report their FQDN, while the wiring knows neither.
var (
	DefaultLLDPIgnoreSuffixes = []string{"-dpu", ".lan", ".maas"}
	DefaultLLDPIgnorePrefixes = []string{}
)

type LLDPNeighborsOpts struct {
	// Ignored in the neighbor system names, unset means nothing is ignored, see the defaults above
	IgnorePrefixes []string
	IgnoreSuffixes []string
}

// lldpNeighborNameCut returns the parts of a neighbor name to ignore for it to be the expected one, e.g. the .lan of
// a server-1.lan wired as server-1. Nothing is cut unless it produces the expected name, so the wiring is free to call
// the neighbor ash033-dpu, and never down to an empty name.
func lldpNeighborNameCut(name, expected string, opts LLDPNeighborsOpts) (string, string) {
	if expected == "" {
		return "", ""
	}

	// offsets into the name, so that the ignored parts stay available for reporting
	type cut struct{ start, end int }

	full := cut{0, len(name)}
	seen := map[cut]bool{full: true}

	for queue := []cut{full}; len(queue) > 0; {
		cur := queue[0]
		queue = queue[1:]

		if strings.EqualFold(name[cur.start:cur.end], expected) {
			return name[:cur.start], name[cur.end:]
		}

		// each step strictly shortens what's left, so this terminates
		next := []cut{}
		for _, prefix := range opts.IgnorePrefixes {
			if prefix != "" && cur.end-cur.start > len(prefix) && strings.EqualFold(name[cur.start:cur.start+len(prefix)], prefix) {
				next = append(next, cut{cur.start + len(prefix), cur.end})
			}
		}
		for _, suffix := range opts.IgnoreSuffixes {
			if suffix != "" && cur.end-cur.start > len(suffix) && strings.EqualFold(name[cur.end-len(suffix):cur.end], suffix) {
				next = append(next, cut{cur.start, cur.end - len(suffix)})
			}
		}

		for _, candidate := range next {
			if !seen[candidate] {
				seen[candidate] = true
				queue = append(queue, candidate)
			}
		}
	}

	return "", ""
}

func GetLLDPNeighbors(ctx context.Context, kube kclient.Reader, sw *wiringapi.Switch, opts LLDPNeighborsOpts) (map[string]LLDPNeighborStatus, error) {
	if sw == nil {
		return nil, fmt.Errorf("switch is nil") //nolint:goerr113
	}

	ag := &agentapi.Agent{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: sw.Name, Namespace: sw.Namespace}, ag); err != nil {
		return nil, fmt.Errorf("getting agent %s: %w", sw.Name, err)
	}

	out := map[string]LLDPNeighborStatus{}

	sps := map[string]*wiringapi.SwitchProfile{}
	swSP := map[string]*wiringapi.SwitchProfile{}
	swNOS2API := map[string]map[string]string{}

	swList := &wiringapi.SwitchList{}
	if err := kube.List(ctx, swList); err != nil {
		return nil, fmt.Errorf("listing switches: %w", err)
	}
	for _, sw := range swList.Items {
		sp, ok := sps[sw.Spec.Profile]
		if !ok {
			sp = &wiringapi.SwitchProfile{}
			if err := kube.Get(ctx, kclient.ObjectKey{Name: sw.Spec.Profile, Namespace: sw.Namespace}, sp); err != nil {
				return nil, fmt.Errorf("getting switch profile %s: %w", sw.Spec.Profile, err)
			}
			sps[sp.Name] = sp
		}
		swSP[sw.Name] = sp

		ports, err := sp.Spec.GetNOS2APIPortsFor(&sw.Spec)
		if err != nil {
			return nil, fmt.Errorf("getting NOS ports mapping for %s: %w", sw.Name, err)
		}

		swNOS2API[sw.Name] = ports
	}

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns, wiringapi.MatchingLabelsForListLabelSwitch(sw.Name)); err != nil {
		return nil, fmt.Errorf("listing connections: %w", err)
	}

	for _, conn := range conns.Items {
		if conn.Spec.VPCLoopback != nil {
			continue
		}

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
			if conn.Spec.Fabric != nil || conn.Spec.Mesh != nil { //nolint:gocritic
				statusType = LLDPNeighborTypeFabric
			} else if conn.Spec.External != nil {
				statusType = LLDPNeighborTypeExternal
			} else if conn.Spec.Gateway != nil {
				statusType = LLDPNeighborTypeGateway
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

	// agents no longer report neighbors of their own management interface, but older ones still do
	for ifaceName, iface := range ag.Status.State.Interfaces {
		if strings.HasPrefix(ifaceName, wiringapi.ManagementPortPrefix) {
			continue
		}

		for _, neighbor := range iface.LLDPNeighbors {
			status := out[ifaceName]

			// port is derived by the agent, fall back to the raw port ID for the agents that don't report it yet
			port := neighbor.Port
			if port == "" {
				port = neighbor.PortID
			}

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

				// mapping is keyed by the NOS interface names, so it's only the raw port ID that can match it
				if apiPort, ok := ports[neighbor.PortID]; ok {
					port = apiPort
				} else {
					slog.Warn("Port mapping not found", "switch", status.Expected.Name, "portID", neighbor.PortID, "port", port)
				}
			}

			ignoredPrefix, ignoredSuffix := lldpNeighborNameCut(neighbor.SystemName, status.Expected.Name, opts)

			status.Actual = append(status.Actual, LLDPNeighbor{
				Name:          neighbor.SystemName,
				Description:   neighbor.SystemDescription,
				Port:          port,
				MAC:           neighbor.MAC,
				TTL:           neighbor.TTL,
				LastUpdate:    neighbor.LastUpdate,
				IgnoredPrefix: ignoredPrefix,
				IgnoredSuffix: ignoredSuffix,
			})

			out[ifaceName] = status
		}
	}

	return out, nil
}
