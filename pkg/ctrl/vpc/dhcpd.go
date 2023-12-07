package vpc

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net"
	"strings"

	"github.com/pkg/errors"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1alpha2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	DHCP_SERVER_CONFIF_TMPL = `
default-lease-time 86400;
max-lease-time 86400;

authoritative;

log-facility local7;

{{ range .Subnets }}
{{ if .Empty -}}
subnet {{ .Subnet }} netmask {{ .Mask }} {}
{{- else -}}
class "Vlan{{ .VLAN }}" {
	match if option agent.circuit-id = "Vlan{{ .VLAN }}";
}

subnet {{ .Subnet }} netmask {{ .Mask }} {
	pool {
	allow members of "Vlan{{ .VLAN }}";
	range {{ .RangeStart }} {{ .RangeEnd }};
	option routers {{ .Router }};
	}
}
{{- end }}
{{ end }}
`
)

type dhcpdConfig struct {
	Subnets []dhcpdSubnet
}

type dhcpdSubnet struct {
	Subnet     string
	Mask       string
	Empty      bool
	VLAN       string
	RangeStart string
	RangeEnd   string
	Router     string
}

func (r *VPCReconciler) updateISCDHCPConfig(ctx context.Context) error {
	tmpl, err := template.New("dhcp-server-config").Parse(DHCP_SERVER_CONFIF_TMPL)
	if err != nil {
		return errors.Wrapf(err, "error parsing dhcp server config template")
	}

	cfg := dhcpdConfig{}

	// Add control VIP
	{
		ip, ipNet, err := net.ParseCIDR(r.Cfg.ControlVIP)
		if err != nil {
			return errors.Wrapf(err, "error parsing control vip %s", r.Cfg.ControlVIP)
		}

		cfg.Subnets = append(cfg.Subnets, dhcpdSubnet{
			Subnet: ip.String(),
			Mask:   net.IP(ipNet.Mask).String(),
			Empty:  true,
		})
	}

	// Add management IPs
	conns := &wiringapi.ConnectionList{}
	err = r.List(ctx, conns, client.MatchingLabels{wiringapi.LabelConnectionType: wiringapi.CONNECTION_TYPE_MANAGEMENT})
	if err != nil {
		return errors.Wrapf(err, "error listing connections")
	}

	for _, conn := range conns.Items {
		if conn.Spec.Management != nil {
			_, ipNet, err := net.ParseCIDR(conn.Spec.Management.Link.Server.IP)
			if err != nil {
				return errors.Wrapf(err, "error parsing control link ip %s", conn.Spec.Management.Link.Server.IP)
			}

			cfg.Subnets = append(cfg.Subnets, dhcpdSubnet{
				Subnet: ipNet.IP.String(),
				Mask:   net.IP(ipNet.Mask).String(),
				Empty:  true,
			})
		}
	}

	vpcs := &vpcapi.VPCList{}
	err = r.List(ctx, vpcs)
	if err != nil {
		return errors.Wrapf(err, "error listing vpcs")
	}

	for _, vpc := range vpcs.Items {
		// TODO remove after switching to custom DHCP server
		if vpc.Spec.IPv4Namespace != "default" || vpc.Spec.VLANNamespace != "default" {
			continue
		}

		for subnetName, subnet := range vpc.Spec.Subnets {
			if !subnet.DHCP.Enable || subnet.VLAN == "" {
				continue
			}

			cidr, err := iputil.ParseCIDR(subnet.Subnet)
			if err != nil {
				return errors.Wrapf(err, "error parsing vpc %s/%s subnet %s", vpc.Name, subnetName, subnet.Subnet)
			}

			start := cidr.DHCPRangeStart.String()
			end := cidr.DHCPRangeEnd.String()

			if subnet.DHCP.Range != nil {
				if subnet.DHCP.Range.Start != "" {
					start = subnet.DHCP.Range.Start
				}
				if subnet.DHCP.Range.End != "" {
					end = subnet.DHCP.Range.End
				}
			}

			// TODO add extra range validation

			cfg.Subnets = append(cfg.Subnets, dhcpdSubnet{
				Subnet:     cidr.Subnet.IP.String(),
				Mask:       net.IP(cidr.Subnet.Mask).String(),
				VLAN:       subnet.VLAN,
				Router:     cidr.Gateway.String(),
				RangeStart: start,
				RangeEnd:   end,
			})
		}
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, cfg)
	if err != nil {
		return errors.Wrapf(err, "error executing dhcp server config template")
	}

	dhcpdConfigMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: r.Cfg.DHCPDConfigMap, Namespace: "default"}} // TODO namespace
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, dhcpdConfigMap, func() error {
		dhcpdConfigMap.Data = map[string]string{
			r.Cfg.DHCPDConfigKey: buf.String(),
		}

		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "error creating dhcp server config map")
	}

	return nil
}

func (r *VPCReconciler) updateDHCPSubnets(ctx context.Context, vpc *vpcapi.VPC) error {
	err := r.deleteDHCPSubnets(ctx, client.ObjectKey{Name: vpc.Name, Namespace: vpc.Namespace}, vpc.Spec.Subnets)
	if err != nil {
		return errors.Wrapf(err, "error deleting obsolete dhcp subnets")
	}

	for subnetName, subnet := range vpc.Spec.Subnets {
		if !subnet.DHCP.Enable {
			continue
		}

		cidr, err := iputil.ParseCIDR(subnet.Subnet)
		if err != nil {
			return errors.Wrapf(err, "error parsing vpc %s/%s subnet %s", vpc.Name, subnetName, subnet.Subnet)
		}

		start := cidr.DHCPRangeStart.String()
		end := cidr.DHCPRangeEnd.String()

		if subnet.DHCP.Range != nil {
			if subnet.DHCP.Range.Start != "" {
				start = subnet.DHCP.Range.Start
			}
			if subnet.DHCP.Range.End != "" {
				end = subnet.DHCP.Range.End
			}
		}

		gateway := cidr.Gateway.String()

		dhcp := &dhcpapi.DHCPSubnet{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s--%s", vpc.Name, subnetName), Namespace: vpc.Namespace}}
		_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, dhcp, func() error {
			dhcp.Labels = map[string]string{
				vpcapi.LabelVPC:    vpc.Name,
				vpcapi.LabelSubnet: subnetName,
			}
			dhcp.Spec = dhcpapi.DHCPSubnetSpec{
				Subnet:    fmt.Sprintf("%s/%s", vpc.Name, subnetName),
				CIDRBlock: subnet.Subnet,
				Gateway:   gateway,
				StartIP:   start,
				EndIP:     end,
				VRF:       fmt.Sprintf("VrfV%s", vpc.Name),    // TODO move to utils
				CircuitID: fmt.Sprintf("Vlan%s", subnet.VLAN), // TODO move to utils
			}

			return nil
		})
		if err != nil {
			return errors.Wrapf(err, "error creating dhcp subnet for %s/%s", vpc.Name, subnetName)
		}
	}

	return nil
}

func (r *VPCReconciler) deleteDHCPSubnets(ctx context.Context, vpcKey client.ObjectKey, subnets map[string]*vpcapi.VPCSubnet) error {
	dhcpSubnets := &dhcpapi.DHCPSubnetList{}
	err := r.List(ctx, dhcpSubnets, client.MatchingLabels{vpcapi.LabelVPC: vpcKey.Name})
	if err != nil {
		return errors.Wrapf(err, "error listing dhcp subnets")
	}

	for _, subnet := range dhcpSubnets.Items {
		subnetName := "default"
		parts := strings.Split(subnet.Spec.Subnet, "/")
		if len(parts) == 2 {
			subnetName = parts[1]
		}

		if _, exists := subnets[subnetName]; exists {
			continue
		}

		err = r.Delete(ctx, &subnet)
		if client.IgnoreNotFound(err) != nil {
			return errors.Wrapf(err, "error deleting dhcp subnet %s", subnet.Name)
		}
	}

	return nil
}
