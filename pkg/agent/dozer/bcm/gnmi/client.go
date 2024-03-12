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

package gnmi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/gnmic/target"
	"github.com/openconfig/ygot/ygot"
	"github.com/openconfig/ygot/ytypes"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

const (
	JSON_IETF       = "json_ietf"
	TARGET          = "sonic"
	DEFAULT_ADDRESS = "127.0.0.1:8080"
	PASSWORD_FILE   = "agent-passwd"
	AGENT_USER      = "hhagent"
)

var (
	DEFAULT_USERS     = []string{"admin"}
	DEFAULT_PASSWORDS = []string{"YourPaSsWoRd"}
)

type Client struct {
	tg *target.Target
}

func NewInSONiC(ctx context.Context, basedir string, skipAgentUserCreation bool) (*Client, error) {
	_, err := os.Stat(filepath.Join(basedir, PASSWORD_FILE))
	if err != nil {
		if os.IsNotExist(err) {
			if skipAgentUserCreation {
				return nil, errors.Wrap(err, "password file does not exist")
			}

			slog.Info("Password file does not exist, creating new agent user")

			password, err := newAgentUser(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "cannot create new agent user")
			}
			err = os.WriteFile(filepath.Join(basedir, PASSWORD_FILE), password, 0o600)
			if err != nil {
				return nil, errors.Wrap(err, "cannot write password file")
			}

			slog.Info("New agent user password generated and saved to password file")
		} else {
			return nil, errors.Wrap(err, "cannot stat password file")
		}
	}

	// let's just read it to make sure password file is good
	password, err := os.ReadFile(filepath.Join(basedir, PASSWORD_FILE))
	if err != nil {
		return nil, errors.Wrap(err, "cannot read password file")
	}

	return New(ctx, DEFAULT_ADDRESS, AGENT_USER, string(password))
}

func New(ctx context.Context, address, username, password string) (*Client, error) {
	tg, err := createGNMIClient(ctx, address, username, password)
	if err != nil {
		return nil, err
	}

	_, err = tg.Capabilities(ctx) // TODO maybe check capabilities?
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get capabilities for %s@%s", username, address)
	}

	return &Client{
		tg: tg,
	}, nil
}

func (c *Client) Close() error {
	if c != nil && c.tg != nil {
		return c.tg.Close()
	}

	return nil
}

func newAgentUser(ctx context.Context) ([]byte, error) {
	agentPassword, err := RandomPassword()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot generate new agent user password")
	}

	// TODO move it to lib of smth
	username := AGENT_USER
	user := &oc.OpenconfigSystem_System_Aaa_Authentication_Users{
		User: map[string]*oc.OpenconfigSystem_System_Aaa_Authentication_Users_User{
			username: {
				Username: ygot.String(username),
				Config: &oc.OpenconfigSystem_System_Aaa_Authentication_Users_User_Config{
					Username: ygot.String(username),
					Password: ygot.String(agentPassword),
					Role:     oc.UnionString("admin"),
				},
			},
		},
	}
	data, err := Marshal(user)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot marshal user %s", username)
	}

	path := fmt.Sprintf("/openconfig-system:system/aaa/authentication/users/user[username=%s]", username)
	req, err := api.NewSetRequest(api.Update(api.Path(path), api.Value(data, JSON_IETF)))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create gnmi set request for user %s", username)
	}

	var lastError error
	for _, user := range DEFAULT_USERS {
		for _, password := range DEFAULT_PASSWORDS {
			defC, err := New(ctx, DEFAULT_ADDRESS, user, password)
			if err != nil {
				lastError = errors.Wrapf(err, "cannot init client with %s", user)
				slog.Debug("cannot init client", "user", user, "err", err)
				continue
			}
			defer defC.Close()

			err = defC.Set(ctx, req)
			if err != nil {
				lastError = errors.Wrapf(err, "cannot set user %s with gnmi", username)
				slog.Debug("cannot set user with gnmi", "user", username, "err", err)
				continue
			}

			return []byte(agentPassword), nil
		}
	}

	return nil, errors.Wrapf(lastError, "cannot create new agent user")
}

func createGNMIClient(ctx context.Context, address, username, password string) (*target.Target, error) {
	tg, err := api.NewTarget(
		api.Name(TARGET),
		api.Address(address),
		api.Username(username),
		api.Password(password),
		api.SkipVerify(true),        // TODO load keys from SONiC
		api.Timeout(30*time.Second), // TODO think about timeout
	)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create target for %s@%s", username, address)
	}

	err = tg.CreateGNMIClient(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create gnmi client for %s@%s", username, address)
	}

	return tg, nil
}

func (c *Client) Set(ctx context.Context, req *gnmi.SetRequest) error {
	_, err := c.tg.Set(ctx, req)
	if err != nil {
		return errors.Wrapf(err, "gnmi set request failed")
	}

	return nil
}

func (c *Client) Get(ctx context.Context, path string, dest ygot.ValidatedGoStruct, options ...api.GNMIOption) error {
	getReq, err := api.NewGetRequest(append(options, api.Encoding(JSON_IETF), api.Path(path))...)
	if err != nil {
		return errors.Wrapf(err, "cannot create get request for: %s", path)
	}

	getResp, err := c.tg.Get(ctx, getReq)
	if err != nil {
		return errors.Wrapf(err, "get request failed for: %s", path)
	}

	val := getResp.Notification[0].Update[0].Val.GetJsonIetfVal()
	if err := UnmarshalWithOpts(val, dest); err != nil {
		return errors.Wrapf(err, "cannot unmarshal response for: %s", path)
	}

	return nil
}

func (c *Client) GetWithOpts(ctx context.Context, path string, dest ygot.ValidatedGoStruct, extract bool, options ...api.GNMIOption) error {
	getReq, err := api.NewGetRequest(append(options, api.Encoding(JSON_IETF), api.Path(path))...)
	if err != nil {
		return errors.Wrapf(err, "cannot create get request for: %s", path)
	}

	getResp, err := c.tg.Get(ctx, getReq)
	if err != nil {
		return errors.Wrapf(err, "get request failed for: %s", path)
	}

	// TODO drop extract opt?
	opts := []ytypes.UnmarshalOpt{}
	if extract {
		opts = append(opts, ExtractOpt{})
	}

	val := getResp.Notification[0].Update[0].Val.GetJsonIetfVal()
	if err := UnmarshalWithOpts(val, dest, opts...); err != nil {
		return errors.Wrapf(err, "cannot unmarshal response for: %s", path)
	}

	return nil
}

func Marshal(value ygot.ValidatedGoStruct) (map[string]any, error) {
	data, err := ygot.ConstructIETFJSON(value, &ygot.RFC7951JSONConfig{})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot construct json ietf from value")
	}

	return data, nil
}

func Unmarshal(data []byte, dest ygot.ValidatedGoStruct) error {
	return UnmarshalWithOpts(data, dest)
}

func UnmarshalWithOpts(data []byte, dest ygot.ValidatedGoStruct, opts ...ytypes.UnmarshalOpt) error {
	typeName := reflect.TypeOf(dest).Elem().Name()
	schema, ok := oc.SchemaTree[typeName]
	if !ok {
		return errors.Errorf("no schema for type %s", typeName)
	}

	var jsonTree map[string]interface{}
	if err := json.Unmarshal(data, &jsonTree); err != nil {
		return errors.Wrapf(err, "can't json unmarshal for type %s", typeName)
	}

	opts = append(opts, &ytypes.IgnoreExtraFields{})

	if hasExtractOpt(opts) {
		container := dest.ΛBelongingModule() + ":" + schema.Name
		if val, exists := jsonTree[container]; exists {
			return ytypes.Unmarshal(schema, dest, val, opts...)
		} else {
			return errors.Errorf("can't extract from container %s", container)
		}
	}

	return ytypes.Unmarshal(schema, dest, jsonTree, opts...)
}

func hasExtractOpt(opts []ytypes.UnmarshalOpt) bool {
	for _, o := range opts {
		if _, ok := o.(ExtractOpt); ok {
			return true
		}
	}
	return false
}

type ExtractOpt struct{}

func (ExtractOpt) IsUnmarshalOpt() {}
