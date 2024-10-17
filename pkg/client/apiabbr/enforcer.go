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

package apiabbr

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Enforcer struct {
	handlers []AbbrHandler
}

func NewEnforcer(ignoreNotDefined bool) (*Enforcer, error) {
	e := &Enforcer{}

	vpc, err := newVPCHandler(ignoreNotDefined)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create VPC abbr handler")
	}

	vpcAttachment, err := newVPCAttachmentHandler(ignoreNotDefined)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create VPCAttachment abbr handler")
	}

	vpcPeering, err := newVPCPeeringHandler(ignoreNotDefined)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create VPCPeering abbr handler")
	}

	extPeering, err := newExternalPeeringHandler(ignoreNotDefined)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create ExternalPeering abbr handler")
	}

	fallback, err := newConnectionFallbackHandler(ignoreNotDefined)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create Connection fallback abbr handler")
	}

	e.handlers = []AbbrHandler{
		vpc,
		vpcAttachment,
		vpcPeering,
		extPeering,
		fallback,
	}

	return e, nil
}

func (e *Enforcer) Load(lines ...string) error {
	for _, line := range lines {
		abbrs := strings.Fields(line)
		for _, abbr := range abbrs {
			if strings.HasPrefix(abbr, "#") {
				continue
			}

			parts := strings.Split(abbr, ":")
			if len(parts) < 1 {
				return errors.Errorf("invalid abbr: %s", abbr)
			}

			abbrType := AbbrType(parts[0])
			if !slices.Contains(AbbrTypes, abbrType) {
				abbrType = AbbrTypeUnknown
			} else {
				parts = parts[1:]
			}

			abbrPart := parts[0]
			parts = parts[1:]

			params := AbbrParams{}
			for _, part := range parts {
				kv := strings.SplitN(part, "=", 2)

				key := kv[0]
				val := TrueVals[0]
				if len(kv) == 2 {
					val = kv[1]
				}

				if _, ok := params[key]; !ok {
					params[key] = []string{}
				}
				params[key] = append(params[key], val)
			}

			handles := 0
			for _, handler := range e.handlers {
				if handled, err := handler.Load(abbrType, abbrPart, params); err != nil {
					return errors.Wrapf(err, "failed to load abbr: %s", abbr)
				} else if handled {
					handles++
				}
			}

			if handles == 0 {
				return errors.Errorf("no handler found for abbr: %s", abbr)
			} else if handles > 1 {
				return errors.Errorf("multiple handlers found for abbr: %s", abbr)
			}
		}
	}

	return nil
}

func (e *Enforcer) Enforce(ctx context.Context, kube client.Client) error {
	for i := len(e.handlers) - 1; i >= 0; i-- {
		handler := e.handlers[i]
		if err := handler.PreProcess(ctx, kube); err != nil {
			return errors.Wrapf(err, "failed to process deletes")
		}
	}

	for _, handler := range e.handlers {
		if err := handler.PostProcess(ctx, kube); err != nil {
			return errors.Wrapf(err, "failed to process updates")
		}
	}

	return nil
}

type AbbrType string

const (
	AbbrTypeUnknown            AbbrType = ""
	AbbrTypeVPC                AbbrType = "vpc"
	AbbrTypeVPCAttachment      AbbrType = "vpcAttach"
	AbbrTypeVPCPeering         AbbrType = "vpcPeering"
	AbbrTypeExternalPeering    AbbrType = "extPeering"
	AbbrTypeConnectionFallback AbbrType = "fallback"
)

var AbbrTypes = []AbbrType{
	AbbrTypeVPC,
	AbbrTypeConnectionFallback,
}

var (
	TrueVals        = []string{"true", "t", "yes", "y", "1"}
	TrueValsDefault = append(TrueVals, "")
)

type AbbrParams map[string][]string

func (p AbbrParams) GetBool(keys []string) (bool, error) {
	ret := false
	found := false

	for _, key := range keys {
		if vals, ok := p[key]; ok {
			if found {
				return false, errors.Errorf("multiple key aliases for key %s", key)
			}

			if len(vals) > 1 {
				return false, errors.Errorf("multiple values for key %s", key)
			}

			ret = slices.Contains(TrueVals, vals[0])
			found = true
		}
	}

	return ret, nil
}

func (p AbbrParams) GetString(keys []string) (string, error) {
	var ret string
	found := false

	for _, key := range keys {
		if vals, ok := p[key]; ok {
			if found {
				return "", errors.Errorf("multiple key aliases for key %s", key)
			}

			if len(vals) > 1 {
				return "", errors.Errorf("multiple values for key %s", key)
			}

			ret = vals[0]
			found = true
		}
	}

	return ret, nil
}

func (p AbbrParams) GetStringSlice(keys []string) []string {
	var ret []string

	for _, key := range keys {
		if vals, ok := p[key]; ok {
			ret = append(ret, vals...)
		}
	}

	return ret
}

type AbbrHandler interface {
	Load(ct AbbrType, abbr string, params AbbrParams) (handled bool, err error)
	PreProcess(ctx context.Context, kube client.Client) error
	PostProcess(ctx context.Context, kube client.Client) error
}

type ObjectAbbrHandler[T meta.Object, TList meta.ObjectList] struct {
	AbbrType         AbbrType
	AcceptedParams   [][]string
	AcceptNoTypeFn   func(abbr string) bool
	NameFn           func(abbr string) (name string)
	ParseObjectFn    func(name, abbr string, params AbbrParams) (T, error)
	ObjectListFn     func(ctx context.Context, kube client.Client) (TList, error)
	CreateOrUpdateFn func(ctx context.Context, kube client.Client, obj T) (ctrlutil.OperationResult, error)
	PatchExistingFn  func(obj T) bool

	DisallowOverride  bool
	CleanupNotDefined bool

	objs map[string]T
}

var _ AbbrHandler = (*ObjectAbbrHandler[*vpcapi.VPC, *vpcapi.VPCList])(nil)

func (h *ObjectAbbrHandler[T, TList]) Init() (*ObjectAbbrHandler[T, TList], error) {
	if h.AbbrType == AbbrTypeUnknown {
		return nil, errors.New("AbbrType is required")
	}
	if h.ParseObjectFn == nil {
		return nil, errors.New("ParseObjectFn is required")
	}
	if h.ObjectListFn == nil {
		return nil, errors.New("ObjectListFn is required")
	}
	if h.CreateOrUpdateFn == nil {
		return nil, errors.New("CreateOrUpdateFn is required")
	}

	h.objs = map[string]T{}

	return h, nil
}

func (h *ObjectAbbrHandler[T, TList]) Load(ct AbbrType, abbr string, params AbbrParams) (bool, error) {
	if ct != AbbrTypeUnknown && ct != h.AbbrType {
		return false, nil
	}

	if ct == AbbrTypeUnknown && (h.AcceptNoTypeFn == nil || !h.AcceptNoTypeFn(abbr)) {
		return false, nil
	}

	name := abbr
	if h.NameFn != nil {
		name = h.NameFn(abbr)
	}

	obj, err := h.ParseObjectFn(name, abbr, params)
	if err != nil {
		return false, errors.Wrapf(err, "failed to parse object for command: %s", abbr)
	}

	_, exists := h.objs[obj.GetName()]
	if h.DisallowOverride && exists {
		return false, errors.Errorf("object already defined: %s", name)
	}

	h.objs[obj.GetName()] = obj

	return true, nil
}

func (h *ObjectAbbrHandler[T, TList]) PreProcess(ctx context.Context, kube client.Client) error {
	if !h.CleanupNotDefined {
		return nil
	}

	list, err := h.ObjectListFn(ctx, kube)
	if err != nil {
		return errors.Wrapf(err, "failed to list objects: %s", list.GetObjectKind().GroupVersionKind().Kind)
	}

	for _, obj := range list.GetItems() {
		name := obj.GetName()
		if _, ok := h.objs[name]; !ok {
			kind := obj.GetObjectKind().GroupVersionKind().Kind
			slog.Debug("deleting not defined object", "kind", kind, "name", name)
			if err := kube.Delete(ctx, obj); err != nil {
				return errors.Wrapf(err, "failed to delete object: %s/%s", kind, name)
			}

			slog.Debug("object deleted", "kind", kind, "name", name)
		}
	}

	return nil
}

func (h *ObjectAbbrHandler[T, TList]) PostProcess(ctx context.Context, kube client.Client) error {
	if h.PatchExistingFn != nil {
		list, err := h.ObjectListFn(ctx, kube)
		if err != nil {
			return errors.Wrapf(err, "failed to list objects: %s", list.GetObjectKind().GroupVersionKind().Kind)
		}

		for _, obj := range list.GetItems() {
			if typed, ok := obj.(T); ok {
				if h.PatchExistingFn(typed) {
					name, kind := obj.GetName(), obj.GetObjectKind().GroupVersionKind().Kind
					slog.Debug("patching existing object", "kind", kind, "name", name)
					if err := kube.Update(ctx, typed); err != nil {
						return errors.Wrapf(err, "failed to patch object: %s/%s", kind, name)
					}

					slog.Debug("patched existing object", "kind", kind, "name", name)
				}
			} else {
				return errors.Errorf("object is not of expected type")
			}
		}
	}

	for _, obj := range h.objs {
		name, kind := obj.GetName(), obj.GetObjectKind().GroupVersionKind().Kind
		slog.Debug("enforcing object", "kind", kind, "name", name)
		if res, err := h.CreateOrUpdateFn(ctx, kube, obj); err != nil {
			return errors.Wrapf(err, "failed to enforce object: %s/%s", kind, name)
		} else if res != ctrlutil.OperationResultNone {
			slog.Debug("object "+string(res), "kind", kind, "name", name)
		}
	}

	return nil
}
