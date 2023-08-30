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

	_, err = createSwitch(data, "switch-1", "rack-1", wiringapi.SwitchSpec{})
	if err != nil {
		return nil, err
	}
	_, err = createSwitch(data, "switch-2", "rack-1", wiringapi.SwitchSpec{})
	if err != nil {
		return nil, err
	}

	_, err = createServer(data, "control-1", "rack-1", wiringapi.ServerSpec{
		Type: wiringapi.ServerTypeControl,
	})
	if err != nil {
		return nil, err
	}

	_, err = createServer(data, "compute-1", "rack-1", wiringapi.ServerSpec{})
	if err != nil {
		return nil, err
	}
	_, err = createServer(data, "compute-2", "rack-1", wiringapi.ServerSpec{})
	if err != nil {
		return nil, err
	}
	_, err = createServer(data, "compute-3", "rack-1", wiringapi.ServerSpec{})
	if err != nil {
		return nil, err
	}
	_, err = createServer(data, "compute-4", "rack-1", wiringapi.ServerSpec{})
	if err != nil {
		return nil, err
	}

	// control-1 <> switch-1
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		Management: &wiringapi.ManagementConn{
			Link: wiringapi.ManagementConnLink{
				{ServerPort: &wiringapi.ConnLinkPort{Name: "control-1/nic0/port1"}},
				{SwitchPort: &wiringapi.ManagementConnSwitchPort{
					ConnLinkPort: wiringapi.ConnLinkPort{
						Name: "switch-1/Management0",
					},
					IP: "192.168.88.121", // TODO
				}},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// control-1 <> switch-2
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		Management: &wiringapi.ManagementConn{
			Link: wiringapi.ManagementConnLink{
				{ServerPort: &wiringapi.ConnLinkPort{Name: "control-1/nic0/port2"}},
				{SwitchPort: &wiringapi.ManagementConnSwitchPort{
					ConnLinkPort: wiringapi.ConnLinkPort{
						Name: "switch-2/Management0",
					},
					IP: "192.168.88.122", // TODO
				}},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// MCLAG Domain peer link
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		MCLAGDomain: &wiringapi.MCLAGDomainConn{
			Links: []wiringapi.ConnLink{
				{
					{SwitchPort: &wiringapi.ConnLinkPort{Name: "switch-1/Ethernet0"}},
					{SwitchPort: &wiringapi.ConnLinkPort{Name: "switch-2/Ethernet0"}},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// compute-1 <MCLAG> (switch-1, switch-2)
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		MCLAG: &wiringapi.MCLAGConn{
			Links: []wiringapi.ConnLink{
				{
					{ServerPort: &wiringapi.ConnLinkPort{Name: "compute-1/nic0/port0"}},
					{SwitchPort: &wiringapi.ConnLinkPort{Name: "switch-1/Ethernet1"}},
				},
				{
					{ServerPort: &wiringapi.ConnLinkPort{Name: "compute-1/nic0/port1"}},
					{SwitchPort: &wiringapi.ConnLinkPort{Name: "switch-2/Ethernet1"}},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// compute-2 <MCLAG> (switch-1, switch-2)
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		MCLAG: &wiringapi.MCLAGConn{
			Links: []wiringapi.ConnLink{
				{
					{ServerPort: &wiringapi.ConnLinkPort{Name: "compute-1/nic0/port0"}},
					{SwitchPort: &wiringapi.ConnLinkPort{Name: "switch-1/Ethernet2"}},
				},
				{
					{ServerPort: &wiringapi.ConnLinkPort{Name: "compute-1/nic0/port1"}},
					{SwitchPort: &wiringapi.ConnLinkPort{Name: "switch-2/Ethernet2"}},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// compute-3 <> switch-1
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		Unbundled: &wiringapi.UnbundledConn{
			Link: wiringapi.ConnLink{
				{ServerPort: &wiringapi.ConnLinkPort{Name: "compute-3/nic0/port0"}},
				{SwitchPort: &wiringapi.ConnLinkPort{Name: "switch-1/Ethernet3"}},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// compute-4 <> switch-2
	_, err = createConnection(data, wiringapi.ConnectionSpec{
		Unbundled: &wiringapi.UnbundledConn{
			Link: wiringapi.ConnLink{
				{ServerPort: &wiringapi.ConnLinkPort{Name: "compute-4/nic0/port0"}},
				{SwitchPort: &wiringapi.ConnLinkPort{Name: "switch-2/Ethernet3"}},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return data, nil
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

	return sw, errors.Wrapf(data.Add(sw), "error creating switch %s", name)
}

func createServer(data *wiring.Data, name string, rack string, spec wiringapi.ServerSpec) (*wiringapi.Server, error) {
	log.Println("Creating server", name)

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

	return conn, errors.Wrapf(data.Add(conn), "error creating connection %s", name)
}