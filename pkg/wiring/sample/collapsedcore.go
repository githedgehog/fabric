package sample

import (
	"log"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/wiring"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CollapsedCore() (*wiring.Data, error) {
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

	_, err = createServer(data, "compute-1", "rack-1", wiringapi.ServerSpec{
		Location: location("4"),
	})
	if err != nil {
		return nil, err
	}
	_, err = createServer(data, "compute-2", "rack-1", wiringapi.ServerSpec{
		Location: location("5"),
	})
	if err != nil {
		return nil, err
	}
	_, err = createServer(data, "compute-3", "rack-1", wiringapi.ServerSpec{
		Location: location("6"),
	})
	if err != nil {
		return nil, err
	}
	_, err = createServer(data, "compute-4", "rack-1", wiringapi.ServerSpec{
		Location: location("7"),
	})
	if err != nil {
		return nil, err
	}

	// control-1 <> switch-1
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		Management: &wiringapi.ConnMgmt{
			Link: wiringapi.ConnMgmtLink{
				Server: wiringapi.NewBasePortName("control-1/switch1"),
				Switch: wiringapi.ConnMgmtLinkSwitch{
					BasePortName: wiringapi.NewBasePortName("switch-1/Management0"),
					IP:           "192.168.42.11/24", // TODO do we need it configurable?
					VLAN:         42,                 // we aren't using VLANs in collapsed core
					ONIEPortName: "eth1",
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
				Server: wiringapi.NewBasePortName("control-1/switch2"),
				Switch: wiringapi.ConnMgmtLinkSwitch{
					BasePortName: wiringapi.NewBasePortName("switch-2/Management0"),
					IP:           "192.168.42.12/24", // TODO do we need it configurable?
					VLAN:         42,                 // we aren't using VLANs in collapsed core
					ONIEPortName: "eth1",
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// MCLAG Domain peer link
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		MCLAGDomain: &wiringapi.ConnMCLAGDomain{
			Links: []wiringapi.SwitchToSwitchLink{
				{
					Switch1: wiringapi.NewBasePortName("switch-1/Ethernet0"),
					Switch2: wiringapi.NewBasePortName("switch-2/Ethernet0"),
				},
				{
					Switch1: wiringapi.NewBasePortName("switch-1/Ethernet1"),
					Switch2: wiringapi.NewBasePortName("switch-2/Ethernet1"),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// compute-1 <MCLAG> (switch-1, switch-2)
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		MCLAG: &wiringapi.ConnMCLAG{
			Links: []wiringapi.ServerToSwitchLink{
				{
					Server: wiringapi.NewBasePortName("compute-1/nic0/port0"),
					Switch: wiringapi.NewBasePortName("switch-1/Ethernet2"),
				},
				{
					Server: wiringapi.NewBasePortName("compute-1/nic0/port1"),
					Switch: wiringapi.NewBasePortName("switch-2/Ethernet2"),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// compute-2 <MCLAG> (switch-1, switch-2)
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		MCLAG: &wiringapi.ConnMCLAG{
			Links: []wiringapi.ServerToSwitchLink{
				{
					Server: wiringapi.NewBasePortName("compute-2/nic0/port0"),
					Switch: wiringapi.NewBasePortName("switch-1/Ethernet3"),
				},
				{
					Server: wiringapi.NewBasePortName("compute-2/nic0/port1"),
					Switch: wiringapi.NewBasePortName("switch-2/Ethernet3"),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// compute-3 <> switch-1
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		Unbundled: &wiringapi.ConnUnbundled{
			Link: wiringapi.ServerToSwitchLink{
				Server: wiringapi.NewBasePortName("compute-3/nic0/port0"),
				Switch: wiringapi.NewBasePortName("switch-1/Ethernet4"),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// compute-4 <> switch-2
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		Unbundled: &wiringapi.ConnUnbundled{
			Link: wiringapi.ServerToSwitchLink{
				Server: wiringapi.NewBasePortName("compute-4/nic0/port0"),
				Switch: wiringapi.NewBasePortName("switch-2/Ethernet4"),
			},
		},
	})
	if err != nil {
		return nil, err
	}

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
	conn.Labels[wiringapi.ConnectionLabel(wiringapi.ConnectionLabelTypeRack, "rack-1")] = wiringapi.ConnectionLabelValue

	return conn, errors.Wrapf(data.Add(conn), "error creating connection %s", name)
}
