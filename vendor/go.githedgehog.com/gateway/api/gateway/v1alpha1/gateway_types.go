// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrInvalidGW = errors.New("invalid gateway")

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewaySpec defines the desired state of Gateway.
type GatewaySpec struct {
	// ProtocolIP is used as a loopback IP and BGP Router ID
	ProtocolIP string `json:"protocolIP,omitempty"`
	// VTEP IP to be used by the gateway
	VTEPIP string `json:"vtepIP,omitempty"`
	// VTEP MAC address to be used by the gateway
	VTEPMAC string `json:"vtepMAC,omitempty"`
	// ASN is the ASN of the gateway
	ASN uint32 `json:"asn,omitempty"`
	// VTEPMTU is the MTU for the VTEP interface
	VTEPMTU uint32 `json:"vtepMTU,omitempty"`
	// Interfaces is a map of interface names to their configurations
	Interfaces map[string]GatewayInterface `json:"interfaces,omitempty"`
	// Neighbors is a list of BGP neighbors
	Neighbors []GatewayBGPNeighbor `json:"neighbors,omitempty"`
	// Logs defines the configuration for logging levels
	Logs GatewayLogs `json:"logs,omitempty"`
	// Workers defines the number of worker threads to use for dataplane
	Workers uint8 `json:"workers,omitempty"`
}

// GatewayInterface defines the configuration for a gateway interface
type GatewayInterface struct {
	// IPs is the list of IP address to assign to the interface
	IPs []string `json:"ips,omitempty"`
	// MTU for the interface
	MTU uint32 `json:"mtu,omitempty"`
}

// GatewayBGPNeighbor defines the configuration for a BGP neighbor
type GatewayBGPNeighbor struct {
	// Source is the source interface for the BGP neighbor configuration
	Source string `json:"source,omitempty"`
	// IP is the IP address of the BGP neighbor
	IP string `json:"ip,omitempty"`
	// ASN is the remote ASN of the BGP neighbor
	ASN uint32 `json:"asn,omitempty"`
}

// GatewayLogs defines the configuration for logging levels
type GatewayLogs struct {
	Default GatewayLogLevel            `json:"default,omitempty"`
	Tags    map[string]GatewayLogLevel `json:"tags,omitempty"`
}

type GatewayLogLevel string

const (
	GatewayLogLevelOff     GatewayLogLevel = "off"
	GatewayLogLevelError   GatewayLogLevel = "error"
	GatewayLogLevelWarning GatewayLogLevel = "warning"
	GatewayLogLevelInfo    GatewayLogLevel = "info"
	GatewayLogLevelDebug   GatewayLogLevel = "debug"
	GatewayLogLevelTrace   GatewayLogLevel = "trace"
)

var GatewayLogLevels = []GatewayLogLevel{
	GatewayLogLevelOff,
	GatewayLogLevelError,
	GatewayLogLevelWarning,
	GatewayLogLevelInfo,
	GatewayLogLevelDebug,
	GatewayLogLevelTrace,
}

// GatewayStatus defines the observed state of Gateway.
type GatewayStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;hedgehog-gateway,shortName=gw
// +kubebuilder:printcolumn:name="ProtoIP",type=string,JSONPath=`.spec.protocolIP`,priority=1
// +kubebuilder:printcolumn:name="VTEPIP",type=string,JSONPath=`.spec.vtepIP`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// Gateway is the Schema for the gateways API.
type Gateway struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewaySpec   `json:"spec,omitempty"`
	Status GatewayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayList contains a list of Gateway.
type GatewayList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []Gateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Gateway{}, &GatewayList{})
}

func (gw *Gateway) Default() {
	if gw.Spec.Logs.Default == "" {
		gw.Spec.Logs.Default = GatewayLogLevelInfo
	}
	if len(gw.Spec.Logs.Tags) == 0 {
		gw.Spec.Logs.Tags = map[string]GatewayLogLevel{
			"all": GatewayLogLevelInfo,
		}
	}
	if gw.Spec.Workers == 0 {
		gw.Spec.Workers = 4
	}
}

func (gw *Gateway) Validate(ctx context.Context, kube kclient.Reader) error {
	if gw.Spec.Workers == 0 || gw.Spec.Workers > 64 {
		return fmt.Errorf("workers should be between 1 and 64: %w", ErrInvalidGW)
	}

	protoIP, err := netip.ParsePrefix(gw.Spec.ProtocolIP)
	if err != nil {
		return fmt.Errorf("invalid ProtocolIP %s: %w", gw.Spec.ProtocolIP, errors.Join(err, ErrInvalidGW))
	}
	if protoIP.Bits() != 32 {
		return fmt.Errorf("ProtocolIP %s must be a /32 prefix: %w", gw.Spec.ProtocolIP, ErrInvalidGW)
	}
	if !protoIP.Addr().Is4() {
		return fmt.Errorf("ProtocolIP %s must be an IPv4 address: %w", gw.Spec.ProtocolIP, ErrInvalidGW)
	}

	vtepIP, err := netip.ParsePrefix(gw.Spec.VTEPIP)
	if err != nil {
		return fmt.Errorf("invalid VTEPIP %s: %w", gw.Spec.VTEPIP, errors.Join(err, ErrInvalidGW))
	}
	if vtepIP.Bits() != 32 {
		return fmt.Errorf("VTEPIP %s must be a /32 prefix: %w", gw.Spec.VTEPIP, ErrInvalidGW)
	}
	if !vtepIP.Addr().Is4() {
		return fmt.Errorf("VTEPIP %s must be an IPv4 address: %w", gw.Spec.VTEPIP, ErrInvalidGW)
	}
	if vtepIP.Addr().IsMulticast() || vtepIP.Addr().IsLoopback() || vtepIP.Addr().IsUnspecified() {
		return fmt.Errorf("VTEPIP %s must be a unicast IPv4 address: %w", gw.Spec.VTEPIP, ErrInvalidGW)
	}
	localhostNet, err := netip.ParsePrefix("127.0.0.0/8")
	if err != nil {
		return fmt.Errorf("internal error: cannot parse localhost network: %w", err)
	}
	if localhostNet.Contains(vtepIP.Addr()) {
		return fmt.Errorf("VTEPIP %s must not be in the localhost range: %w", gw.Spec.VTEPIP, ErrInvalidGW)
	}

	if gw.Spec.VTEPMAC == "" {
		return fmt.Errorf("VTEPMAC must be set: %w", ErrInvalidGW)
	}
	vtepMAC, err := net.ParseMAC(gw.Spec.VTEPMAC)
	if err != nil {
		return fmt.Errorf("invalid VTEPMAC %s: %w", gw.Spec.VTEPMAC, errors.Join(err, ErrInvalidGW))
	}
	if vtepMAC.String() == "00:00:00:00:00:00" {
		return fmt.Errorf("VTEPMAC must not be all zeros: %w", ErrInvalidGW)
	}
	if (vtepMAC[0] & 1) == 1 {
		return fmt.Errorf("VTEPMAC %s must be a unicast MAC address: %w", gw.Spec.VTEPMAC, ErrInvalidGW)
	}

	if gw.Spec.ASN == 0 {
		return fmt.Errorf("ASN must be set: %w", ErrInvalidGW)
	}

	if len(gw.Spec.Interfaces) == 0 {
		return fmt.Errorf("at least one interface must be defined: %w", ErrInvalidGW)
	}
	for name, iface := range gw.Spec.Interfaces {
		if len(iface.IPs) == 0 {
			return fmt.Errorf("interface %s must have at least one IP address: %w", name, ErrInvalidGW)
		}
		for _, ifaceIP := range iface.IPs {
			ifaceIP, err := netip.ParsePrefix(ifaceIP)
			if err != nil {
				return fmt.Errorf("invalid interface %s IP %s: %w", name, ifaceIP, errors.Join(err, ErrInvalidGW))
			}
			if !ifaceIP.Addr().Is4() {
				return fmt.Errorf("interface %s IP %s must be an IPv4 address: %w", name, ifaceIP, ErrInvalidGW)
			}
		}
	}

	if len(gw.Spec.Neighbors) == 0 {
		return fmt.Errorf("at least one BGP neighbor must be defined: %w", ErrInvalidGW)
	}
	for _, neigh := range gw.Spec.Neighbors {
		if neigh.IP == "" {
			return fmt.Errorf("BGP neighbor must have an IP address: %w", ErrInvalidGW)
		}
		neighIP, err := netip.ParseAddr(neigh.IP)
		if err != nil {
			return fmt.Errorf("invalid neighbor IP %s: %w", neigh.IP, errors.Join(err, ErrInvalidGW))
		}
		if !neighIP.Is4() {
			return fmt.Errorf("BGP neighbor IP %s must be an IPv4 address: %w", neigh.IP, ErrInvalidGW)
		}
		if neighIP.IsMulticast() || neighIP.IsUnspecified() {
			return fmt.Errorf("BGP neighbor IP %s must be a unicast IPv4 address: %w", neigh.IP, ErrInvalidGW)
		}

		if neigh.ASN == 0 {
			return fmt.Errorf("BGP neighbor %s must have an ASN: %w", neigh.IP, ErrInvalidGW)
		}
	}

	// uniqueness checks
	if kube != nil {
		protocolIPs := map[netip.Addr]bool{}
		vtepIPs := map[netip.Addr]bool{}
		gateways := &GatewayList{}
		if err := kube.List(ctx, gateways); err != nil {
			return fmt.Errorf("listing gateways: %w", err)
		}
		// TODO: check switches too when we remove the circular dependency issue
		for _, other := range gateways.Items {
			if other.Name == gw.Name {
				continue
			}
			if other.Spec.ProtocolIP != "" {
				if ip, err := netip.ParsePrefix(other.Spec.ProtocolIP); err == nil {
					protocolIPs[ip.Addr()] = true
				}
			}
			if other.Spec.VTEPIP != "" {
				if ip, err := netip.ParsePrefix(other.Spec.VTEPIP); err == nil {
					vtepIPs[ip.Addr()] = true
				}
			}
		}
		if _, exist := protocolIPs[protoIP.Addr()]; exist {
			return fmt.Errorf("gateway %s protocol IP %s is already in use: %w", gw.Name, protoIP, ErrInvalidGW)
		}
		if _, exist := vtepIPs[vtepIP.Addr()]; exist {
			return fmt.Errorf("gateway %s VTEP IP %s is already in use: %w", gw.Name, vtepIP, ErrInvalidGW)
		}
	}

	return nil
}
