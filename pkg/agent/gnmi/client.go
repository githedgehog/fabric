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

	"github.com/mitchellh/mapstructure"
	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/gnmic/target"
	"github.com/openconfig/ygot/ygot"
	"github.com/openconfig/ygot/ytypes"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/gnmi/bcom/oc"
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

	slog.Info("New agent user password generated and saved to password file")

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

			err = defC.Set(ctx, EntUser(AGENT_USER, agentPassword, "admin"))
			if err != nil {
				lastError = errors.Wrapf(err, "cannot set user %s with gnmi", user)
				slog.Debug("cannot set user with gnmi", "user", user, "err", err)
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
		api.Timeout(10*time.Second), // TODO
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

func (c *Client) SetEntry(ctx context.Context, summary, path string, value ygot.ValidatedGoStruct) error {
	return c.Set(ctx, &Entry{
		Summary: summary,
		Path:    path,
		Value:   value,
	})
}

func (c *Client) Set(ctx context.Context, entries ...*Entry) error {
	for _, entry := range entries {
		slog.Debug("Running gNMI set", "summary", entry.Summary)

		json, err := Marshal(entry.Value)
		if err != nil {
			return errors.Wrapf(err, "cannot marshal object: %s", entry.Summary)
		}

		setReq, err := api.NewSetRequest(api.Update(api.Path(entry.Path), api.Value(json, JSON_IETF)))
		if err != nil {
			return errors.Wrapf(err, "cannot create set request for: %s", entry.Summary)
		}
		// fmt.Println(prototext.Format(setReq))

		setResp, err := c.tg.Set(ctx, setReq)
		if err != nil {
			return errors.Wrapf(err, "set request failed for: %s", entry.Summary)
		}
		_ = setResp
		// fmt.Println(prototext.MarshalOptions{Multiline: true}.Format(setResp))
	}
	return nil
}

func (c *Client) Get(ctx context.Context, path string, dest ygot.ValidatedGoStruct) error {
	slog.Debug("Running gNMI get", "path", path)

	getReq, err := api.NewGetRequest(api.Path(path), api.Encoding(JSON_IETF))
	if err != nil {
		return errors.Wrapf(err, "cannot create get request for: %s", path)
	}
	// fmt.Println(prototext.Format(getReq))

	getResp, err := c.tg.Get(ctx, getReq)
	if err != nil {
		return errors.Wrapf(err, "get request failed for: %s", path)
	}
	// fmt.Println(prototext.Format(getResp))

	val := getResp.Notification[0].Update[0].Val.GetJsonIetfVal()
	if err := UnmarshalWithOpts(val, dest, extractOpt{}); err != nil {
		return errors.Wrapf(err, "cannot unmarshal response for: %s", path)
	}

	return nil
}

// TODO find better place for it?
func (c *Client) GetNOSInfo(ctx context.Context) (*agentapi.NOSInfo, error) {
	ocInfo := &oc.OpenconfigPlatform_Components_Component_SoftwareModule{}
	err := c.Get(ctx, "/openconfig-platform:components/component[name=SoftwareModule]/software-module", ocInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get NOS info")
	}

	info := &agentapi.NOSInfo{}
	err = mapstructure.Decode(ocInfo.State, info)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot convert NOS info")
	}

	return info, nil
}

func Marshal(value ygot.ValidatedGoStruct) (map[string]any, error) {
	data, err := ygot.ConstructIETFJSON(value, &ygot.RFC7951JSONConfig{
		AppendModuleName: true,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot construct json ietf from value")
	}

	return data, nil
}

func Unmarshal(data []byte, dest ygot.ValidatedGoStruct) error {
	return UnmarshalWithOpts(data, dest, extractOpt{})
}

func UnmarshalWithOpts(data []byte, dest ygot.ValidatedGoStruct, opts ...ytypes.UnmarshalOpt) error {
	typeName := reflect.TypeOf(dest).Elem().Name()
	schema, ok := oc.SchemaTree[typeName]
	if !ok {
		return fmt.Errorf("no schema for type %s", typeName)
	}

	var jsonTree map[string]interface{}
	if err := json.Unmarshal(data, &jsonTree); err != nil {
		return errors.Wrapf(err, "can't json unmarshal for type %s", typeName)
	}

	if hasExtractOpt(opts) {
		container := dest.ΛBelongingModule() + ":" + schema.Name
		if val, exists := jsonTree[container]; exists {
			return ytypes.Unmarshal(schema, dest, val, opts...)
		} else {
			return fmt.Errorf("can't extract from container %s", container)
		}
	}

	return ytypes.Unmarshal(schema, dest, jsonTree, opts...)
}

func hasExtractOpt(opts []ytypes.UnmarshalOpt) bool {
	for _, o := range opts {
		if _, ok := o.(extractOpt); ok {
			return true
		}
	}
	return false
}

type extractOpt struct{}

func (extractOpt) IsUnmarshalOpt() {}
