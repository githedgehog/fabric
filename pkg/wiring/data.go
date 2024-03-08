// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wiring

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type Data struct {
	Rack               *Store[*wiringapi.Rack]
	SwitchGroup        *Store[*wiringapi.SwitchGroup]
	Switch             *Store[*wiringapi.Switch]
	Server             *Store[*wiringapi.Server]
	Connection         *Store[*wiringapi.Connection]
	SwitchProfile      *Store[*wiringapi.SwitchProfile]
	ServerProfile      *Store[*wiringapi.ServerProfile]
	IPv4Namespaces     *Store[*vpcapi.IPv4Namespace]
	VLANNamespace      *Store[*wiringapi.VLANNamespace]
	External           *Store[*vpcapi.External]
	ExternalAttachment *Store[*vpcapi.ExternalAttachment]

	Native *NativeData
}

func New(objs ...metav1.Object) (*Data, error) {
	native, err := NewNativeData()
	if err != nil {
		return nil, errors.Wrap(err, "error creating native data")
	}

	data := &Data{
		Rack:               NewStore[*wiringapi.Rack](),
		SwitchGroup:        NewStore[*wiringapi.SwitchGroup](),
		Switch:             NewStore[*wiringapi.Switch](),
		Server:             NewStore[*wiringapi.Server](),
		Connection:         NewStore[*wiringapi.Connection](),
		SwitchProfile:      NewStore[*wiringapi.SwitchProfile](),
		ServerProfile:      NewStore[*wiringapi.ServerProfile](),
		IPv4Namespaces:     NewStore[*vpcapi.IPv4Namespace](),
		VLANNamespace:      NewStore[*wiringapi.VLANNamespace](),
		External:           NewStore[*vpcapi.External](),
		ExternalAttachment: NewStore[*vpcapi.ExternalAttachment](),

		Native: native,
	}

	return data, data.Add(objs...)
}

func (d *Data) Add(objs ...metav1.Object) error {
	return d.addOrUpdate(false, objs...)
}

func (d *Data) Update(objs ...metav1.Object) error {
	return d.addOrUpdate(true, objs...)
}

func (d *Data) addOrUpdate(update bool, objs ...metav1.Object) error {
	for _, obj := range objs {
		group := obj.(runtime.Object).GetObjectKind().GroupVersionKind().Group
		if group != wiringapi.GroupVersion.Group && group != vpcapi.GroupVersion.Group {
			return errors.Errorf("object has unknown or unsupported group %s", group)
		}

		if fabricObj, ok := obj.(meta.Object); !ok {
			return errors.Errorf("object %#v is not a Fabric Object", obj)
		} else {
			fabricObj.Default()
		}

		var err error
		switch typed := obj.(type) {
		case *wiringapi.Rack:
			err = d.Rack.Add(update, typed)
		case *wiringapi.SwitchGroup:
			err = d.SwitchGroup.Add(update, typed)
		case *wiringapi.Switch:
			err = d.Switch.Add(update, typed)
		case *wiringapi.Server:
			err = d.Server.Add(update, typed)
		case *wiringapi.Connection:
			err = d.Connection.Add(update, typed)
		case *wiringapi.SwitchProfile:
			err = d.SwitchProfile.Add(update, typed)
		case *wiringapi.ServerProfile:
			err = d.ServerProfile.Add(update, typed)
		case *vpcapi.IPv4Namespace:
			err = d.IPv4Namespaces.Add(update, typed)
		case *wiringapi.VLANNamespace:
			err = d.VLANNamespace.Add(update, typed)
		case *vpcapi.External:
			err = d.External.Add(update, typed)
		case *vpcapi.ExternalAttachment:
			err = d.ExternalAttachment.Add(update, typed)
		default:
			return errors.Errorf("unrecognized obj type")
		}

		if err != nil {
			return errors.Wrap(err, "error adding object")
		}

		if !update {
			obj.SetResourceVersion("")
			if err := d.Native.Create(context.TODO(), obj.(client.Object)); err != nil {
				return errors.Wrap(err, "error creating object")
			}
		} else {
			clientObj := obj.(client.Object)
			key := client.ObjectKeyFromObject(clientObj)

			if err := d.Native.Update(context.TODO(), clientObj); err != nil {
				return errors.Wrapf(err, "error updating object: %s", key.String())
			}
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

func (s *Store[T]) Add(update bool, item T) error {
	if _, exists := s.m[item.GetName()]; !update && exists {
		return errors.Errorf("item already exists %s", item.GetName())
	}

	s.m[item.GetName()] = item

	return nil
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

	idx := 0

	for _, vlan := range d.VLANNamespace.All() {
		err := marshal(vlan, idx > 0, w)
		if err != nil {
			return err
		}
		idx++
	}

	for _, ns := range d.IPv4Namespaces.All() {
		err := marshal(ns, idx > 0, w)
		if err != nil {
			return err
		}
		idx++
	}

	for _, rack := range d.Rack.All() {
		err := marshal(rack, idx > 0, w)
		if err != nil {
			return err
		}
		idx++
	}

	for _, sg := range d.SwitchGroup.All() {
		err := marshal(sg, idx > 0, w)
		if err != nil {
			return err
		}
		idx++
	}

	for _, sw := range d.Switch.All() {
		err := marshal(sw, idx > 0, w)
		if err != nil {
			return err
		}
		idx++
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

	for _, ext := range d.External.All() {
		err := marshal(ext, true, w)
		if err != nil {
			return err
		}
	}

	for _, extAtt := range d.ExternalAttachment.All() {
		err := marshal(extAtt, true, w)
		if err != nil {
			return err
		}
	}

	// ugly output cleanup
	scan := bufio.NewScanner(w)
	for scan.Scan() {
		line := scan.Text()

		if slices.Contains([]string{"status: {}", "  creationTimestamp: null", "  position: {}", "    time: null", "  applied:", ""}, line) {
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

	rv := obj.GetResourceVersion()
	defer obj.SetResourceVersion(rv)

	obj.SetResourceVersion("")

	buf, err := yaml.Marshal(obj)
	if err != nil {
		return errors.Wrap(err, "error marshaling into yaml")
	}
	_, err = w.Write(buf)

	return errors.Wrap(err, "error writing yaml")
}
