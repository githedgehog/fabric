package cookiecutter

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func GenerateSpineLeaf(cfg *SpineLeaf) error {
	if cfg.Controls != 1 {
		return errors.Errorf("unsupported number of control nodes %d (only 1 is supported)", cfg.Controls)
	}

	store := NewStore(cfg)

	for _, t := range []struct {
		num  int
		role string
	}{
		{cfg.Spines, "spine"},
		{cfg.Leafs, "leaf"},
	} {
		for id := 0; id < t.num; id++ {
			store.AddSwitch(t.role, id)
			store.AddPortsForSwitch(t.role, id)
		}
	}

	// TODO switch prints to specified io.Writer

	fmt.Printf(`
# Auto-generated spine leaf topology
# Spines: %d
# Leafs: %d
# Controls: %d
# Computes: %d
# Links (in addition to auto-generated):%s
#
# All switches are in a single Rack named rack-1
# Switch names are "<role>-<id>", <id> is zero-based
# Port names are "<switch-name>--<port-id>", <port-id> is zero-based
#
# Fabric facing interface IPs are 169.254.<leaf-id>.<2 x spine-id + foffset>/31
# > foffset is 0 for leafs and 1 for spines which means leafs have even last octet and spines have odd one  

# Loopback IPs are 10.0.<loffset + leaf/spine-id>.<vrf/loopback-id>/32
# > loffset is 100 for leafs and 200 for spines
#
# Example IPs:
# * leaf-2 <-> spine-1 will have 169.254.2.2/31 on leaf and 169.254.2.3/31 on spine
# * Loopback1 (mgmt) on leaf-2 will have 10.0.102.1/32
# * Loopback0 (default) on spine-1 will have 10.0.201.0/32
#
# ASN for leafs is "65100 + <leaf-id>", e.g. 65101 for leaf-1
# ASN for all spines is 65200


`, cfg.Spines, cfg.Leafs, cfg.Controls, cfg.Computes, FormatLinksComment(cfg.Links))

	// err := store.PrintControl()
	// if err != nil {
	// 	return err
	// }

	err := store.PrintSwitches("spine")
	if err != nil {
		return err
	}

	err = store.PrintSwitches("leaf")
	if err != nil {
		return err
	}

	err = store.PrintRack()
	if err != nil {
		return err
	}

	return nil
}

func DeviceName(t string, id int) string {
	return fmt.Sprintf("%s-%d", t, id)
}

type SpineLeaf struct {
	Spines   int
	Leafs    int
	Controls int
	Computes int
	Links    []Link
}

type Link [2]string

type Store struct {
	Topo     *SpineLeaf
	Switches map[string]*wiringapi.Switch
	Ports    map[string]*wiringapi.SwitchPort
}

func NewStore(topo *SpineLeaf) *Store {
	return &Store{
		Topo:     topo,
		Switches: map[string]*wiringapi.Switch{},
		Ports:    map[string]*wiringapi.SwitchPort{},
	}
}

func (s *Store) Rack() wiringapi.Rack {
	name := "rack-1"
	return wiringapi.Rack{
		TypeMeta: metav1.TypeMeta{
			Kind:       wiringapi.KindRack,
			APIVersion: wiringapi.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{},
		},
	}
}

func (s *Store) AddSwitch(role string, id int) {
	name := DeviceName(role, id)
	s.Switches[name] = &wiringapi.Switch{
		TypeMeta: metav1.TypeMeta{
			Kind:       wiringapi.KindSwitch,
			APIVersion: wiringapi.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				wiringapi.LabelRack: "rack-1",
			},
		},
		Spec: wiringapi.SwitchSpec{
			Role:      wiringapi.SwitchRole(role),
			BGPConfig: s.GetFabricBGPConfig(role, id),
			LLDPConfig: wiringapi.LLDPConfig{
				HelloTimer:        5,
				ManagementIP:      GetLoopbackIP(role, 0, id),
				SystemDescription: fmt.Sprintf("This is %s", name),
				SystemName:        name,
			},
			Vrfs: []string{
				"VrfHHMgmt",
				"VrfHHCtrl",
			},
		},
	}
}

func (s *Store) Switch(t string, id int) *wiringapi.Switch {
	return s.Switches[DeviceName(t, id)]
}

func GetFabricIntIP(switchType string, spine int, leaf int) string {
	shift := 0
	if switchType == "spine" {
		shift = 1
	}
	return fmt.Sprintf("169.254.%d.%d", leaf, 2*spine+shift)
}

func GetLoopbackIP(switchType string, loopbackID int, switchID int) string {
	shift := 100
	if switchType == "spine" {
		shift = 200
	}
	return fmt.Sprintf("10.0.%d.%d", switchID+shift, loopbackID)
}

func GetASN(switchType string, switchID int) int {
	switch switchType {
	case "leaf":
		return 65100 + switchID
	case "spine":
		return 65200
	default:
		log.Fatalf("unrecognized switchType %s", switchType)
	}
	return -1
}

func (s *Store) GetFabricBGPConfig(switchType string, switchID int) []wiringapi.BGPConfig {
	neighbors := []wiringapi.BGPNeighborInfo{}

	// if switchType == "leaf" {
	neighborsNum := s.Topo.Spines
	neighborType := "spine"

	if switchType == "spine" {
		neighborsNum = s.Topo.Leafs
		neighborType = "leaf"
	}

	for id := 0; id < neighborsNum; id++ {
		// if switchType == "leaf" {
		spine := id
		leaf := switchID

		if switchType == "spine" {
			spine = switchID
			leaf = id
		}

		neighbors = append(neighbors, wiringapi.BGPNeighborInfo{
			ID:         GetFabricIntIP(neighborType, spine, leaf),
			ASN:        GetASN(neighborType, id),
			Filterinfo: "undefined",
		})
	}

	return []wiringapi.BGPConfig{
		{
			LoopbackInterfaceNum: 0,
			LoopbackAddress:      GetLoopbackIP(switchType, 0, switchID) + "/32",
			BGPRouterConfig: []wiringapi.BGPRouterConfig{
				{
					ASN:          GetASN(switchType, switchID),
					VRF:          "default",
					RouterID:     GetLoopbackIP(switchType, 0, switchID),
					NeighborInfo: neighbors,
					AddressFamily: wiringapi.AddressFamily{
						Family: "ipv4",
					},
				},
			},
		},
		{
			LoopbackInterfaceNum: 1,
			LoopbackAddress:      GetLoopbackIP(switchType, 1, switchID) + "/32",
			BGPRouterConfig: []wiringapi.BGPRouterConfig{
				{
					ASN:          GetASN(switchType, switchID),
					VRF:          "VrfHHMgmt",
					RouterID:     GetLoopbackIP(switchType, 1, switchID),
					NeighborInfo: neighbors,
					AddressFamily: wiringapi.AddressFamily{
						Family: "ipv4",
					},
				},
			},
		},
		{
			LoopbackInterfaceNum: 2,
			LoopbackAddress:      GetLoopbackIP(switchType, 2, switchID) + "/32",
			BGPRouterConfig: []wiringapi.BGPRouterConfig{
				{
					ASN:          GetASN(switchType, switchID),
					VRF:          "VrfHHCtrl",
					RouterID:     GetLoopbackIP(switchType, 2, switchID),
					NeighborInfo: neighbors,
					AddressFamily: wiringapi.AddressFamily{
						Family: "ipv4",
					},
				},
			},
		},
	}
}

func (s *Store) AddPortsForSwitch(switchRole string, switchID int) {
	// if switchRole == "leaf"
	portsNum := s.Topo.Spines
	portRole := "leaf-fabric"
	toSwitchRole := "spine"

	if switchRole == "spine" {
		portsNum = s.Topo.Leafs
		portRole = "spine-fabric"
		toSwitchRole = "leaf"
	}

	switchName := DeviceName(switchRole, switchID)

	// add port for control node
	{
		name := fmt.Sprintf("%s--%d", switchName, 0)
		nosPortName := fmt.Sprintf("Ethernet%d", 0) // TODO make configurable, *4 is VS-only
		controlName := "control-0"

		s.Ports[name] = &wiringapi.SwitchPort{
			TypeMeta: metav1.TypeMeta{
				Kind:       wiringapi.KindSwitchPort,
				APIVersion: wiringapi.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					wiringapi.LabelRack:   "rack-1",
					wiringapi.LabelSwitch: switchName,
				},
			},
			Spec: wiringapi.SwitchPortSpec{
				Role:        wiringapi.SwitchPortRole("leaf-server-l2-tagged"), // TODO replace with proper one
				NOSPortNum:  0,
				NOSPortName: nosPortName,
				Neighbor: wiringapi.Neighbor{
					Server: &wiringapi.NeighborInfo{
						Name: controlName,
						Port: fmt.Sprintf("%s--%s", controlName, switchName),
					},
				},
				// Interfaces: []fabricv1alpha1.Interface{
				// 	{
				// 		Name:       fmt.Sprintf("%s.%d", nosPortName, 1),
				// 		VLAN:       200,
				// 		IPAddress:  GetFabricIntIP(switchRole, spine, leaf) + "/31",
				// 		BGPEnabled: true,
				// 		BFDEnabled: true,
				// 		VRF:        "VrfHHMgmt",
				// 	},
				// 	{
				// 		Name:       fmt.Sprintf("%s.%d", nosPortName, 2),
				// 		VLAN:       300,
				// 		IPAddress:  GetFabricIntIP(switchRole, spine, leaf) + "/31",
				// 		BGPEnabled: true,
				// 		BFDEnabled: true,
				// 		VRF:        "VrfHHCtrl",
				// 	},
				// },
			},
		}
	}

	for toSwitchID := 0; toSwitchID < portsNum; toSwitchID++ {
		name := fmt.Sprintf("%s--%d", switchName, toSwitchID+1)
		nosPortName := fmt.Sprintf("Ethernet%d", (toSwitchID+1)*4) // TODO make configurable, *4 is VS-only

		// if switchRole == "leaf" {
		spine := toSwitchID
		leaf := switchID

		if switchRole == "spine" {
			spine = switchID
			leaf = toSwitchID
		}

		toSwitchName := DeviceName(toSwitchRole, toSwitchID)

		s.Ports[name] = &wiringapi.SwitchPort{
			TypeMeta: metav1.TypeMeta{
				Kind:       wiringapi.KindSwitchPort,
				APIVersion: wiringapi.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					wiringapi.LabelRack:   "rack-1",
					wiringapi.LabelSwitch: switchName,
				},
			},
			Spec: wiringapi.SwitchPortSpec{
				Role:        wiringapi.SwitchPortRole(portRole),
				NOSPortNum:  uint16(toSwitchID + 1),
				NOSPortName: nosPortName,
				Neighbor: wiringapi.Neighbor{
					Switch: &wiringapi.NeighborInfo{
						Name: toSwitchName,
						Port: fmt.Sprintf("%s--%d", toSwitchName, switchID+1),
					},
				},
				Interfaces: []wiringapi.Interface{
					{
						Name:       fmt.Sprintf("%s.%d", nosPortName, 0),
						VLANs:      []uint16{100},
						IPAddress:  GetFabricIntIP(switchRole, spine, leaf) + "/31",
						BGPEnabled: true,
						BFDEnabled: true,
						VRF:        "default",
					},
					{
						Name:       fmt.Sprintf("%s.%d", nosPortName, 1),
						VLANs:      []uint16{200},
						IPAddress:  GetFabricIntIP(switchRole, spine, leaf) + "/31",
						BGPEnabled: true,
						BFDEnabled: true,
						VRF:        "VrfHHMgmt",
					},
					{
						Name:       fmt.Sprintf("%s.%d", nosPortName, 2),
						VLANs:      []uint16{300},
						IPAddress:  GetFabricIntIP(switchRole, spine, leaf) + "/31",
						BGPEnabled: true,
						BFDEnabled: true,
						VRF:        "VrfHHCtrl",
					},
				},
			},
		}
	}

	// todo if leaf && computes present - continue creating ports for computes connected to current switch
}

func (s *Store) PrintObj(obj any) error {
	buf, err := yaml.Marshal(obj)
	if err != nil {
		return errors.Wrap(err, "error marshaling into yaml")
	}
	_, err = fmt.Println(string(buf))

	return err
}

func (s *Store) PrintSeparator() error {
	_, err := fmt.Println("---")

	return err
}

func (s *Store) SwitchesByRole(role string) []*wiringapi.Switch {
	result := []*wiringapi.Switch{}

	for name, sw := range s.Switches {
		if strings.HasPrefix(name, role) {
			result = append(result, sw)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

func (s *Store) PortsBySwitch(sw string) []*wiringapi.SwitchPort {
	result := []*wiringapi.SwitchPort{}

	for name, port := range s.Ports {
		if strings.HasPrefix(name, sw) {
			result = append(result, port)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

func (s *Store) PrintCommentHeader(name string, highlight bool) error {
	if highlight {
		_, err := fmt.Println("###########################################")
		if err != nil {
			return err
		}
	}

	_, err := fmt.Println("# " + name)
	if err != nil {
		return err
	}

	if highlight {
		_, err := fmt.Println("###########################################")
		if err != nil {
			return nil
		}
	}

	return nil
}

func (s *Store) PrintSwitches(role string) error {
	for _, sw := range s.SwitchesByRole(role) {
		err := s.PrintCommentHeader(sw.Name, true)
		if err != nil {
			return err
		}

		err = s.PrintObj(sw)
		if err != nil {
			return err
		}

		err = s.PrintSeparator()
		if err != nil {
			return err
		}

		for _, port := range s.PortsBySwitch(sw.Name) {
			err := s.PrintCommentHeader(sw.Name, false)
			if err != nil {
				return err
			}

			err = s.PrintObj(port)
			if err != nil {
				return err
			}

			err = s.PrintSeparator()
			if err != nil {
				return err
			}
		}

		err = s.PrintCommentHeader("end of "+sw.Name, false)
		if err != nil {
			return err
		}
	}

	return nil
}

// func (s *Store) PrintControl() error {
// 	control :=
// }

func (s *Store) PrintRack() error {
	rack := s.Rack()

	err := s.PrintCommentHeader(rack.Name, true)
	if err != nil {
		return err
	}

	err = s.PrintObj(rack)
	if err != nil {
		return err
	}

	err = s.PrintCommentHeader("end of "+rack.Name, true)
	if err != nil {
		return err
	}

	return nil
}

func FormatLinksComment(links []Link) string {
	b := strings.Builder{}

	for _, link := range links {
		b.WriteString(fmt.Sprintf("\n#  %s:%s", link[0], link[1]))
	}

	return b.String()
}
