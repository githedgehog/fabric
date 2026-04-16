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

package bcm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	gnmiproto "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmic/pkg/api"
	gnmipath "github.com/openconfig/gnmic/pkg/api/path"
	"github.com/openconfig/ygot/ygot"
	"github.com/openconfig/ygot/ytypes"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
)

var (
	errMockGetUnmarshal      = errors.New("mock get: could not unmarshal into dest")
	errMockCallOpUnsupported = errors.New("gnmiMockClient.CallOperation is not implemented (test should not call it)")
)

// gnmiMockClient is a test-only gnmiClient that applies SetRequests to an
// in-memory *oc.Device root using ytypes (the same primitives a real gNMI
// server uses), so we can golden-test the device state ApplyActions would
// produce without talking to a real switch.
type gnmiMockClient struct {
	root *oc.Device
}

var _ GNMICClient = (*gnmiMockClient)(nil)

func newGNMIMock() *gnmiMockClient {
	return &gnmiMockClient{root: &oc.Device{}}
}

func (m *gnmiMockClient) Set(_ context.Context, req *gnmiproto.SetRequest) error {
	schema := oc.SchemaTree["Device"]

	// gNMI semantics: process deletes, then replaces, then updates.
	for _, p := range req.GetDelete() {
		if err := ytypes.DeleteNode(schema, m.root, stripPathOriginPrefixes(p)); err != nil {
			return fmt.Errorf("mock delete %s: %w", gnmipath.GnmiPathToXPath(p, false), err)
		}
	}
	for _, u := range req.GetReplace() {
		if err := m.applySet(u, true); err != nil {
			return fmt.Errorf("mock replace %s: %w", gnmipath.GnmiPathToXPath(u.Path, false), err)
		}
	}
	for _, u := range req.GetUpdate() {
		if err := m.applySet(u, false); err != nil {
			return fmt.Errorf("mock update %s: %w", gnmipath.GnmiPathToXPath(u.Path, false), err)
		}
	}

	return nil
}

// applySet writes a single gNMI update/replace into the OC root tree.
//
// Production Marshal funcs in spec_*.go return a parent ygot struct that
// already wraps the value with a field named after the path's last element
// (e.g., path=/ztp/config, value={"config": {...}}). The real switch's gNMI
// server is lenient about this off-by-one wrapping; ytypes.SetNode is not.
// We compensate by setting the value at the path's PARENT, which aligns the
// wrapped value with the schema's actual shape at that level.
func (m *gnmiMockClient) applySet(u *gnmiproto.Update, replace bool) error {
	schema := oc.SchemaTree["Device"]
	cleanPath := stripPathOriginPrefixes(u.Path)

	parentPath := cleanPath
	if len(cleanPath.Elem) > 0 {
		parentPath = &gnmiproto.Path{
			Origin: cleanPath.Origin,
			Target: cleanPath.Target,
			Elem:   cleanPath.Elem[:len(cleanPath.Elem)-1],
		}
	}

	if replace {
		// gNMI replace prunes the target subtree first; ignore "not found" on cold boot.
		_ = ytypes.DeleteNode(schema, m.root, cleanPath)
	}

	if err := ytypes.SetNode(schema, m.root, parentPath, u.Val,
		&ytypes.InitMissingElements{}, &ytypes.TolerateJSONInconsistencies{}); err != nil {
		return fmt.Errorf("ytypes.SetNode: %w", err)
	}

	return nil
}

// stripPathOriginPrefixes returns a copy of p with module-name prefixes
// (e.g. "openconfig-bfd:bfd") stripped from each element name. ytypes
// matches against schema entry names without the YANG module prefix.
func stripPathOriginPrefixes(p *gnmiproto.Path) *gnmiproto.Path {
	if p == nil {
		return nil
	}
	out := &gnmiproto.Path{
		Origin: p.Origin,
		Target: p.Target,
		Elem:   make([]*gnmiproto.PathElem, len(p.Elem)),
	}
	for i, e := range p.Elem {
		name := e.Name
		if idx := strings.Index(name, ":"); idx >= 0 {
			name = name[idx+1:]
		}
		out.Elem[i] = &gnmiproto.PathElem{Name: name, Key: e.Key}
	}

	return out
}

// Get reads a subtree from the in-memory OC root and unmarshals it into dest.
//
// The bcm code's loadActual* functions expect dest to be populated as if a
// real SONiC gNMI server returned the value AT the requested path — but with
// the same off-by-one wrapping that the Marshal funcs produce on writes
// (e.g., Get("/system/config", &System{}) expects {config: {hostname: ...}}
// to be unmarshaled into the System parent). To match this:
//
//  1. Locate the parent node (path minus its last element) in our state tree.
//  2. Marshal that parent to IETF JSON — it naturally includes the
//     last-element key as the wrapper.
//  3. Try to unmarshal that wrapped JSON into dest (matches the common case
//     where dest is the parent type).
//  4. On failure, unwrap one level and retry (matches the ZTP-style case
//     where dest is the leaf type).
//
// If the parent is missing/empty, return success with an untouched dest —
// the loadActual* callers handle empty results the same way they handle
// production gNMI's NotFound.
func (m *gnmiMockClient) Get(_ context.Context, path string, dest ygot.ValidatedGoStruct, _ ...api.GNMIOption) error {
	schema := oc.SchemaTree["Device"]

	p, err := gnmipath.ParsePath(path)
	if err != nil {
		return fmt.Errorf("mock get: parse path %s: %w", path, err)
	}
	p = stripPathOriginPrefixes(p)

	parentPath := p
	if len(p.Elem) > 0 {
		parentPath = &gnmiproto.Path{Origin: p.Origin, Target: p.Target, Elem: p.Elem[:len(p.Elem)-1]}
	}

	nodes, _ := ytypes.GetNode(schema, m.root, parentPath)
	if len(nodes) == 0 {
		return nil // no data at this subtree — leave dest at zero value
	}
	parentStruct, ok := nodes[0].Data.(ygot.GoStruct)
	if !ok {
		return nil
	}
	if rv := reflect.ValueOf(parentStruct); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil // parent container exists in schema but has no data
	}

	jsonMap, err := ygot.ConstructIETFJSON(parentStruct, &ygot.RFC7951JSONConfig{})
	if err != nil {
		return fmt.Errorf("mock get: construct ietf json for %s: %w", path, err)
	}
	if len(jsonMap) == 0 {
		return nil
	}

	rawBytes, err := json.Marshal(jsonMap)
	if err != nil {
		return fmt.Errorf("mock get: marshal json for %s: %w", path, err)
	}

	// Try unmarshaling the parent-wrapped JSON into dest.
	if err := gnmi.Unmarshal(rawBytes, dest); err == nil {
		return nil
	}

	// Fallback: dest matches the path's last element type (ZTP-style),
	// so unwrap one level and retry.
	if len(p.Elem) > 0 {
		lastName := p.Elem[len(p.Elem)-1].Name
		if inner, ok := jsonMap[lastName]; ok {
			innerBytes, err := json.Marshal(inner)
			if err != nil {
				return fmt.Errorf("mock get: marshal inner json for %s: %w", path, err)
			}
			if err := gnmi.Unmarshal(innerBytes, dest); err != nil {
				return fmt.Errorf("mock get: unmarshal %s into %T: %w", path, dest, err)
			}

			return nil
		}
	}

	return fmt.Errorf("%w: %s into %T", errMockGetUnmarshal, path, dest)
}

func (m *gnmiMockClient) CallOperation(_ context.Context, _ string, _ []byte) ([]byte, error) {
	return nil, errMockCallOpUnsupported
}

// StateMap returns the accumulated device state as RFC7951 IETF JSON in
// map[string]any form, ready for kyaml.Marshal.
func (m *gnmiMockClient) StateMap() (map[string]any, error) {
	out, err := gnmi.Marshal(m.root)
	if err != nil {
		return nil, fmt.Errorf("gnmi.Marshal root: %w", err)
	}

	return out, nil
}
