// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewayTopology is where a Gateway sits in the fabric topology
type GatewayTopology struct {
	// Fabric is the name of the Fabric this Gateway belongs to (if not specified, "default" is used)
	Fabric string `json:"fabric,omitempty"`
	// Domain is the Fabric domain (spine layer) this Gateway is cabled into (if not specified, "default" is used).
	// It is immutable
	Domain string `json:"domain,omitempty"`
}

// GatewaySpec defines the desired state of Gateway.
type GatewaySpec struct {
	// Topology is where the Gateway sits in the fabric topology
	Topology GatewayTopology `json:"topology,omitempty"`
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
	// Profiling defines the configuration for profiling
	Profiling GatewayProfiling `json:"profiling,omitempty"`
	// Workers defines the number of worker threads to use for dataplane
	Workers uint8 `json:"workers,omitempty"`
	// Groups is a list of group memberships for the gateway
	Groups []GatewayGroupMembership `json:"groups,omitempty"`
	// FlowTableCapacity defines the maximum number of flow entres
	FlowTableCapacity uint32 `json:"flowTableCapacity,omitempty"`
	// Driver is the dataplane driver for the interfaces, kernel or dpdk
	Driver GatewayDriver `json:"driver,omitempty"`
}

type GatewayDriver string

const (
	GatewayDriverKernel GatewayDriver = "kernel"
	GatewayDriverDPDK   GatewayDriver = "dpdk"
)

var GatewayDrivers = []GatewayDriver{
	GatewayDriverKernel,
	GatewayDriverDPDK,
}

// GatewayInterface defines the configuration for a gateway interface
type GatewayInterface struct {
	// PCI address of the interface (required for dpdk driver, optional for kernel driver), e.g. 0000:00:01.0
	PCI string `json:"pci,omitempty"`
	// Kernel is the kernel name of the interface to use (kernel driver only, ignored for dpdk driver or if it's the same
	// as the interface name), e.g. enp2s1
	Kernel string `json:"kernel,omitempty"`
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
	Default   GatewayLogLevel            `json:"default,omitempty"`
	Tags      map[string]GatewayLogLevel `json:"tags,omitempty"`
	RateLimit *GatewayLogRateLimit       `json:"rateLimit,omitempty"`
}

// GatewayLogRateLimit configures the token-bucket rate limiter applied to logs
type GatewayLogRateLimit struct {
	// +kubebuilder:validation:Minimum=0
	// token bucket capacity (default 50)
	Burst uint32 `json:"burst,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// ReplenishPerSecond is the number of tokens (messages) replenished per second (default 5)
	ReplenishPerSecond uint32 `json:"replenishPerSecond,omitempty"`
}

const (
	DefaultGatewayLogRateLimitBurst              uint32 = 50
	DefaultGatewayLogRateLimitReplenishPerSecond uint32 = 5
)

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

type GatewayProfiling struct {
	Enabled bool `json:"enabled,omitempty"`
}

type GatewayGroupMembership struct {
	// Name is the name of the group to which the gateway belongs
	Name string `json:"name,omitempty"`
	// Priority is the priority of the gateway within the group
	Priority uint32 `json:"priority,omitempty"`
}

// GatewayStatus defines the observed state of Gateway.
type GatewayStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;hedgehog-gateway,shortName=gw
// +kubebuilder:printcolumn:name="Fabric",type=string,JSONPath=`.spec.topology.fabric`,priority=0
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.topology.domain`,priority=0
// +kubebuilder:printcolumn:name="VTEPIP",type=string,JSONPath=`.spec.vtepIP`,priority=0
// +kubebuilder:printcolumn:name="Groups",type=string,JSONPath=`.spec.groups`,priority=0
// +kubebuilder:printcolumn:name="Workers",type=integer,JSONPath=`.spec.workers`,priority=1
// +kubebuilder:printcolumn:name="ProtoIP",type=string,JSONPath=`.spec.protocolIP`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
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
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &Gateway{}, &GatewayList{})

		return nil
	})
}

func (gw *Gateway) Default() {
	if gw.Namespace == "" {
		gw.Namespace = kmetav1.NamespaceDefault
	}

	if gw.Spec.Logs.Default == "" {
		gw.Spec.Logs.Default = GatewayLogLevelInfo
	}
	if len(gw.Spec.Logs.Tags) == 0 {
		gw.Spec.Logs.Tags = map[string]GatewayLogLevel{
			"all": GatewayLogLevelInfo,
		}
	}
	if rl := gw.Spec.Logs.RateLimit; rl != nil {
		if rl.Burst == 0 {
			rl.Burst = DefaultGatewayLogRateLimitBurst
		}
		if rl.ReplenishPerSecond == 0 {
			rl.ReplenishPerSecond = DefaultGatewayLogRateLimitReplenishPerSecond
		}
	}
	if gw.Spec.Workers == 0 {
		gw.Spec.Workers = 4
	}

	if gw.Spec.Driver == "" {
		// keeps the driver of the gateways created before it was explicit, it was dpdk with only PCI addresses set
		pcis, kernels := 0, 0
		for _, iface := range gw.Spec.Interfaces {
			if iface.PCI != "" {
				pcis++
			}
			if iface.Kernel != "" {
				kernels++
			}
		}

		gw.Spec.Driver = GatewayDriverKernel
		if pcis > 0 && kernels == 0 {
			gw.Spec.Driver = GatewayDriverDPDK
		}
	}

	if gw.Spec.Topology.Fabric == "" {
		gw.Spec.Topology.Fabric = wiringapi.DefaultFabric
	}
	if gw.Spec.Topology.Domain == "" {
		gw.Spec.Topology.Domain = wiringapi.DefaultFabricDomain
	}

	if gw.Labels == nil {
		gw.Labels = map[string]string{}
	}

	wiringapi.CleanupFabricLabels(gw.Labels)

	gw.Labels[wiringapi.ListLabelFabric(gw.Spec.Topology.Fabric)] = ListLabelValue
	gw.Labels[wiringapi.ListLabelDomain(gw.Spec.Topology.Domain)] = ListLabelValue

	slices.SortFunc(gw.Spec.Groups, func(a, b GatewayGroupMembership) int {
		return strings.Compare(a.Name, b.Name)
	})

	if len(gw.Spec.Groups) == 0 {
		gw.Spec.Groups = []GatewayGroupMembership{
			{Name: DefaultGatewayGroup, Priority: 0},
		}
	}
}

var linuxIfaceNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]{0,8}[a-zA-Z0-9]$`)

// MaxGatewayNameLength leaves room for the gateway's daemonset names, gw--<name>--dataplane at most (from older releases),
// which are used as the app.kubernetes.io/name label value of their pods as well, and label values are capped at 63
// characters
const MaxGatewayNameLength = 63 - len("gw--") - len("--dataplane")

// DaemonSetName is the name of the daemonset running the gateway's dataplane (incl. FRR) in the gateway namespace, it's
// the app.kubernetes.io/name label value of its pods as well
func DaemonSetName(gwName string) string {
	return "gw--" + gwName
}

func (gw *Gateway) Validate(ctx context.Context, kube kclient.Reader, fabricCfg *meta.FabricConfig) error {
	spec := field.NewPath("spec")
	domainPath := spec.Child("topology", "domain")
	protoIPPath, vtepIPPath, vtepMACPath := spec.Child("protocolIP"), spec.Child("vtepIP"), spec.Child("vtepMAC")

	if fabricCfg != nil && !fabricCfg.EnableGateway {
		return field.Forbidden(spec, "gateway support is not enabled")
	}
	if err := meta.ValidateObjectMetadata(gw); err != nil {
		return err //nolint:wrapcheck
	}
	if len(gw.Name) > MaxGatewayNameLength {
		return field.TooLongCharacters(field.NewPath("metadata", "name"), gw.Name, MaxGatewayNameLength)
	}

	fabric, err := wiringapi.CheckFabricExists(ctx, kube, gw.Namespace, gw.Spec.Topology.Fabric)
	if err != nil {
		return err //nolint:wrapcheck
	}
	if gw.Spec.Topology.Domain == "" {
		return field.Required(domainPath, "")
	}

	if gw.Spec.Workers == 0 || gw.Spec.Workers > 64 {
		return field.Invalid(spec.Child("workers"), gw.Spec.Workers, "should be between 1 and 64")
	}

	if rl := gw.Spec.Logs.RateLimit; rl != nil {
		rlPath := spec.Child("logs", "rateLimit")
		if rl.Burst == 0 {
			return field.Invalid(rlPath.Child("burst"), rl.Burst, "must be greater than 0")
		}
		if rl.ReplenishPerSecond == 0 {
			return field.Invalid(rlPath.Child("replenishPerSecond"), rl.ReplenishPerSecond, "must be greater than 0")
		}
	}

	protoIP, err := netip.ParsePrefix(gw.Spec.ProtocolIP)
	if err != nil {
		return field.Invalid(protoIPPath, gw.Spec.ProtocolIP, err.Error())
	}
	if protoIP.Bits() != 32 {
		return field.Invalid(protoIPPath, gw.Spec.ProtocolIP, "must be a /32 prefix")
	}
	if !protoIP.Addr().Is4() {
		return field.Invalid(protoIPPath, gw.Spec.ProtocolIP, "must be an IPv4 address")
	}

	vtepIP, err := netip.ParsePrefix(gw.Spec.VTEPIP)
	if err != nil {
		return field.Invalid(vtepIPPath, gw.Spec.VTEPIP, err.Error())
	}
	if vtepIP.Bits() != 32 {
		return field.Invalid(vtepIPPath, gw.Spec.VTEPIP, "must be a /32 prefix")
	}
	if !vtepIP.Addr().Is4() {
		return field.Invalid(vtepIPPath, gw.Spec.VTEPIP, "must be an IPv4 address")
	}
	if vtepIP.Addr().IsMulticast() || vtepIP.Addr().IsLoopback() || vtepIP.Addr().IsUnspecified() {
		return field.Invalid(vtepIPPath, gw.Spec.VTEPIP, "must be a unicast IPv4 address")
	}
	localhostNet, err := netip.ParsePrefix("127.0.0.0/8")
	if err != nil {
		return fmt.Errorf("internal error: cannot parse localhost network: %w", err)
	}
	if localhostNet.Contains(vtepIP.Addr()) {
		return field.Invalid(vtepIPPath, gw.Spec.VTEPIP, "must not be in the localhost range")
	}

	if gw.Spec.VTEPMAC == "" {
		return field.Required(vtepMACPath, "")
	}
	vtepMAC, err := net.ParseMAC(gw.Spec.VTEPMAC)
	if err != nil {
		return field.Invalid(vtepMACPath, gw.Spec.VTEPMAC, err.Error())
	}
	if vtepMAC.String() == "00:00:00:00:00:00" {
		return field.Invalid(vtepMACPath, gw.Spec.VTEPMAC, "must not be all zeros")
	}
	if (vtepMAC[0] & 1) == 1 {
		return field.Invalid(vtepMACPath, gw.Spec.VTEPMAC, "must be a unicast MAC address")
	}

	if gw.Spec.ASN == 0 {
		return field.Required(spec.Child("asn"), "")
	}
	if fabricCfg != nil && fabric != nil {
		// leaves peer with every gateway of their domain using the domain gateway ASN
		domainName := gw.Spec.Topology.Domain
		domain, exists := fabric.Domains[domainName]
		if !exists {
			return field.Invalid(domainPath, domainName, fmt.Sprintf("domain %s not found in fabric %s", domainName, gw.Spec.Topology.Fabric))
		}
		if gw.Spec.ASN != domain.GatewayASN {
			return field.Invalid(spec.Child("asn"), gw.Spec.ASN, fmt.Sprintf("not the gateway ASN %d of domain %s", domain.GatewayASN, domainName))
		}
	}

	if !slices.Contains(GatewayDrivers, gw.Spec.Driver) {
		return field.NotSupported(spec.Child("driver"), gw.Spec.Driver, GatewayDrivers)
	}

	if len(gw.Spec.Interfaces) == 0 {
		return field.Required(spec.Child("interfaces"), "at least one interface must be defined")
	}
	pcis := map[string]string{}
	for name, iface := range gw.Spec.Interfaces {
		ifacePath := spec.Child("interfaces").Key(name)
		if !linuxIfaceNameRegex.MatchString(name) {
			return field.Invalid(ifacePath, name, "must be a valid linux interface name (2-10 characters)")
		}

		if len(iface.IPs) == 0 {
			return field.Required(ifacePath.Child("ips"), "at least one IP address is required")
		}
		for idx, ifaceIP := range iface.IPs {
			ifaceIP, err := netip.ParsePrefix(ifaceIP)
			if err != nil {
				return field.Invalid(ifacePath.Child("ips").Index(idx), iface.IPs[idx], err.Error())
			}
			if !ifaceIP.Addr().Is4() {
				return field.Invalid(ifacePath.Child("ips").Index(idx), iface.IPs[idx], "must be an IPv4 address")
			}
		}

		// same range as the dataplane accepts, 0 means unset
		if iface.MTU != 0 && (iface.MTU < 68 || iface.MTU > math.MaxUint16) {
			return field.Invalid(ifacePath.Child("mtu"), iface.MTU, fmt.Sprintf("must be between 68 and %d", math.MaxUint16))
		}

		// kernel name is allowed but ignored with the dpdk driver
		if gw.Spec.Driver == GatewayDriverDPDK && iface.PCI == "" {
			return field.Required(ifacePath.Child("pci"), "required with the dpdk driver")
		}
		if iface.PCI != "" {
			if err := validatePCI(ifacePath.Child("pci"), iface.PCI); err != nil {
				return err
			}

			pci := strings.ToLower(iface.PCI)
			if other, exist := pcis[pci]; exist {
				return field.Invalid(ifacePath.Child("pci"), iface.PCI, "same PCI address as interface "+other)
			}
			pcis[pci] = name
		}
		if iface.Kernel != "" && !linuxIfaceNameRegex.MatchString(iface.Kernel) {
			return field.Invalid(ifacePath.Child("kernel"), iface.Kernel, "must be a valid linux interface name (2-10 characters)")
		}
	}

	if len(gw.Spec.Neighbors) == 0 {
		return field.Required(spec.Child("neighbors"), "at least one BGP neighbor must be defined")
	}
	for idx, neigh := range gw.Spec.Neighbors {
		neighPath := spec.Child("neighbors").Index(idx)
		if neigh.IP == "" {
			return field.Required(neighPath.Child("ip"), "")
		}
		neighIP, err := netip.ParseAddr(neigh.IP)
		if err != nil {
			return field.Invalid(neighPath.Child("ip"), neigh.IP, err.Error())
		}
		if !neighIP.Is4() {
			return field.Invalid(neighPath.Child("ip"), neigh.IP, "must be an IPv4 address")
		}
		if neighIP.IsMulticast() || neighIP.IsUnspecified() {
			return field.Invalid(neighPath.Child("ip"), neigh.IP, "must be a unicast IPv4 address")
		}

		if neigh.ASN == 0 {
			return field.Required(neighPath.Child("asn"), "")
		}
	}

	if len(gw.Spec.Groups) == 0 {
		return field.Required(spec.Child("groups"), "at least one gateway group must be defined")
	}
	usedGwGroups := map[string]bool{}
	for idx, gwGroup := range gw.Spec.Groups {
		if usedGwGroups[gwGroup.Name] {
			return field.Duplicate(spec.Child("groups").Index(idx).Child("name"), gwGroup.Name)
		}
		usedGwGroups[gwGroup.Name] = true
	}

	// uniqueness checks
	if kube != nil {
		protocolIPs := map[netip.Addr]bool{}
		vtepIPs := map[netip.Addr]bool{}
		vtepMACs := map[string]bool{}
		gwGroupMembers := map[string]int{}
		gateways := &GatewayList{}
		switches := &wiringapi.SwitchList{}
		if err := kube.List(ctx, gateways); err != nil {
			return fmt.Errorf("listing gateways: %w", err)
		}
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
			if other.Spec.VTEPMAC != "" {
				if mac, err := net.ParseMAC(other.Spec.VTEPMAC); err == nil {
					vtepMACs[mac.String()] = true
				}
			}
			for _, group := range other.Spec.Groups {
				gwGroupMembers[group.Name]++
			}
		}
		// ProtocolIP and VTEPIP ranges are shared with switches too
		if err := kube.List(ctx, switches); err != nil {
			return fmt.Errorf("listing switches: %w", err)
		}
		for _, sw := range switches.Items {
			if sw.Spec.ProtocolIP != "" {
				if ip, err := netip.ParsePrefix(sw.Spec.ProtocolIP); err == nil {
					protocolIPs[ip.Addr()] = true
				}
			}
			if sw.Spec.VTEPIP != "" {
				if ip, err := netip.ParsePrefix(sw.Spec.VTEPIP); err == nil {
					vtepIPs[ip.Addr()] = true
				}
			}
		}

		if _, exist := protocolIPs[protoIP.Addr()]; exist {
			return field.Invalid(protoIPPath, gw.Spec.ProtocolIP, "already in use")
		}
		if _, exist := vtepIPs[vtepIP.Addr()]; exist {
			return field.Invalid(vtepIPPath, gw.Spec.VTEPIP, "already in use")
		}
		if _, exist := vtepMACs[vtepMAC.String()]; exist {
			return field.Invalid(vtepMACPath, gw.Spec.VTEPMAC, "already in use")
		}

		gwGroupList := &GatewayGroupList{}
		if err := kube.List(ctx, gwGroupList, kclient.InNamespace(kmetav1.NamespaceDefault)); err != nil {
			return fmt.Errorf("listing gateway groups: %w", err)
		}
		gwGroupTopologies := map[string]GatewayGroupTopology{}
		for _, gwGroup := range gwGroupList.Items {
			gwGroupTopologies[gwGroup.Name] = gwGroup.Spec.Topology
		}
		gwFabric := gw.Spec.Topology.Fabric
		gwDomain := gw.Spec.Topology.Domain
		for idx, gwGroup := range gw.Spec.Groups {
			groupPath := spec.Child("groups").Index(idx).Child("name")
			groupTopology, exists := gwGroupTopologies[gwGroup.Name]
			if !exists {
				return field.NotFound(groupPath, gwGroup.Name)
			}
			if groupFabric := groupTopology.Fabric; groupFabric != gwFabric {
				return field.Invalid(groupPath, gwGroup.Name, fmt.Sprintf("gateway is in fabric %s but the gateway group is in fabric %s", gwFabric, groupFabric))
			}
			if groupDomain := groupTopology.Domain; groupDomain != gwDomain {
				return field.Invalid(groupPath, gwGroup.Name, fmt.Sprintf("gateway is in domain %s but the gateway group is in domain %s", gwDomain, groupDomain))
			}
			if fabricCfg != nil && len(fabricCfg.GatewayCommunities) > 0 && gwGroupMembers[gwGroup.Name] >= len(fabricCfg.GatewayCommunities) {
				return field.Invalid(groupPath, gwGroup.Name, fmt.Sprintf("gateway group already has too many members (%d), max is %d", gwGroupMembers[gwGroup.Name], len(fabricCfg.GatewayCommunities)))
			}
		}
	}

	return nil
}

func validatePCI(path *field.Path, pci string) error {
	invalidPCI := field.Invalid(path, pci, "must be a valid PCI address (e.g. 0000:02:00.0)")
	if len(pci) != 12 {
		return invalidPCI
	}

	parts := strings.Split(pci, ".")
	if len(parts) != 2 {
		return invalidPCI
	}

	if function, err := strconv.ParseInt(parts[1], 16, 64); err != nil {
		return field.Invalid(path, pci, "invalid function: "+err.Error())
	} else if function < 0 || function > 7 {
		return field.Invalid(path, pci, "invalid function (0-7)")
	}

	parts = strings.Split(parts[0], ":")
	if len(parts) != 3 {
		return invalidPCI
	}

	if domain, err := strconv.ParseInt(parts[0], 16, 64); err != nil {
		return field.Invalid(path, pci, "invalid domain: "+err.Error())
	} else if domain < 0 || domain > 0xFFFF {
		return field.Invalid(path, pci, "invalid domain (0-FFFF)")
	}

	if bus, err := strconv.ParseInt(parts[1], 16, 64); err != nil {
		return field.Invalid(path, pci, "invalid bus: "+err.Error())
	} else if bus < 0 || bus > 0xFF {
		return field.Invalid(path, pci, "invalid bus (0-FF)")
	}

	if device, err := strconv.ParseInt(parts[2], 16, 64); err != nil {
		return field.Invalid(path, pci, "invalid device: "+err.Error())
	} else if device < 0 || device > 0x1F {
		return field.Invalid(path, pci, "invalid device (0-1F)")
	}

	return nil
}
