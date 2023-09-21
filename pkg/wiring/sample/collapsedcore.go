package sample

import (
	"fmt"
	"log"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/wiring"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Preset string

const (
	SAMPLE_CC_VLAB Preset = "vlab"
	SAMPLE_CC_LAB  Preset = "lab"
)

var PresetsAll = []Preset{
	SAMPLE_CC_VLAB,
	SAMPLE_CC_LAB,
}

func CollapsedCore(preset Preset) (*wiring.Data, error) {
	if preset == "" {
		preset = SAMPLE_CC_VLAB
	}

	ctrlSwitchPort := func(portID int) string {
		if preset == SAMPLE_CC_VLAB {
			return fmt.Sprintf("eth%d", portID)
		}
		if preset == SAMPLE_CC_LAB {
			return fmt.Sprintf("eno%d", portID+1)
		}

		return "<invalid>"
	}

	oniePort := "eth0" // we're using mgmt port for now

	data, err := wiring.New()
	if err != nil {
		return nil, err
	}

	_, err = createRack(data, "rack-1", wiringapi.RackSpec{})
	if err != nil {
		return nil, err
	}

	_, err = createSwitch(data, "switch-1", "rack-1", wiringapi.SwitchSpec{
		Location: location("1"),
	})
	if err != nil {
		return nil, err
	}
	_, err = createSwitch(data, "switch-2", "rack-1", wiringapi.SwitchSpec{
		Location: location("2"),
	})
	if err != nil {
		return nil, err
	}

	_, err = createServer(data, "control-1", "rack-1", wiringapi.ServerSpec{
		Type:     wiringapi.ServerTypeControl,
		Location: location("3"),
	})
	if err != nil {
		return nil, err
	}

	_, err = createServer(data, "server-1", "rack-1", wiringapi.ServerSpec{
		Location: location("4"),
	})
	if err != nil {
		return nil, err
	}
	_, err = createServer(data, "server-2", "rack-1", wiringapi.ServerSpec{
		Location: location("5"),
	})
	if err != nil {
		return nil, err
	}

	// _, err = createServer(data, "server-3", "rack-1", wiringapi.ServerSpec{
	// 	Location: location("6"),
	// })
	// if err != nil {
	// 	return nil, err
	// }
	// _, err = createServer(data, "server-4", "rack-1", wiringapi.ServerSpec{
	// 	Location: location("7"),
	// })
	// if err != nil {
	// 	return nil, err
	// }

	// control-1 <> switch-1
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		Management: &wiringapi.ConnMgmt{
			Link: wiringapi.ConnMgmtLink{
				Server: wiringapi.ConnMgmtLinkServer{
					BasePortName: wiringapi.NewBasePortName("control-1/" + ctrlSwitchPort(1)),
					IP:           "192.168.101.1/31",
				},
				Switch: wiringapi.ConnMgmtLinkSwitch{
					BasePortName: wiringapi.NewBasePortName("switch-1/Management0"),
					IP:           "192.168.101.0/31",
					// VLAN:         uint16(mgmtVLAN),
					ONIEPortName: oniePort,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// control-1 <> switch-2
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		Management: &wiringapi.ConnMgmt{
			Link: wiringapi.ConnMgmtLink{
				Server: wiringapi.ConnMgmtLinkServer{
					BasePortName: wiringapi.NewBasePortName("control-1/" + ctrlSwitchPort(2)),
					IP:           "192.168.102.1/31",
				},
				Switch: wiringapi.ConnMgmtLinkSwitch{
					BasePortName: wiringapi.NewBasePortName("switch-2/Management0"),
					IP:           "192.168.102.0/31",
					// VLAN:         uint16(mgmtVLAN),
					ONIEPortName: oniePort,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	mclagPeerPort1 := "Ethernet0"
	mclagPeerPort2 := "Ethernet1"
	mclagSessionPort1 := "Ethernet2"
	mclagSessionPort2 := "Ethernet3"
	if preset == SAMPLE_CC_LAB {
		mclagPeerPort1 = "Ethernet48"
		mclagPeerPort2 = "Ethernet56"
		mclagSessionPort1 = "Ethernet64"
		mclagSessionPort2 = "Ethernet68"
	}

	// MCLAG Domain peer link
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		MCLAGDomain: &wiringapi.ConnMCLAGDomain{
			PeerLinks: []wiringapi.SwitchToSwitchLink{
				{
					Switch1: wiringapi.NewBasePortName("switch-1/" + mclagPeerPort1),
					Switch2: wiringapi.NewBasePortName("switch-2/" + mclagPeerPort1),
				},
				{
					Switch1: wiringapi.NewBasePortName("switch-1/" + mclagPeerPort2),
					Switch2: wiringapi.NewBasePortName("switch-2/" + mclagPeerPort2),
				},
			},
			SessionLinks: []wiringapi.SwitchToSwitchLink{
				{
					Switch1: wiringapi.NewBasePortName("switch-1/" + mclagSessionPort1),
					Switch2: wiringapi.NewBasePortName("switch-2/" + mclagSessionPort1),
				},
				{
					Switch1: wiringapi.NewBasePortName("switch-1/" + mclagSessionPort2),
					Switch2: wiringapi.NewBasePortName("switch-2/" + mclagSessionPort2),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	server1Port1 := "eth1"
	server1Port2 := "eth2"
	server1SwitchPort := "Ethernet4"
	if preset == SAMPLE_CC_LAB {
		server1Port1 = "enp7s0"
		server1Port2 = "enp8s0"
		server1SwitchPort = "Ethernet46" // TODO confirm which one is which
	}

	// server-1 <MCLAG> (switch-1, switch-2)
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		MCLAG: &wiringapi.ConnMCLAG{
			Links: []wiringapi.ServerToSwitchLink{
				{
					Server: wiringapi.NewBasePortName("server-1/" + server1Port1),
					Switch: wiringapi.NewBasePortName("switch-1/" + server1SwitchPort),
				},
				{
					Server: wiringapi.NewBasePortName("server-1/" + server1Port2),
					Switch: wiringapi.NewBasePortName("switch-2/" + server1SwitchPort),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	server2Port1 := "eth1"
	server2Port2 := "eth2"
	server2SwitchPort := "Ethernet5"
	if preset == SAMPLE_CC_LAB {
		server2Port1 = "enp7s0"
		server2Port2 = "enp8s0"
		server2SwitchPort = "Ethernet47" // TODO confirm which one is which
	}

	// server-2 <MCLAG> (switch-1, switch-2)
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		MCLAG: &wiringapi.ConnMCLAG{
			Links: []wiringapi.ServerToSwitchLink{
				{
					Server: wiringapi.NewBasePortName("server-2/" + server2Port1),
					Switch: wiringapi.NewBasePortName("switch-1/" + server2SwitchPort),
				},
				{
					Server: wiringapi.NewBasePortName("server-2/" + server2Port2),
					Switch: wiringapi.NewBasePortName("switch-2/" + server2SwitchPort),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// // server-3 <> switch-1
	// _, err = createConnection(data, wiringapi.ConnectionSpec{
	// 	Unbundled: &wiringapi.ConnUnbundled{
	// 		Link: wiringapi.ServerToSwitchLink{
	// 			Server: wiringapi.NewBasePortName("server-3/nic0/port0"),
	// 			Switch: wiringapi.NewBasePortName("switch-1/Ethernet5"),
	// 		},
	// 	},
	// })
	// if err != nil {
	// 	return nil, err
	// }

	// // server-4 <> switch-2
	// _, err = createConnection(data, wiringapi.ConnectionSpec{
	// 	Unbundled: &wiringapi.ConnUnbundled{
	// 		Link: wiringapi.ServerToSwitchLink{
	// 			Server: wiringapi.NewBasePortName("server-4/nic0/port0"),
	// 			Switch: wiringapi.NewBasePortName("switch-2/Ethernet5"),
	// 		},
	// 	},
	// })
	// if err != nil {
	// 	return nil, err
	// }

	return data, nil
}

func location(slot string) wiringapi.Location {
	return wiringapi.Location{
		Location: "LOC",
		Aisle:    "1",
		Row:      "1",
		Rack:     "1",
		Slot:     slot,
	}
}

func createRack(data *wiring.Data, name string, spec wiringapi.RackSpec) (*wiringapi.Rack, error) {
	log.Println("Creating rack", name)

	sw := &wiringapi.Rack{
		TypeMeta: meta.TypeMeta{
			Kind:       wiringapi.KindRack,
			APIVersion: wiringapi.GroupVersion.String(),
		},
		ObjectMeta: meta.ObjectMeta{
			Name:   name,
			Labels: map[string]string{},
		},
		Spec: spec,
	}

	return sw, errors.Wrapf(data.Add(sw), "error creating switch %s", name)
}

func createSwitch(data *wiring.Data, name string, rack string, spec wiringapi.SwitchSpec) (*wiringapi.Switch, error) {
	log.Println("Creating switch", name)

	spec.LocationSig = wiringapi.LocationSig{
		Sig:     "long-signature",
		UUIDSig: "also-long-signature",
	}
	sw := &wiringapi.Switch{
		TypeMeta: meta.TypeMeta{
			Kind:       wiringapi.KindSwitch,
			APIVersion: wiringapi.GroupVersion.String(),
		},
		ObjectMeta: meta.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				wiringapi.LabelRack: rack,
			},
		},
		Spec: spec,
	}
	locUUID, _ := sw.Spec.Location.GenerateUUID()
	sw.Labels[wiringapi.LabelLocation] = locUUID

	return sw, errors.Wrapf(data.Add(sw), "error creating switch %s", name)
}

func createServer(data *wiring.Data, name string, rack string, spec wiringapi.ServerSpec) (*wiringapi.Server, error) {
	log.Println("Creating server", name)

	spec.LocationSig = wiringapi.LocationSig{
		Sig:     "long-signature",
		UUIDSig: "also-long-signature",
	}
	server := &wiringapi.Server{
		TypeMeta: meta.TypeMeta{
			Kind:       wiringapi.KindServer,
			APIVersion: wiringapi.GroupVersion.String(),
		},
		ObjectMeta: meta.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				wiringapi.LabelRack: rack,
			},
		},
		Spec: spec,
	}
	locUUID, _ := server.Spec.Location.GenerateUUID()
	server.Labels[wiringapi.LabelLocation] = locUUID

	return server, errors.Wrapf(data.Add(server), "error creating server %s", name)
}

func createConnection(data *wiring.Data, spec wiringapi.ConnectionSpec) (*wiringapi.Connection, error) {
	name := spec.GenerateName()

	log.Println("Creating connection", name)

	conn := &wiringapi.Connection{
		TypeMeta: meta.TypeMeta{
			Kind:       wiringapi.KindConnection,
			APIVersion: wiringapi.GroupVersion.String(),
		},
		ObjectMeta: meta.ObjectMeta{
			Name:   name,
			Labels: map[string]string{},
		},
		Spec: spec,
	}
	conn.Labels = spec.ConnectionLabels()
	// TODO replace it with an actuall racks, not hardcoded one
	conn.Labels[wiringapi.ListLabelRack("rack-1")] = wiringapi.ListLabelValue

	return conn, errors.Wrapf(data.Add(conn), "error creating connection %s", name)
}
