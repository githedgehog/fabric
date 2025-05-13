// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"net/netip"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewaySpec defines the desired state of Gateway.
type GatewaySpec struct {
	// ProtocolIP is used as a loopback IP and BGP Router ID
	ProtocolIP string `json:"protocolIP,omitempty"`
	// ASN is the ASN of the gateway
	ASN uint32 `json:"asn,omitempty"`
	// Interfaces is a map of interface names to their configurations
	Interfaces map[string]GatewayInterface `json:"interfaces,omitempty"`
	// Neighbors is a list of BGP neighbors
	Neighbors []GatewayBGPNeighbor `json:"neighbors,omitempty"`
}

// GatewayInterface defines the configuration for a gateway interface
type GatewayInterface struct {
	// IP is the IP address to assign to the interface
	IP string `json:"ip,omitempty"`
}

// GatewayBGPNeighbor defines the configuration for a BGP neighbor
type GatewayBGPNeighbor struct {
	// IP is the IP address of the BGP neighbor
	IP string `json:"ip,omitempty"`
	// ASN is the remote ASN of the BGP neighbor
	ASN uint32 `json:"asn,omitempty"`
}

// GatewayStatus defines the observed state of Gateway.
type GatewayStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;hedgehog-gateway,shortName=gw
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
}

func (gw *Gateway) Validate(_ context.Context, _ kclient.Reader) error {
	protoIP, err := netip.ParsePrefix(gw.Spec.ProtocolIP)
	if err != nil {
		return fmt.Errorf("invalid ProtocolIP %s: %w", gw.Spec.ProtocolIP, err)
	}
	if protoIP.Bits() != 32 {
		return fmt.Errorf("ProtocolIP %s must be a /32 prefix", gw.Spec.ProtocolIP) //nolint:goerr113
	}
	if !protoIP.Addr().Is4() {
		return fmt.Errorf("ProtocolIP %s must be an IPv4 address", gw.Spec.ProtocolIP) //nolint:goerr113
	}

	if gw.Spec.ASN == 0 {
		return fmt.Errorf("ASN must be set") //nolint:goerr113
	}

	if len(gw.Spec.Interfaces) == 0 {
		return fmt.Errorf("at least one interface must be defined") //nolint:goerr113
	}
	for name, iface := range gw.Spec.Interfaces {
		if iface.IP == "" {
			return fmt.Errorf("interface %s must have an IP address", name) //nolint:goerr113
		}
		ifaceIP, err := netip.ParsePrefix(iface.IP)
		if err != nil {
			return fmt.Errorf("invalid interface IP %s: %w", iface.IP, err)
		}
		if !ifaceIP.Addr().Is4() {
			return fmt.Errorf("interface %s IP %s must be an IPv4 address", name, iface.IP) //nolint:goerr113
		}
	}

	if len(gw.Spec.Neighbors) == 0 {
		return fmt.Errorf("at least one BGP neighbor must be defined") //nolint:goerr113
	}
	for _, neigh := range gw.Spec.Neighbors {
		if neigh.IP == "" {
			return fmt.Errorf("BGP neighbor must have an IP address") //nolint:goerr113
		}
		neighIP, err := netip.ParseAddr(neigh.IP)
		if err != nil {
			return fmt.Errorf("invalid neighbor IP %s: %w", neigh.IP, err)
		}
		if !neighIP.Is4() {
			return fmt.Errorf("BGP neighbor %s IP %s must be an IPv4 address", neigh.IP, neigh.IP) //nolint:goerr113
		}

		if neigh.ASN == 0 {
			return fmt.Errorf("BGP neighbor %s must have an ASN", neigh.IP) //nolint:goerr113
		}
	}

	return nil
}
