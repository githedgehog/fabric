package controlagent

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AgentReconciler reconciles a Agent object
type ControlAgentReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Cfg     *meta.FabricConfig
	Version string
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *meta.FabricConfig, version string) error {
	r := &ControlAgentReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Cfg:     cfg,
		Version: version,
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("control-agent").
		For(&wiringapi.Server{}).
		Watches(&wiringapi.Connection{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByConnectionServerListLabels)).
		Complete(r)
}

func (r *ControlAgentReconciler) enqueueByConnectionServerListLabels(ctx context.Context, obj client.Object) []reconcile.Request {
	res := []reconcile.Request{}

	labels := obj.GetLabels()

	// we only need to rebuild control agent if it's a management connection
	if labels[wiringapi.LabelConnectionType] != wiringapi.CONNECTION_TYPE_MANAGEMENT {
		return res
	}

	// TODO extract to lib
	serverConnPrefix := wiringapi.ListLabelPrefix(wiringapi.ConnectionLabelTypeServer)

	for label, val := range labels {
		if val != wiringapi.ListLabelValue {
			continue
		}

		if strings.HasPrefix(label, serverConnPrefix) {
			serverName := strings.TrimPrefix(label, serverConnPrefix)
			res = append(res, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      serverName,
			}})
		}
	}

	return res
}

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=controlagents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=controlagents/status,verbs=get;get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=controlagents/finalizers,verbs=update

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=servers,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=servers/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

func (r *ControlAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	server := &wiringapi.Server{}
	err := r.Get(ctx, req.NamespacedName, server)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrapf(err, "error getting server")
	}

	if server.Spec.Type != wiringapi.ServerTypeControl {
		return ctrl.Result{}, nil
	}

	conns := &wiringapi.ConnectionList{}
	err = r.List(ctx, conns, client.InNamespace(server.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting server connections")
	}

	switchList := &wiringapi.SwitchList{}
	err = r.List(ctx, switchList, client.InNamespace(server.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting switch list")
	}
	switches := map[string]wiringapi.Switch{}
	for _, sw := range switchList.Items {
		switches[sw.Name] = sw
	}

	networkd, err := r.buildNetworkd(server.Name, conns, switches)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error building networkd config")
	}

	hosts, err := r.buildHosts(server.Name, switchList.Items)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error building hosts config")
	}

	agent := &agentapi.ControlAgent{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, agent, func() error {
		agent.Spec.ControlVIP = r.Cfg.ControlVIP
		agent.Spec.Version.Default = r.Version
		agent.Spec.Version.Repo = r.Cfg.AgentRepo
		agent.Spec.Version.CA = r.Cfg.AgentRepoCA
		agent.Spec.Networkd = networkd
		agent.Spec.Hosts = hosts

		return nil
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error creating/updating control agent")
	}

	l.Info("control agent reconciled")

	return ctrl.Result{}, nil
}

func (r *ControlAgentReconciler) buildNetworkd(serverName string, conns *wiringapi.ConnectionList, switches map[string]wiringapi.Switch) (map[string]string, error) {
	networkd := map[string]string{}
	var err error

	networkd["00-hh-0--loopback.network"], err = executeTemplate(loopbackNetworkTmpl, networkdConfig{
		ControlVIP: r.Cfg.ControlVIP,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error executing loopback template")
	}

	direct := map[string]bool{}
	for _, conn := range conns.Items {
		if conn.Spec.Management == nil {
			continue
		}

		direct[conn.Spec.Management.Link.Switch.DeviceName()] = true
	}

	chainSwitchIPs := []string{}
	for swName, sw := range switches {
		if direct[swName] {
			continue
		}

		chainSwitchIPs = append(chainSwitchIPs, sw.Spec.IP)
	}

	for _, conn := range conns.Items {
		if conn.Spec.Fabric == nil {
			continue
		}

		for _, link := range conn.Spec.Fabric.Links {
			// if leaf is directly attached, we're adding spine side of the link
			if direct[link.Leaf.DeviceName()] {
				chainSwitchIPs = append(chainSwitchIPs, link.Spine.IP)
			} else { // if leaf is not directly attached, we're adding leaf side of the link
				chainSwitchIPs = append(chainSwitchIPs, link.Leaf.IP)
			}
		}
	}

	for _, conn := range conns.Items {
		if conn.Spec.Management == nil {
			continue
		}

		if conn.Spec.Management.Link.Server.DeviceName() != serverName {
			continue
		}

		link := conn.Spec.Management.Link

		swName := link.Switch.DeviceName()
		swPort := link.Switch.LocalPortName()

		mud := fmt.Sprintf("http://hedgehog/?my_ipnet=%s&your_ipnet=%s&control_vip=%s",
			link.Server.IP, link.Switch.IP, r.Cfg.ControlVIP)

		switchIP := ""
		if sw, ok := switches[swName]; ok {
			switchIP = sw.Spec.IP
		} else {
			return nil, errors.Errorf("switch %s not found but used in conn %s", swName, conn.Name)
		}
		if switchIP == "" {
			return nil, errors.Errorf("switch %s has no IP but used in conn %s", swName, conn.Name)
		}

		gateway := strings.TrimSuffix(link.Switch.IP, "/31") // TODO remove hardcoded /31
		nextHop := getHextHopID(gateway)

		routes := []networkdRoute{
			{
				Destination: switchIP,
				Gateway:     gateway,
			},
		}
		nextHops := []networkdNextHop{}

		// enable chain link only for non Management ports as mgmt <> front panel doesn't work on real switches
		if !strings.HasPrefix(swPort, "Management") {
			for _, ip := range chainSwitchIPs {
				routes = append(routes, networkdRoute{
					Destination: ip,
					NextHop:     nextHop,
				})
			}
			nextHops = append(nextHops, networkdNextHop{
				Id:      nextHop,
				Gateway: gateway,
			})
		}

		cfg := networkdConfig{
			ControlVIP: r.Cfg.ControlVIP,
			Port:       link.Server.LocalPortName(),
			MAC:        link.Server.MAC,
			IP:         link.Server.IP,
			MUDURL:     mud,
			Routes:     routes,
			NextHops:   nextHops,
		}

		name := strings.ToLower(fmt.Sprintf("00-hh-1--%s--%s--%s", cfg.Port, swName, swPort))
		networkd[name+".network"], err = executeTemplate(controlNetworkTmpl, cfg)
		if err != nil {
			return nil, errors.Wrapf(err, "error executing network template for conn %s", conn.Name)
		}
	}

	return networkd, nil
}

func (r *ControlAgentReconciler) buildHosts(serverName string, switches []wiringapi.Switch) (map[string]string, error) {
	hosts := map[string]string{}

	for _, sw := range switches {
		if sw.Spec.IP == "" {
			return nil, errors.Errorf("switch %s has no IP", sw.Name)
		}

		ip, _, err := net.ParseCIDR(sw.Spec.IP)
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing switch %s IP", sw.Name)
		}

		hosts[sw.Name] = ip.String()
	}

	return hosts, nil
}

func executeTemplate(tmplText string, data any) (string, error) {
	tmplText = strings.TrimPrefix(tmplText, "\n")
	tmplText = strings.TrimSpace(tmplText)

	tmpl, err := template.New("tmpl").Parse(tmplText)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing template")
	}

	buf := bytes.NewBuffer(nil)
	err = tmpl.Execute(buf, data)
	if err != nil {
		return "", errors.Wrapf(err, "error executing template")
	}

	return buf.String(), nil
}

type networkdConfig struct {
	ControlVIP string
	Port       string
	MAC        string
	IP         string
	MUDURL     string
	Routes     []networkdRoute
	NextHops   []networkdNextHop
}

type networkdRoute struct {
	Destination string
	Gateway     string
	NextHop     uint32
}

type networkdNextHop struct {
	Id      uint32
	Gateway string
}

const loopbackNetworkTmpl = `
[Match]
Name=lo
Type=loopback

[Network]
LinkLocalAddressing=ipv6
LLDP=no
EmitLLDP=no
IPv6AcceptRA=no
IPv6SendRA=no
Address=127.0.0.1/8
Address=::1/128
Address={{ .ControlVIP }}
`

const controlNetworkTmpl = `
[Match]
{{ if .MAC }}MACAddress={{ .MAC }}{{ else }}Name={{ .Port }}{{ end }}
Type=ether

[Network]
LinkLocalAddressing=ipv6
LLDP=yes
EmitLLDP=yes
IPv6AcceptRA=yes
IPv6SendRA=yes
Address={{ .IP }}

[LLDP]
MUDURL={{ .MUDURL }}

{{ range .Routes }}
[Route]
{{ if .Destination }}Destination={{ .Destination }}{{ end }}
{{ if .Gateway }}Gateway={{ .Gateway }}{{ end }}
{{ if .NextHop }}NextHop={{ .NextHop }}{{ end }}
{{ end }}

{{ range .NextHops }}
[NextHop]
Id={{ .Id }}
Gateway={{ .Gateway }}
{{ end }}
`

// assuming we have unique /31 links we can just use it as an ID for next hop
func getHextHopID(gateway string) uint32 {
	return binary.BigEndian.Uint32(net.ParseIP(gateway).To4())
}
