package vlab

import (
	"fmt"

	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/wiring"
	"golang.org/x/exp/maps"
	"sigs.k8s.io/yaml"
)

type Config struct {
	VMs   []VM   `json:"vms"`
	Links []Link `json:"links"`
}

type VM struct {
	Name  string
	Type  string
	Ports int
}

type Link [2]LinkPart

type LinkPart map[string]int

const (
	VMSwitch  = "switch"
	VMControl = "control"
	VMCompute = "compute"
	VMONIE    = "onie"
)

func PrintConfig(data *wiring.Data) error {
	cfg := &Config{
		VMs: []VM{
			{
				Name:  "control-0",
				Type:  VMControl,
				Ports: len(data.Switch.All()),
			},
		},
		Links: []Link{},
	}

	for _, sw := range data.Switch.All() {
		cfg.VMs = append(cfg.VMs, VM{
			Name: sw.Name,
			Type: VMSwitch,
			Ports: len(data.Port.Lookup(map[string]string{
				wiringapi.LabelSwitch: sw.Name,
			})),
		})
	}

	// add switch <-> switch links
	dedup := map[string]bool{}
	for _, port := range data.Port.All() {
		nPort := &wiringapi.SwitchPort{}
		if port.Spec.Neighbor.Switch != nil {
			nPort = data.Port.Get(port.Spec.Neighbor.Switch.Port)
		}
		// TODO handle server part of neighbor too

		if nPort.Name == "" {
			// log.Println("Skipping port", port.Name)
			continue
		}

		if dedup[port.Name] || dedup[nPort.Name] {
			continue
		}

		cfg.Links = append(cfg.Links, Link(
			[2]LinkPart{
				{port.GetSwitchName(): int(port.Spec.NOSPortNum)},
				{nPort.GetSwitchName(): int(nPort.Spec.NOSPortNum)},
			},
		))

		dedup[port.Name] = true
		dedup[nPort.Name] = true
	}

	// add control <-> switch links
	for id, sw := range data.Switch.All() {
		cfg.Links = append(cfg.Links, Link(
			[2]LinkPart{
				{"control-0": id},
				{sw.Name: 0},
			},
		))
	}

	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	fmt.Println(string(b))

	return nil
}

func (l *Link) LocalVM() string {
	return maps.Keys(l[0])[0]
}

func (l *Link) LocalPort() int {
	return maps.Values(l[0])[0]
}

func (l *Link) RemoteVM() string {
	return maps.Keys(l[1])[0]
}

func (l *Link) RemotePort() int {
	return maps.Values(l[1])[0]
}
