// Copyright 2024 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package kubeutil

import (
	"context"
	"fmt"
	"io"
	"reflect"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kyaml "sigs.k8s.io/yaml"
)

func PrintObjectList(ctx context.Context, kube kclient.Reader, w io.Writer, objList meta.ObjectList, objs *int) error {
	if objs == nil {
		objs = new(int)
	}

	if err := kube.List(ctx, objList); err != nil {
		return fmt.Errorf("listing objects: %w", err)
	}

	if len(objList.GetItems()) > 0 {
		_, err := fmt.Fprintf(w, "#\n# %s\n#\n", reflect.TypeOf(objList).Elem().Name())
		if err != nil {
			return fmt.Errorf("writing comment: %w", err)
		}
	}

	for _, obj := range objList.GetItems() {
		if *objs > 0 {
			_, err := fmt.Fprintf(w, "---\n")
			if err != nil {
				return fmt.Errorf("writing separator: %w", err)
			}
		}
		*objs++

		if err := PrintObject(obj, w, false); err != nil {
			return fmt.Errorf("printing object: %w", err)
		}
	}

	return nil
}

type printObj struct {
	APIVersion string       `json:"apiVersion,omitempty"`
	Kind       string       `json:"kind,omitempty"`
	Meta       printObjMeta `json:"metadata,omitempty"`
	Spec       any          `json:"spec,omitempty"`
	Status     any          `json:"status,omitempty"`
}

type printObjMeta struct {
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

func PrintObject(obj kclient.Object, w io.Writer, withStatus bool) error {
	labels := obj.GetLabels()
	wiringapi.CleanupFabricLabels(labels)
	if len(labels) == 0 {
		labels = nil
	}

	annotations := obj.GetAnnotations()
	for key := range annotations {
		if key == "kubectl.kubernetes.io/last-applied-configuration" {
			delete(annotations, key)
		}
	}
	if len(annotations) == 0 {
		annotations = nil
	}

	p := printObj{
		APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
		Meta: printObjMeta{
			Name:        obj.GetName(),
			Namespace:   obj.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: reflect.ValueOf(obj).Elem().FieldByName("Spec").Interface(),
	}

	if withStatus {
		p.Status = reflect.ValueOf(obj).Elem().FieldByName("Status").Interface()
	}

	buf, err := kyaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshalling: %w", err)
	}
	_, err = w.Write(buf)
	if err != nil {
		return fmt.Errorf("writing: %w", err)
	}

	return nil
}
