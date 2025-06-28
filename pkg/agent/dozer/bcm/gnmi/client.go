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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

const (
	JSONIETFEncoding  = "json_ietf"
	Target            = "sonic"
	GNMIAPIAddress    = "127.0.0.1:8080"
	RestAPIAddress    = "https://127.0.0.1:443/restconf/operations"
	AgentUser         = "hhagent"
	AgentPasswordFile = "agent-passwd"
)

var (
	DefaultUsers     = []string{"admin"}
	DefaultPasswords = []string{"YourPaSsWoRd"}
)

type Client struct {
	tg       *target.Target
	address  string
	username string
	password string
}

func NewInSONiC(ctx context.Context, basedir string, skipAgentUserCreation bool) (*Client, error) {
	_, err := os.Stat(filepath.Join(basedir, AgentPasswordFile))
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
			err = os.WriteFile(filepath.Join(basedir, AgentPasswordFile), password, 0o600)
			if err != nil {
				return nil, errors.Wrap(err, "cannot write password file")
			}

			slog.Info("New agent user password generated and saved to password file")
		} else {
			return nil, errors.Wrap(err, "cannot stat password file")
		}
	}

	// let's just read it to make sure password file is good
	password, err := os.ReadFile(filepath.Join(basedir, AgentPasswordFile))
	if err != nil {
		return nil, errors.Wrap(err, "cannot read password file")
	}

	return New(ctx, GNMIAPIAddress, AgentUser, string(password))
}

func New(ctx context.Context, address, username, password string) (*Client, error) {
	client := &Client{
		address:  address,
		username: username,
		password: password,
	}

	if err := client.Connect(ctx); err != nil {
		return nil, errors.Wrapf(err, "cannot connect to %s@%s", username, address)
	}

	return client, nil
}

func (c *Client) Connect(ctx context.Context) error {
	tg, err := createGNMIClient(ctx, c.address, c.username, c.password)
	if err != nil {
		return err
	}

	_, err = tg.Capabilities(ctx) // TODO maybe check capabilities?
	if err != nil {
		return errors.Wrapf(err, "cannot get capabilities for %s@%s", c.username, c.address)
	}

	c.tg = tg

	return nil
}

func (c *Client) Close() error {
	if c != nil && c.tg != nil {
		return errors.Wrapf(c.tg.Close(), "cannot close gnmi client")
	}

	return nil
}

func (c *Client) Reconnect(ctx context.Context) error {
	if err := c.Close(); err != nil {
		slog.Warn("Failed to close GNMI client", "err", err)
	}

	return c.Connect(ctx)
}

func newAgentUser(ctx context.Context) ([]byte, error) {
	agentPassword, err := RandomPassword()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot generate new agent user password")
	}

	// TODO move it to lib of smth
	username := AgentUser
	user := &oc.OpenconfigSystem_System_Aaa_Authentication_Users{
		User: map[string]*oc.OpenconfigSystem_System_Aaa_Authentication_Users_User{
			username: {
				Username: pointer.To(username),
				Config: &oc.OpenconfigSystem_System_Aaa_Authentication_Users_User_Config{
					Username: pointer.To(username),
					Password: pointer.To(agentPassword),
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
	req, err := api.NewSetRequest(api.Update(api.Path(path), api.Value(data, JSONIETFEncoding)))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create gnmi set request for user %s", username)
	}

	var lastError error
	for _, user := range DefaultUsers {
		for _, password := range DefaultPasswords {
			defC, err := New(ctx, GNMIAPIAddress, user, password)
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
		api.Name(Target),
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
	getReq, err := api.NewGetRequest(append(options, api.Encoding(JSONIETFEncoding), api.Path(path))...)
	if err != nil {
		return errors.Wrapf(err, "cannot create get request for: %s", path)
	}

	getResp, err := c.tg.Get(ctx, getReq)
	if err != nil {
		return errors.Wrapf(err, "get request failed for: %s", path)
	}

	val := getResp.Notification[0].Update[0].Val.GetJsonIetfVal()
	if err := Unmarshal(val, dest); err != nil {
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

func Unmarshal(data []byte, dest ygot.ValidatedGoStruct, opts ...ytypes.UnmarshalOpt) error {
	typeName := reflect.TypeOf(dest).Elem().Name()
	schema, ok := oc.SchemaTree[typeName]
	if !ok {
		return errors.Errorf("no schema for type %s", typeName)
	}

	var jsonTree interface{}
	if err := json.Unmarshal(data, &jsonTree); err != nil {
		return errors.Wrapf(err, "can't json unmarshal for type %s", typeName)
	}

	opts = append(opts, &ytypes.IgnoreExtraFields{})

	return errors.Wrapf(ytypes.Unmarshal(schema, dest, jsonTree, opts...), "error unmarshaling for type %s", typeName)
}

var ErrReqFailed = errors.New("request failed")

func (c *Client) CallOperation(ctx context.Context, path string, paylod []byte) ([]byte, error) {
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // TODO properly setup SONiC TLS
	}
	client := &http.Client{
		Transport: baseTransport,
		Timeout:   30 * time.Second,
	}

	reqURL, err := url.JoinPath(RestAPIAddress, path)
	if err != nil {
		return nil, fmt.Errorf("joining path %s with rest api address %s: %w", path, RestAPIAddress, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(paylod))
	if err != nil {
		return nil, fmt.Errorf("creating http request to %s: %w", reqURL, err)
	}
	req.Header.Set("Content-Type", "application/yang-data+json")
	req.SetBasicAuth(AgentUser, c.password)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doing http request to %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body from %s: %w", reqURL, err)
	}

	if resp.StatusCode != http.StatusOK {
		return respBody, fmt.Errorf("%w: http request to %s failed with status code %d", ErrReqFailed, reqURL, resp.StatusCode)
	}

	return respBody, nil
}
