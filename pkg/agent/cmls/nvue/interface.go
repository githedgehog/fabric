// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"fmt"
	"time"
)

const (
	InterfaceTypeSwp      = "swp"
	InterfaceTypeEth      = "eth"
	InterfaceTypeBond     = "bond"
	InterfaceTypeSub      = "sub"
	InterfaceTypeSVI      = "svi"
	InterfaceTypeLoopback = "loopback"
	InterfaceTypeBridge   = "bridge"
	InterfaceTypeVRF      = "vrf"
	InterfaceTypeVxlan    = "vxlan"

	StatusUp   = "up"
	StatusDown = "down"

	StateEnabled  = "enabled"
	StateDisabled = "disabled"

	// lastChangeLayout is how the link reports its last change, in the local time of the switch
	lastChangeLayout = "2006/01/02 15:04:05.000"
	lastChangeNever  = "Never"
)

// Interface is an interface of any type, only the parts the agent reads are modeled
type Interface struct {
	Type        string                    `json:"type"`
	Description string                    `json:"description"`
	Link        *InterfaceLink            `json:"link"`
	Bond        *InterfaceBond            `json:"bond"`
	LLDP        *InterfaceLLDP            `json:"lldp"`
	Counters    *InterfaceCarrierCounters `json:"counters"`
}

type InterfaceLink struct {
	AdminStatus string `json:"admin-status"`
	OperStatus  string `json:"oper-status"`
	// OperStatusLastChange is in the local time of the switch, see LastChange
	OperStatusLastChange string `json:"oper-status-last-change"`
	Speed                string `json:"speed"`
	AutoNegotiate        string `json:"auto-negotiate"`
	FEC                  string `json:"fec"`
	MACAddress           string `json:"mac-address"`
	MTU                  Number `json:"mtu"`
	// Protodown is how the protocols such as link flap protection take a port down
	Protodown string `json:"protodown"`
}

// LastChange is when the oper status last changed, loc is the timezone of the switch
func (l *InterfaceLink) LastChange(loc *time.Location) (time.Time, error) {
	// an interface that hasn't changed its oper status since it was created, such as a bond that was created down, has no
	// last change
	if l.OperStatusLastChange == "" || l.OperStatusLastChange == lastChangeNever {
		return time.Time{}, nil
	}

	t, err := time.ParseInLocation(lastChangeLayout, l.OperStatusLastChange, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing last change %q: %w", l.OperStatusLastChange, err)
	}

	return t, nil
}

type InterfaceBond struct {
	Mode       string                         `json:"mode"`
	LACPRate   string                         `json:"lacp-rate"`
	LACPBypass string                         `json:"lacp-bypass"`
	Member     map[string]InterfaceBondMember `json:"member"`
}

type InterfaceBondMember struct {
	BondingState string `json:"bonding-state"`
	MIIStatus    string `json:"mii-status"`
}

type InterfaceLLDP struct {
	Neighbor map[string]LLDPNeighbor `json:"neighbor"`
}

type LLDPNeighbor struct {
	// Age is in seconds
	Age     Number      `json:"age"`
	Chassis LLDPChassis `json:"chassis"`
	Port    LLDPPort    `json:"port"`
	MED     LLDPMED     `json:"lldp-med"`
}

type LLDPChassis struct {
	ChassisID         string `json:"chassis-id"`
	SystemName        string `json:"system-name"`
	SystemDescription string `json:"system-description"`
}

type LLDPPort struct {
	// Name is the port id, of the type in Type ("ifname", "mac", "local")
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	// TTL is in seconds
	TTL Number `json:"ttl"`
}

type LLDPMED struct {
	DeviceType string        `json:"device-type"`
	Inventory  LLDPInventory `json:"inventory"`
}

type LLDPInventory struct {
	Manufacturer     string `json:"manufacturer"`
	Model            string `json:"model"`
	SerialNumber     string `json:"serial-number"`
	FirmwareRevision string `json:"firmware-revision"`
	SoftwareRevision string `json:"software-revision"`
}

// InterfaceCarrierCounters are the only counters the interface listing has
type InterfaceCarrierCounters struct {
	Link struct {
		CarrierTransitions Counter `json:"carrier-transitions"`
		CarrierUpCount     Counter `json:"carrier-up-count"`
		CarrierDownCount   Counter `json:"carrier-down-count"`
	} `json:"link"`
}

// Interfaces lists the interfaces of every type by name, with their LLDP neighbors
func (c *Client) Interfaces(ctx context.Context) (map[string]Interface, error) {
	ifaces := map[string]Interface{}
	if err := c.get(ctx, pathOf("interface"), nil, &ifaces); err != nil {
		return nil, err
	}

	return ifaces, nil
}
