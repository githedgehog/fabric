// Copyright 2024 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package apiutil

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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

// lldpData is what the LLDP neighbors of any switch may refer to, loaded once or on demand and shared by all of them
type lldpData struct {
	// only the servers that advertise a system name other than their object name
	serverNames map[string]string
	nos2API     map[string]map[string]string
}

func loadLLDPData(ctx context.Context, kube kclient.Reader) (*lldpData, error) {
	data := &lldpData{
		serverNames: map[string]string{},
		nos2API:     map[string]map[string]string{},
	}

	srvList := &wiringapi.ServerList{}
	if err := kube.List(ctx, srvList, kclient.InNamespace(kmetav1.NamespaceDefault)); err != nil {
		return nil, fmt.Errorf("listing servers: %w", err)
	}
	for _, srv := range srvList.Items {
		if name := srv.Spec.Inspect.ExpectedSystemName; name != "" {
			data.serverNames[srv.Name] = name
		}
	}

	return data, nil
}

func normalizePortName(ctx context.Context, in *switchInput, swName, port string) (string, error) {
	_, sp, err := in.cache.switchProfile(ctx, swName)
	if err != nil {
		return "", err
	}

	normalized, err := sp.Spec.NormalizePortName(port)
	if err != nil {
		return "", fmt.Errorf("normalizing port name %s: %w", port, err)
	}

	return normalized, nil
}

// nos2APIPorts is the NOS to API port names mapping of a switch, only ever needed for the fabric peers
func (d *lldpData) nos2APIPorts(ctx context.Context, in *switchInput, swName string) (map[string]string, error) {
	if ports, ok := d.nos2API[swName]; ok {
		return ports, nil
	}

	sw, sp, err := in.cache.switchProfile(ctx, swName)
	if err != nil {
		return nil, err
	}

	ports, err := sp.Spec.GetNOS2APIPortsFor(&sw.Spec)
	if err != nil {
		return nil, fmt.Errorf("getting NOS ports mapping for %s: %w", swName, err)
	}
	d.nos2API[swName] = ports

	return ports, nil
}

// GetLLDPNeighbors returns the LLDP neighbors of the switches selected by the filter, keyed by switch name and port
func GetLLDPNeighbors(ctx context.Context, kube kclient.Reader, filter SwitchFilter, opts LLDPNeighborsOpts) (map[string]map[string]LLDPNeighborStatus, error) {
	if err := filter.check(ctx, kube); err != nil {
		return nil, err
	}

	data, err := loadLLDPData(ctx, kube)
	if err != nil {
		return nil, err
	}

	out := map[string]map[string]LLDPNeighborStatus{}
	if err := forEachSwitch(ctx, kube, filter, nil, func(in *switchInput) error {
		neighbors, err := lldpNeighbors(ctx, data, in, opts)
		if err != nil {
			return fmt.Errorf("getting LLDP neighbors for switch %s: %w", in.sw.Name, err)
		}

		out[in.sw.Name] = neighbors

		return nil
	}); err != nil {
		return nil, err
	}

	return out, nil
}

func lldpNeighbors(ctx context.Context, data *lldpData, in *switchInput, opts LLDPNeighborsOpts) (map[string]LLDPNeighborStatus, error) {
	sw, ag := in.sw, in.ag

	out := map[string]LLDPNeighborStatus{}

	for _, conn := range in.conns {
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

			kPort, err = normalizePortName(ctx, in, kDevice, kPort)
			if err != nil {
				return nil, err
			}

			if statusType == LLDPNeighborTypeFabric {
				vPort, err = normalizePortName(ctx, in, vDevice, vPort)
				if err != nil {
					return nil, err
				}
			}

			status, ok := out[kPort]
			if ok {
				return nil, fmt.Errorf("duplicate port %s", kPort) //nolint:goerr113
			}

			expectedName := vDevice
			// a server may advertise a system name that isn't its object name, only it knows so
			if statusType == LLDPNeighborTypeServer {
				if name, ok := data.serverNames[vDevice]; ok {
					expectedName = name
				}
			}

			status.Type = statusType
			status.ConnectionName = conn.Name
			status.ConnectionType = conn.Spec.Type()
			status.Expected = LLDPNeighbor{
				Name: expectedName,
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

				ports, err := data.nos2APIPorts(ctx, in, status.Expected.Name)
				if err != nil {
					return nil, err
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
