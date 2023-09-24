package vpc

import (
	"bytes"
	"context"
	"html/template"
	"net"

	"github.com/pkg/errors"
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
	VLAN       uint16
	RangeStart string
	RangeEnd   string
	Router     string
}

func (r *VPCReconciler) updateDHCPConfig(ctx context.Context) error {
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
		if !vpc.Spec.DHCP.Enable || vpc.Status.VLAN == 0 {
			continue
		}

		cidr, err := iputil.ParseCIDR(vpc.Spec.Subnet)
		if err != nil {
			return errors.Wrapf(err, "error parsing vpc subnet %s", vpc.Spec.Subnet)
		}

		start := cidr.RangeStart.String()
		end := ""

		if vpc.Spec.DHCP.Range != nil {
			if vpc.Spec.DHCP.Range.Start != nil {
				start = *vpc.Spec.DHCP.Range.Start
			}
			if vpc.Spec.DHCP.Range.End != nil {
				end = *vpc.Spec.DHCP.Range.End
			}
		}

		cfg.Subnets = append(cfg.Subnets, dhcpdSubnet{
			Subnet:     cidr.Subnet.IP.String(),
			Mask:       net.IP(cidr.Subnet.Mask).String(),
			VLAN:       vpc.Status.VLAN,
			Router:     cidr.Gateway.String(),
			RangeStart: start,
			RangeEnd:   end,
		})
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
