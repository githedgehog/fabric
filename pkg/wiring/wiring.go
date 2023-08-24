package wiring

import (
	"sort"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Data struct {
	Rack   *Store[*wiringapi.Rack]
	Switch *Store[*wiringapi.Switch]
	Port   *Store[*wiringapi.SwitchPort]
}

func New(objs ...metav1.Object) (*Data, error) {
	data := &Data{
		Rack:   NewStore[*wiringapi.Rack](),
		Switch: NewStore[*wiringapi.Switch](),
		Port:   NewStore[*wiringapi.SwitchPort](),
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
		case *wiringapi.SwitchPort:
			d.Port.Add(typed)
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
