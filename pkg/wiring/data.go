package wiring

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sort"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type Data struct {
	Rack          *Store[*wiringapi.Rack]
	Switch        *Store[*wiringapi.Switch]
	Server        *Store[*wiringapi.Server]
	Connection    *Store[*wiringapi.Connection]
	SwitchProfile *Store[*wiringapi.SwitchProfile]
	ServerProfile *Store[*wiringapi.ServerProfile]
}

func New(objs ...metav1.Object) (*Data, error) {
	data := &Data{
		Rack:          NewStore[*wiringapi.Rack](),
		Switch:        NewStore[*wiringapi.Switch](),
		Server:        NewStore[*wiringapi.Server](),
		Connection:    NewStore[*wiringapi.Connection](),
		SwitchProfile: NewStore[*wiringapi.SwitchProfile](),
		ServerProfile: NewStore[*wiringapi.ServerProfile](),
	}

	return data, data.Add(objs...)
}

func (d *Data) Add(objs ...metav1.Object) error {
	for _, obj := range objs {
		switch typed := obj.(type) {
		case *wiringapi.Rack:
			d.Rack.Add(typed)
		case *wiringapi.Switch:
			d.Switch.Add(typed)
		case *wiringapi.Server:
			d.Server.Add(typed)
		case *wiringapi.Connection:
			d.Connection.Add(typed)
		case *wiringapi.SwitchProfile:
			d.SwitchProfile.Add(typed)
		case *wiringapi.ServerProfile:
			d.ServerProfile.Add(typed)
		default:
			return errors.Errorf("unrecognized obj type")
		}
	}

	return nil
}

type Store[T metav1.Object] struct {
	m map[string]T
}

func NewStore[T metav1.Object]() *Store[T] {
	return &Store[T]{
		make(map[string]T),
	}
}

func (s *Store[T]) Add(item T) {
	s.m[item.GetName()] = item
}

func (s *Store[T]) Get(name string) T {
	return s.m[name]
}

func (s *Store[T]) LookupLabel(name, value string) []T {
	return s.Lookup(map[string]string{
		name: value,
	})
}

func (s *Store[T]) Lookup(labels map[string]string) []T {
	objs := []T{}
	for _, obj := range maps.Values(s.m) {
		accepted := true

		for lookupName, lookupValue := range labels {
			if value, ok := obj.GetLabels()[lookupName]; !ok || lookupValue != value {
				accepted = false
				break
			}
		}

		if accepted {
			objs = append(objs, obj)
		}
	}

	SortByName(objs)

	return objs
}

func (s *Store[T]) All() []T {
	objs := maps.Values(s.m)

	SortByName(objs)

	return objs
}

func (s *Store[T]) Size() int {
	return len(s.m)
}

func SortByName[T metav1.Object](objs []T) {
	sort.Slice(objs, func(i, j int) bool {
		return objs[i].GetName() < objs[j].GetName()
	})
}

func (d *Data) Write(ret io.Writer) error {
	w := new(bytes.Buffer)

	for idx, rack := range d.Rack.All() {
		err := marshal(rack, idx > 0, w)
		if err != nil {
			return err
		}
	}

	for _, sw := range d.Switch.All() {
		err := marshal(sw, true, w)
		if err != nil {
			return err
		}
	}

	for _, server := range d.Server.All() {
		if !server.IsControl() {
			continue
		}
		err := marshal(server, true, w)
		if err != nil {
			return err
		}
	}

	for _, server := range d.Server.All() {
		if server.IsControl() {
			continue
		}
		err := marshal(server, true, w)
		if err != nil {
			return err
		}
	}

	for _, conn := range d.Connection.All() {
		err := marshal(conn, true, w)
		if err != nil {
			return err
		}
	}

	// ugly output cleanup
	scan := bufio.NewScanner(w)
	for scan.Scan() {
		line := scan.Text()

		if slices.Contains([]string{"status: {}", "  creationTimestamp: null", "  position: {}"}, line) {
			continue
		}

		_, err := ret.Write([]byte(line + "\n"))
		if err != nil {
			return errors.Wrap(err, "error writing line")
		}
	}

	return nil
}

func marshal(obj metav1.Object, separator bool, w io.Writer) error {
	if separator {
		_, err := w.Write([]byte("---\n"))
		if err != nil {
			return errors.Wrap(err, "error writing separator")
		}
	}

	_, err := w.Write([]byte(fmt.Sprintf("###\n### %s\n###\n", obj.GetName())))
	if err != nil {
		return errors.Wrap(err, "error writing title")
	}

	buf, err := yaml.Marshal(obj)
	if err != nil {
		return errors.Wrap(err, "error marshaling into yaml")
	}
	_, err = w.Write(buf)

	return errors.Wrap(err, "error writing yaml")
}
