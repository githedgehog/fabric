package bcm

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"reflect"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/openconfig/gnmic/api"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
	"go.githedgehog.com/fabric/pkg/util/uefiutil"
)

type broadcomProcessor struct {
	client *gnmi.Client
}

var _ dozer.Processor = &broadcomProcessor{}

func Processor(client *gnmi.Client) *broadcomProcessor {
	return &broadcomProcessor{
		client: client,
	}
}

func (p *broadcomProcessor) WaitReady(ctx context.Context) error {
	// TODO think about better timeout handling
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	for {
		slog.Debug("Checking if system is ready")

		buf := &bytes.Buffer{}
		// TODO figure out how to call gNMI actions(rpcs?) from agent
		cmd := exec.CommandContext(ctx, "su", "-c", "sonic-cli -c \"show system status brief\"", "admin") // TODO use hhadmin user
		cmd.Stdout = io.MultiWriter(buf, os.Stdout)
		cmd.Stderr = os.Stdout
		err := cmd.Run()
		if err != nil {
			return errors.Wrap(err, "failed to run sonic-cli: show system status brief")
		}

		if bytes.Contains(buf.Bytes(), []byte("System is ready")) {
			break
		}

		time.Sleep(3 * time.Second)
	}

	return nil
}

func (p *broadcomProcessor) Reboot(ctx context.Context, force bool) error {
	cmd := exec.CommandContext(ctx, "wall", "Hedgehog Agent initiated reboot")
	err := cmd.Run()
	if err != nil {
		slog.Warn("Failed to send wall message", "err", err)
	}

	// TODO impl force
	// TODO use sonic-cli for it and then switch to GNOI
	// reboot force yes
	cmd = exec.CommandContext(ctx, "reboot")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return errors.Wrap(cmd.Run(), "failed to reboot")
}

func (p *broadcomProcessor) Reinstall(ctx context.Context) error {
	err := uefiutil.MakeONIEDefaultBootEntryAndCleanup()
	if err != nil {
		return errors.Wrapf(err, "failed to make ONIE default boot entry")
	}

	return p.Reboot(ctx, true)
}

func (p *broadcomProcessor) FactoryReset(ctx context.Context) error {
	// TODO use sonic-cli for it and then switch to GNOI
	// write erase boot

	// stdin, err := cmd.StdinPipe()
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// go func() {
	// 	defer stdin.Close()
	// 	// todo test it
	// 	io.WriteString(stdin, "y\n")
	// }()

	panic("unimplemented")
}

func (p *broadcomProcessor) LoadActualState(ctx context.Context) (*dozer.Spec, error) {
	spec := &dozer.Spec{}

	if err := loadActualSpec(ctx, p.client, spec); err != nil {
		return nil, errors.Wrapf(err, "failed to load actual state")
	}

	spec.Normalize()

	return spec, nil
}

func (p *broadcomProcessor) CalculateActions(ctx context.Context, actual, desired *dozer.Spec) ([]dozer.Action, error) {
	if reflect.DeepEqual(actual, desired) {
		return []dozer.Action{}, nil
	}

	actions := &ActionQueue{}

	if err := specEnforcer.Handle("", "root", actual, desired, actions); err != nil {
		return nil, errors.Wrap(err, "failed to handle spec")
	}

	actions.Sort()

	return actions.actions, nil
}

func (p *broadcomProcessor) ApplyActions(ctx context.Context, actions []dozer.Action) ([]string, error) {
	for idx, action := range actions {
		act := action.(*Action)

		if act.CustomFunc != nil {
			slog.Debug("Action", "idx", idx, "weight", act.Weight, "summary", action.Summary())

			err := act.CustomFunc()
			if err != nil {
				return nil, errors.Wrapf(err, "failed to run custom action")
			}
		} else {
			slog.Debug("Action", "idx", idx, "weight", act.Weight, "summary", action.Summary(), "command", act.Type, "path", act.Path)

			var ocData map[string]any
			var err error
			if act.Value != nil && !(reflect.ValueOf(act.Value).Kind() == reflect.Ptr && reflect.ValueOf(act.Value).IsNil()) {
				ocData, err = gnmi.Marshal(act.Value)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to OC marshal gnmi action value")
				}
			}

			options := []api.GNMIOption{}
			if act.Type == ActionTypeUpdate {
				options = append(options, api.Update(api.Path(act.Path), api.Value(ocData, gnmi.JSON_IETF)))
			} else if act.Type == ActionTypeReplace {
				options = append(options, api.Replace(api.Path(act.Path), api.Value(ocData, gnmi.JSON_IETF)))
			} else if act.Type == ActionTypeDelete {
				options = append(options, api.Delete(act.Path))
			} else {
				return nil, errors.Errorf("unsupported gnmi action %+v", act)
			}

			req, err := api.NewSetRequest(options...)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot create GNMI set request")
			}

			err = p.client.Set(ctx, req)
			if err != nil {
				return nil, errors.Wrapf(err, "GNMI set request failed")
			}
		}

		slog.Info("Action applied", "idx", idx, "summary", action.Summary())
	}

	return nil, nil
}

func (p *broadcomProcessor) Info(ctx context.Context) (*agentapi.NOSInfo, error) {
	ocInfo := &oc.OpenconfigPlatform_Components_Component_SoftwareModule{}
	err := p.client.Get(ctx, "/openconfig-platform:components/component[name=SoftwareModule]/software-module", ocInfo)
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
