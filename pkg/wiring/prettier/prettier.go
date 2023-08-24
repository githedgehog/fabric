package prettier

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/wiring"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type Prettier struct {
	*wiring.Data
}

func objectSummary(obj metav1.Object) map[string]any {
	summary := make(map[string]any)
	summary["name"] = obj.GetName()

	switch o := obj.(type) {
	case *wiringapi.Rack:
		summary["spec"] = o.Spec
	case *wiringapi.Switch:
		summary["spec"] = o.Spec
	case *wiringapi.SwitchPort:
		summary["spec"] = o.Spec
	}

	return summary
}

func withFabricLabel(labels map[string]string, key string, value string) map[string]string {
	extended := map[string]string{}
	for k, v := range labels {
		extended[k] = v
	}
	extended[wiringapi.LabelName(key)] = value

	return extended
}

func (d *Prettier) PrintTree() error {
	tree := make(map[string]any)
	for _, rack := range d.Rack.All() {
		nested := objectSummary(rack)
		switches := []map[string]any{}

		for _, sswitch := range d.Switch.All() {
			switchSum := objectSummary(sswitch)

			switches = append(switches, switchSum)
		}

		nested["switches"] = switches
		tree[rack.Name] = nested
	}

	tree = make(map[string]any)
	racks := []map[string]any{}
	for _, rack := range d.Rack.All() {
		rackSum := objectSummary(rack)

		switches := []map[string]any{}
		for _, sswitch := range d.Switch.Lookup(withFabricLabel(rack.Labels, "rack", rack.Name)) {
			switchSum := objectSummary(sswitch)

			ports := []map[string]any{}
			for _, port := range d.Port.Lookup(withFabricLabel(sswitch.Labels, "switch", sswitch.Name)) {
				portSum := objectSummary(port)

				ports = append(ports, portSum)
			}

			switchSum["ports"] = ports
			switches = append(switches, switchSum)
		}

		rackSum["switches"] = switches

		racks = append(racks, rackSum)
	}

	tree["racks"] = racks

	// it's important to use sigs.k8s.io/yaml b/c it marshalls into json and then converts to yaml
	buf, err := yaml.Marshal(tree)
	if err != nil {
		return errors.Wrap(err, "error marshaling into yaml")
	}

	fmt.Println(string(buf))

	return nil
}

/*
"spine-1" [label="<name> spine-1|{<0> Port 0|192.168.0.1|10.1.1.1|10.2.3.4}|<1> 1"];
"spine-2" [label="<name> spine-2|<0> 0|<1> 1"];

"leaf-1" [label="<name> leaf-1|<0> 0|<1> 1"];

"leaf-1":"0" -> "spine-1" [headlabel="192.168.0.1" label="blah"];
"leaf-1":"1" -> "spine-2";
*/
func (d *Prettier) PrintDot() error {
	fmt.Println("graph {")
	fmt.Println("  node [shape=Mrecord];")
	fmt.Println("  graph [pad=\"0.5\", nodesep=\"1.5\", ranksep=\"2\"];")
	fmt.Println("  splines = \"false\";")
	fmt.Println("  ordering = \"out\";")
	fmt.Println("  rankdir = \"TB\";")
	fmt.Println()

	fmt.Print("  subgraph \"cluster_control-0\" {\n")
	fmt.Printf("    label = <<b>control-0</b>>;\n")
	fmt.Println()

	for _, sw := range d.Switch.All() {
		fmt.Printf("    \"control-0--%s\";\n", sw.Name)
	}

	fmt.Println("  }")
	fmt.Println()

	edges := strings.Builder{}
	for _, role := range []string{"leaf", "spine"} {
		// fmt.Printf("  subgraph \"cluster_%s\" {\n", role)
		// fmt.Printf("    label = <<b>%ss</b>>;\n", role)
		// fmt.Println()

		for _, sw := range d.Switch.All() {
			if sw.Spec.Role != wiringapi.SwitchRole(role) {
				continue
			}

			l := strings.Builder{}
			l.WriteString(fmt.Sprintf("<b>%s</b> (ASN %d)<br/>", sw.Name, sw.Spec.BGPConfig[0].BGPRouterConfig[0].ASN)) // TODO move asn to top level?
			for _, bgp := range sw.Spec.BGPConfig {
				l.WriteString(fmt.Sprintf("Loopback%d %s", bgp.LoopbackInterfaceNum, bgp.LoopbackAddress))

				for _, router := range bgp.BGPRouterConfig {
					l.WriteString(fmt.Sprintf(" vrf %s neighbors: ", router.VRF))
					for _, neighbor := range router.NeighborInfo {
						l.WriteString(fmt.Sprintf("%d/%s ", neighbor.ASN, neighbor.ID))
					}
				}
				l.WriteString("<br/>")
			}
			// fmt.Printf("      \"%s-bgp\" [label=<%s>];\n", sw.Name, l.String())

			fmt.Printf("    subgraph \"cluster_%s\" {\n", sw.Name)
			// fmt.Printf("      label = <<b>%s</b>>;\n", sw.Name)
			fmt.Printf("      label = <%s>;\n", l.String())
			fmt.Println()

			for _, port := range d.Port.Lookup(map[string]string{
				"fabric.githedgehog.com/switch": sw.Name,
			}) {
				l := strings.Builder{}

				l.WriteString(fmt.Sprintf("<b>Port %d</b> (%s %s)<br/>", port.Spec.NOSPortNum, port.Name, port.Spec.NOSPortName))
				for _, inter := range port.Spec.Interfaces {
					l.WriteString(fmt.Sprintf("%s %d %s %s<br/>", inter.Name, inter.VLANs, inter.VRF, inter.IPAddress))
				}

				fmt.Printf("      \"%s\" [label=<%s>];\n", port.Name, l.String())

				// TODO tmp hack to generate only one link per port pair
				if sw.Spec.Role == "spine" || port.Spec.Role == "control" {
					edges.WriteString(fmt.Sprintf("  \"%s\" -- \"%s\";\n", port.Name, port.Spec.Neighbor.Port()))
				}
			}

			// l = strings.Builder{}
			// l.WriteString(fmt.Sprintf("<b>BGP ASN %d</b><br/>", sw.Spec.BGPConfig[0].BGPRouterConfig[0].ASN)) // TODO move asn to top level?
			// for _, bgp := range sw.Spec.BGPConfig {
			// 	l.WriteString(fmt.Sprintf("Loopback%d %s<br/>", bgp.LoopbackInterfaceNum, bgp.LoopbackAddress))

			// 	for _, router := range bgp.BGPRouterConfig {
			// 		// l.WriteString(fmt.Sprintf(" vrf %s neighbors:<br/>", router.VRF))
			// 		for _, neighbor := range router.NeighborInfo {
			// 			l.WriteString(fmt.Sprintf("(%s) %d/%s<br/>", router.VRF, neighbor.Asn, neighbor.ID))
			// 		}
			// 	}
			// }
			// fmt.Printf("      \"%s-bgp\" [label=<%s>];\n", sw.Name, l.String())

			fmt.Println("    }") // cluster_<switch>
		}

		// fmt.Println("  }") // cluster_<role>
		fmt.Println()
	}

	fmt.Println(edges.String())

	fmt.Println("}")

	return nil
}
