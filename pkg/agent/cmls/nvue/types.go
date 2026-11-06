// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var errNotNumber = errors.New("not a number")

// leadingNumber matches the numeric part of values such as "54.0", "28.80 C", "2.00m" or "-2.22 dBm"
var leadingNumber = regexp.MustCompile(`^\s*[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?`)

// Number is a numeric value as NVUE reports it: a JSON number, a number in a string ("7020", "54.0"), a number
// with a unit ("28.80 C", "2.00m") or a placeholder such as "N/A" when the hardware has nothing to report. OK is
// false for the placeholders and Raw keeps the original text either way, so the non-numeric meanings are not lost
// (a BGP remote-as of "external" for one).
type Number struct {
	Value float64
	OK    bool
	Raw   string
}

func (n *Number) UnmarshalJSON(data []byte) error {
	*n = Number{}

	raw, err := decodeScalar(data)
	if err != nil {
		return err
	}

	switch v := raw.(type) {
	case nil:
	case json.Number:
		n.Raw = v.String()
		n.Value, err = v.Float64()
		if err != nil {
			return fmt.Errorf("parsing number %q: %w", v, err)
		}
		n.OK = true
	case string:
		n.Raw = v
		n.Value, n.OK = parseLeadingNumber(v)
	default:
		return fmt.Errorf("%w: %s", errNotNumber, string(data))
	}

	return nil
}

// Counter is a packet or byte counter, which NVUE reports as a JSON number or as "n/a" when the counter doesn't
// apply, such as transmit ACL drops. Unlike Number it keeps the full 64 bits.
type Counter struct {
	Value uint64
	OK    bool
}

func (c *Counter) UnmarshalJSON(data []byte) error {
	*c = Counter{}

	raw, err := decodeScalar(data)
	if err != nil {
		return err
	}

	var text string
	switch v := raw.(type) {
	case nil:
		return nil
	case json.Number:
		text = v.String()
	case string:
		text = strings.TrimSpace(v)
	default:
		return fmt.Errorf("%w: %s", errNotNumber, string(data))
	}

	// anything that isn't a plain integer is a placeholder, there's nothing to count
	if value, err := strconv.ParseUint(text, 10, 64); err == nil {
		c.Value, c.OK = value, true
	}

	return nil
}

// Power is an optical power level, which NVUE reports as "0.6004 mW / -2.22 dBm". DBm is what the rest of the
// fabric reports, OK is false when the module has no reading.
type Power struct {
	DBm float64
	OK  bool
	Raw string
}

func (p *Power) UnmarshalJSON(data []byte) error {
	*p = Power{}

	raw, err := decodeScalar(data)
	if err != nil {
		return err
	}

	switch v := raw.(type) {
	case nil:
	case json.Number:
		// a bare number is taken as dBm, the unit the thresholds are given in
		p.Raw = v.String()
		p.DBm, err = v.Float64()
		if err != nil {
			return fmt.Errorf("parsing power %q: %w", v, err)
		}
		p.OK = true
	case string:
		p.Raw = v
		p.DBm, p.OK = parsePower(v)
	default:
		return fmt.Errorf("%w: %s", errNotNumber, string(data))
	}

	return nil
}

func parsePower(s string) (float64, bool) {
	// "0.6004 mW / -2.22 dBm" carries both, the thresholds only "1.00 dBm"
	parts := strings.Split(s, "/")
	for _, part := range parts {
		if strings.Contains(part, "dBm") {
			return parseLeadingNumber(part)
		}
	}

	// a unit-less value is taken as dBm, like a bare JSON number
	if len(parts) == 1 && !strings.Contains(s, "mW") {
		return parseLeadingNumber(s)
	}

	// only the linear value is usable, convert it unless it's a zero that has no logarithm
	if mw, ok := parseLeadingNumber(parts[0]); ok && mw > 0 {
		return 10 * math.Log10(mw), true
	}

	return 0, false
}

func parseLeadingNumber(s string) (float64, bool) {
	match := leadingNumber.FindString(s)
	if match == "" {
		return 0, false
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(match), 64)

	return value, err == nil
}

// decodeScalar decodes a JSON scalar keeping numbers as json.Number, so that large counters aren't rounded
func decodeScalar(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", string(data), err)
	}

	return raw, nil
}

// Interface types as reported by GET /interface
const (
	InterfaceTypeSWP      = "swp"
	InterfaceTypeEth      = "eth"
	InterfaceTypeBond     = "bond"
	InterfaceTypeLoopback = "loopback"
)

// Interface is one entry of GET /interface, only the subtrees the agent reads are modeled
type Interface struct {
	Type     string             `json:"type"`
	IfIndex  int                `json:"ifindex"`
	Link     *InterfaceLink     `json:"link"`
	LLDP     *InterfaceLLDP     `json:"lldp"`
	Counters *InterfaceCounters `json:"counters"`
	// Only present with ?view=rates
	Rates *InterfaceRates `json:"rates"`
}

type InterfaceLink struct {
	AdminStatus string `json:"admin-status"`
	OperStatus  string `json:"oper-status"`
	// Local switch time formatted as "2006/01/02 15:04:05.000"
	OperStatusLastChange string `json:"oper-status-last-change"`
	MACAddress           string `json:"mac-address"`
	Speed                string `json:"speed"`
	AutoNegotiate        string `json:"auto-negotiate"`
	Duplex               string `json:"duplex"`
	FEC                  string `json:"fec"`
	MTU                  Number `json:"mtu"`
	Lanes                Number `json:"lanes"`
	// "disabled" normally, anything else means the port was taken down by a protocol (link flap, BPDU guard...)
	Protodown string `json:"protodown"`
}

type InterfaceLLDP struct {
	// Keyed by the neighbor system name
	Neighbor map[string]LLDPNeighbor `json:"neighbor"`
}

type LLDPNeighbor struct {
	// Seconds since initial discovery
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
	Name        string `json:"name"`
	Description string `json:"description"`
	// Port ID subtype as advertised, "ifname" or "mac" for example
	Type string `json:"type"`
	TTL  Number `json:"ttl"`
	MTU  Number `json:"mtu"`
}

type LLDPMED struct {
	DeviceType string        `json:"device-type"`
	Inventory  LLDPInventory `json:"inventory"`
}

type LLDPInventory struct {
	Manufacturer     string `json:"manufacturer"`
	Model            string `json:"model"`
	SerialNumber     string `json:"serial-number"`
	SoftwareRevision string `json:"software-revision"`
	FirmwareRevision string `json:"firmware-revision"`
}

// InterfaceCounters is GET /interface/<name>/counters, the listing only carries the Link part
type InterfaceCounters struct {
	InBytes          Counter `json:"in-bytes"`
	InPkts           Counter `json:"in-pkts"`
	InDrops          Counter `json:"in-drops"`
	InErrors         Counter `json:"in-errors"`
	InUnicastPkts    Counter `json:"in-unicast-pkts"`
	InMulticastPkts  Counter `json:"in-multicast-pkts"`
	InBroadcastPkts  Counter `json:"in-broadcast-pkts"`
	OutBytes         Counter `json:"out-bytes"`
	OutPkts          Counter `json:"out-pkts"`
	OutDrops         Counter `json:"out-drops"`
	OutErrors        Counter `json:"out-errors"`
	OutUnicastPkts   Counter `json:"out-unicast-pkts"`
	OutMulticastPkts Counter `json:"out-multicast-pkts"`
	OutBroadcastPkts Counter `json:"out-broadcast-pkts"`

	Link *LinkCounters `json:"link"`
	QoS  *QoSCounters  `json:"qos"`
}

type LinkCounters struct {
	CarrierTransitions Counter `json:"carrier-transitions"`
	CarrierUpCount     Counter `json:"carrier-up-count"`
	CarrierDownCount   Counter `json:"carrier-down-count"`
}

type QoSCounters struct {
	// Keyed by egress traffic class, "0" to "7"
	EgressQueueStats map[string]EgressQueueStats `json:"egress-queue-stats"`
}

type EgressQueueStats struct {
	ECNMarkedFrames    Counter `json:"ecn-marked-frames"`
	TxBytes            Counter `json:"tx-bytes"`
	TxFrames           Counter `json:"tx-frames"`
	TxUCBufferDiscards Counter `json:"tx-uc-buffer-discards"`
	WREDDiscards       Counter `json:"wred-discards"`
}

// InterfaceRates is GET /interface/<name>/rates, utilization is a percentage
type InterfaceRates struct {
	LoadInterval   Number `json:"load-interval"`
	InBitsRate     Number `json:"in-bits-rate"`
	InPktsRate     Number `json:"in-pkts-rate"`
	InUtilization  Number `json:"in-utilization"`
	OutBitsRate    Number `json:"out-bits-rate"`
	OutPktsRate    Number `json:"out-pkts-rate"`
	OutUtilization Number `json:"out-utilization"`
}

// Transceiver status as reported by GET /platform/transceiver/<name>
const TransceiverStatusInserted = "Inserted"

// Transceiver is GET /platform/transceiver/<name>, the listing only carries the vendor fields
type Transceiver struct {
	// Form factor, such as "OSFP" or "SFP"
	Identifier string `json:"identifier"`
	Status     string `json:"status"`
	// "Passive copper cable", "Optical module"...
	CableType   string `json:"cable-type"`
	CableLength Number `json:"cable-length"`
	// Such as "ETH_400GBASE_CR4"
	PMDType            string `json:"pmd-type"`
	DiagnosticsStatus  string `json:"diagnostics-status"`
	ErrorStatus        string `json:"error-status"`
	RevisionCompliance string `json:"revision-compliance"`
	FWVersion          string `json:"fw-version"`
	VendorName         string `json:"vendor-name"`
	VendorPN           string `json:"vendor-pn"`
	VendorSN           string `json:"vendor-sn"`
	VendorRev          string `json:"vendor-rev"`
	VendorOUI          string `json:"vendor-oui"`
	VendorDateCode     string `json:"vendor-date-code"`

	// Only modules with diagnostics report these
	Temperature *TransceiverTemperature `json:"temperature"`
	Voltage     *TransceiverVoltage     `json:"voltage"`
	// Keyed "channel-1" and up
	Channel map[string]TransceiverChannel `json:"channel"`
}

// TransceiverThresholds are shared by every transceiver sensor, the units match the measured value
type TransceiverThresholds struct {
	Alarm       string `json:"alarm"`
	HighAlarm   Number `json:"high-alarm-threshold"`
	LowAlarm    Number `json:"low-alarm-threshold"`
	HighWarning Number `json:"high-warning-threshold"`
	LowWarning  Number `json:"low-warning-threshold"`
}

type TransceiverTemperature struct {
	TransceiverThresholds
	// Celsius, "28.80 C"
	Temperature Number `json:"temperature"`
}

type TransceiverVoltage struct {
	TransceiverThresholds
	// Volts, "3.2766 V"
	Voltage Number `json:"voltage"`
}

type TransceiverChannel struct {
	RxPower       *TransceiverPower `json:"rx-power"`
	TxPower       *TransceiverPower `json:"tx-power"`
	TxBiasCurrent *TransceiverBias  `json:"tx-bias-current"`
	RxLOS         string            `json:"rx-los"`
	TxFault       string            `json:"tx-fault"`
}

type TransceiverPower struct {
	TransceiverThresholds
	Power Power `json:"power"`
}

type TransceiverBias struct {
	TransceiverThresholds
	// Milliamps, "5.480 mA"
	Current Number `json:"current"`
}

// Platform is GET /platform
type Platform struct {
	ASICModel    string `json:"asic-model"`
	CPU          string `json:"cpu"`
	DiskSize     string `json:"disk-size"`
	Manufacturer string `json:"manufacturer"`
	Memory       string `json:"memory"`
	PartNumber   string `json:"part-number"`
	PortLayout   string `json:"port-layout"`
	SerialNumber string `json:"serial-number"`
	SystemMAC    string `json:"system-mac"`
	SystemType   string `json:"system-type"`
	SystemUUID   string `json:"system-uuid"`
}

// Environment component states as reported under GET /platform/environment
const (
	EnvironmentStateOK     = "ok"
	EnvironmentStateAbsent = "absent"
)

// Fan is one entry of GET /platform/environment/fan, speeds are RPM
type Fan struct {
	CurrentSpeed Number `json:"current-speed"`
	MaxSpeed     Number `json:"max-speed"`
	MinSpeed     Number `json:"min-speed"`
	Direction    string `json:"direction"`
	State        string `json:"state"`
}

// PSU is one entry of GET /platform/environment/psu, the electrical values are on the DC output side
type PSU struct {
	Capacity Number `json:"capacity"`
	Current  Number `json:"current"`
	Power    Number `json:"power"`
	Voltage  Number `json:"voltage"`
	State    string `json:"state"`
}

// Temperature is one entry of GET /platform/environment/temperature, in Celsius
type Temperature struct {
	Current Number `json:"current"`
	Crit    Number `json:"crit"`
	Max     Number `json:"max"`
	Min     Number `json:"min"`
	State   string `json:"state"`
}

// Firmware is one entry of GET /platform/firmware
type Firmware struct {
	ActualFirmware string `json:"actual-firmware"`
	FWSource       string `json:"fw-source"`
	PartNumber     string `json:"part-number"`
}

// System is GET /system, only the identity and version parts are modeled
type System struct {
	Hostname       string         `json:"hostname"`
	FQDN           string         `json:"fqdn"`
	Platform       string         `json:"platform"`
	ProductName    string         `json:"product-name"`
	ProductRelease string         `json:"product-release"`
	Status         string         `json:"status"`
	Uptime         Number         `json:"uptime"`
	Version        SystemVersion  `json:"version"`
	Health         SystemHealth   `json:"health"`
	DateTime       SystemDateTime `json:"date-time"`
}

type SystemDateTime struct {
	LocalTime string `json:"local-time"`
	// IANA name, such as "Etc/UTC"
	Timezone string `json:"timezone"`
}

type SystemVersion struct {
	BaseOS         string             `json:"base-os"`
	Kernel         string             `json:"kernel"`
	ONIE           string             `json:"onie"`
	ProductRelease string             `json:"product-release"`
	Image          SystemVersionImage `json:"image"`
}

type SystemVersionImage struct {
	BuildDate string `json:"build-date"`
	BuildID   string `json:"build-id"`
}

type SystemHealth struct {
	Status    string                       `json:"status"`
	StatusLED string                       `json:"status-led"`
	Issues    map[string]SystemHealthIssue `json:"issues"`
}

type SystemHealthIssue struct {
	Issue string `json:"issue"`
}

// BGP neighbor values as reported by GET /vrf/<vrf>/router/bgp/neighbor
const (
	BGPNeighborTypeUnnumbered = "unnumbered"
	BGPRemoteASInternal       = "internal"
	BGPRemoteASExternal       = "external"
	StateEnabled              = "enabled"
	StateDisabled             = "disabled"
)

// BGPInstance is GET /vrf/<vrf>/router/bgp, which carries only a summary of each neighbor, see BGPNeighbor
type BGPInstance struct {
	AutonomousSystem     Number                 `json:"autonomous-system"`
	RouterID             string                 `json:"router-id"`
	ConfiguredNeighbors  Number                 `json:"configured-neighbors"`
	EstablishedNeighbors Number                 `json:"established-neighbors"`
	Neighbor             map[string]BGPNeighbor `json:"neighbor"`
}

// BGPPeerGroup is GET /vrf/<vrf>/router/bgp/peer-group/<id>, Type is "internal" or "external" and is known even
// while the sessions of its members are still coming up
type BGPPeerGroup struct {
	Type        string `json:"type"`
	RemoteAS    Number `json:"remote-as"`
	MemberCount Number `json:"member-count"`
}

// BGPNeighbor is GET /vrf/<vrf>/router/bgp/neighbor/<id>, the id is an address or an interface for unnumbered. The
// listings only fill the session state, uptime, remote AS, message totals and prefix counts.
type BGPNeighbor struct {
	State       string `json:"state"`
	Description string `json:"description"`
	// "numbered" or "unnumbered"
	Type      string `json:"type"`
	PeerGroup string `json:"peer-group"`
	// A number, or "auto", "internal" or "external"
	RemoteAS Number                      `json:"remote-as"`
	LocalAS  *BGPLocalAS                 `json:"local-as"`
	Shutdown string                      `json:"shutdown"`
	Timers   *BGPTimers                  `json:"timers"`
	Nexthop  *BGPNexthop                 `json:"nexthop"`
	Messages *BGPMsgStats                `json:"message-stats"`
	AF       map[string]BGPAddressFamily `json:"address-family"`

	SessionState string `json:"session-state"`
	// Milliseconds
	Uptime                 Number `json:"uptime"`
	ConnectionsEstablished Number `json:"connections-established"`
	ConnectionsDropped     Number `json:"connections-dropped"`
	// Milliseconds since the reset
	LastResetTime   Number `json:"last-reset-time"`
	LastResetReason string `json:"last-reset-reason"`
	LastResetCode   Number `json:"last-reset-code"`
	RemoteRouterID  string `json:"remote-router-id"`
	RemoteHostname  string `json:"remote-hostname"`
	LocalIP         string `json:"local-ip"`
	RemoteIP        string `json:"remote-ip"`
	LocalPort       Number `json:"local-port"`
	RemotePort      Number `json:"remote-port"`
}

type BGPLocalAS struct {
	State string `json:"state"`
	ASN   Number `json:"asn"`
}

type BGPTimers struct {
	Keepalive Number `json:"keepalive"`
	Hold      Number `json:"hold"`
}

type BGPNexthop struct {
	IPv4       string `json:"ipv4"`
	IPv6Global string `json:"ipv6-global"`
	IPv6Local  string `json:"ipv6-local"`
}

type BGPMsgStats struct {
	InputQueue       Number `json:"input-queue"`
	OutputQueue      Number `json:"output-queue"`
	RxOpens          Number `json:"rx-opens"`
	TxOpens          Number `json:"tx-opens"`
	RxKeepalives     Number `json:"rx-keepalives"`
	TxKeepalives     Number `json:"tx-keepalives"`
	RxRouteRefreshes Number `json:"rx-route-refreshes"`
	TxRouteRefreshes Number `json:"tx-route-refreshes"`
	RxTotal          Number `json:"rx-total"`
	TxTotal          Number `json:"tx-total"`
}

// BGPAddressFamily is keyed "ipv4-unicast", "ipv6-unicast" or "l2vpn-evpn" in BGPNeighbor.AF
type BGPAddressFamily struct {
	State    string `json:"state"`
	TxPrefix Number `json:"tx-prefix"`
	RxPrefix Number `json:"rx-prefix"`
}
