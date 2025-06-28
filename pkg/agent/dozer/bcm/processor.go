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
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"github.com/openconfig/gnmic/api"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/logutil"
	"go.githedgehog.com/fabric/pkg/util/uefiutil"
)

type BroadcomProcessor struct {
	client *gnmi.Client
}

var _ dozer.Processor = &BroadcomProcessor{}

func Processor(client *gnmi.Client) (*BroadcomProcessor, error) {
	return &BroadcomProcessor{
		client: client,
	}, initCompat()
}

func (p *BroadcomProcessor) WaitReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	// TODO replace with better handling
	cmd := exec.CommandContext(ctx, "bash", "-c", "(sudo dmidecode -t system | grep 'QEMU') && (sudo iptables -t filter -C INPUT -p udp --dport 4789 -j ACCEPT || sudo iptables -t filter -I INPUT 1 -p udp --dport 4789 -j ACCEPT) || true")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	err := cmd.Run()
	if err != nil {
		return errors.Wrap(err, "failed to fix iptables for vxlan on VS")
	}

	slog.Debug("Checking if system is ready")

	lOut := logutil.NewSink(ctx, slog.Debug, "status: ")
	lErr := logutil.NewSink(ctx, slog.Warn, "status: ")

	retriesStart := time.Now()
	for time.Since(retriesStart) < 10*time.Minute {
		buf := &bytes.Buffer{}
		cmd := exec.CommandContext(ctx, "su", "-c", "sonic-cli -c \"show system status brief\"", gnmi.AgentUser) //nolint:gosec
		cmd.Stdout = io.MultiWriter(buf, lOut)
		cmd.Stderr = lErr
		err := cmd.Run()
		if err != nil {
			slog.Warn("Failed to run sonic-cli: show system status brief", "err", err)
		} else if bytes.Contains(buf.Bytes(), []byte("System is ready")) {
			slog.Info("System is ready")

			break
		}

		time.Sleep(15 * time.Second)
	}

	return nil
}

func (p *BroadcomProcessor) Reboot(ctx context.Context, _ /* force */ bool) error {
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

func (p *BroadcomProcessor) Reinstall(ctx context.Context) error {
	err := uefiutil.MakeONIEDefaultBootEntryAndCleanup()
	if err != nil {
		return errors.Wrapf(err, "failed to make ONIE default boot entry")
	}

	return p.Reboot(ctx, true)
}

func (p *BroadcomProcessor) FactoryReset(_ context.Context) error {
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

func (p *BroadcomProcessor) LoadActualState(ctx context.Context, agent *agentapi.Agent) (*dozer.Spec, error) {
	spec := &dozer.Spec{}

	if err := loadActualSpec(ctx, agent, p.client, spec); err != nil {
		return nil, errors.Wrapf(err, "failed to load actual state")
	}

	spec.Normalize()

	return spec, nil
}

func (p *BroadcomProcessor) CalculateActions(_ context.Context, actual, desired *dozer.Spec) ([]dozer.Action, error) {
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

func (p *BroadcomProcessor) ApplyActions(ctx context.Context, actions []dozer.Action) ([]string, error) {
	for idx, action := range actions {
		act := action.(*Action)

		if act.CustomFunc != nil {
			slog.Debug("Action", "idx", idx, "weight", act.Weight, "summary", action.Summary())

			err := act.CustomFunc(ctx, p.client)
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
			switch act.Type {
			case ActionTypeUpdate:
				options = append(options, api.Update(api.Path(act.Path), api.Value(ocData, gnmi.JSONIETFEncoding)))
			case ActionTypeReplace:
				options = append(options, api.Replace(api.Path(act.Path), api.Value(ocData, gnmi.JSONIETFEncoding)))
			case ActionTypeDelete:
				options = append(options, api.Delete(act.Path))
			default:
				return nil, errors.Errorf("unsupported gnmi action %+v", act)
			}

			for attempt := 0; attempt < 50; attempt++ {
				req, err := api.NewSetRequest(options...)
				if err != nil {
					return nil, errors.Wrapf(err, "cannot create GNMI set request")
				}

				if err := p.client.Set(ctx, req); err != nil {
					// workaround for port breakout being still in progress when configuring interfaces
					if strings.Contains(err.Error(), "Port breakout is in progress") {
						slog.Warn("Port breakout is in progress, retrying in 2 seconds")
						time.Sleep(2 * time.Second)

						continue // retry
					}

					return nil, errors.Wrapf(err, "GNMI set request failed")
				}

				break // retries
			}
		}

		slog.Info("Action applied", "idx", idx, "summary", action.Summary())
	}

	return nil, nil
}

func (p *BroadcomProcessor) GetRoCE(ctx context.Context) (bool, error) {
	ocVal := &oc.SonicSwitch_SonicSwitch_SWITCH{}
	err := p.client.Get(ctx, "/sonic-switch/SWITCH/SWITCH_LIST[switch=switch]", ocVal)
	if err != nil {
		return false, fmt.Errorf("reading RoCE state: %w", err) //nolint:goerr113
	}

	for key, sw := range ocVal.SWITCH_LIST {
		if key != oc.SonicSwitch_SonicSwitch_SWITCH_SWITCH_LIST_Switch_switch {
			continue
		}

		if sw.RoceEnable != nil {
			return *sw.RoceEnable, nil
		}
	}

	return false, nil
}

func (p *BroadcomProcessor) SetRoCE(ctx context.Context, val bool) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	action := "ENABLE"
	if !val {
		action = "DISABLE"
	}

	resp, err := p.client.CallOperation(ctx, "openconfig-qos-private:qos-roce-config",
		[]byte(fmt.Sprintf(`{"openconfig-qos-private:input":{"operation":"%s"}}`, action)))

	// it just hangs so timeout is expected
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		slog.Warn("RoCE set operation failed", "error", err, "data", string(resp), "action", action)

		return fmt.Errorf("calling RoCE set operation: %w", err)
	}
	if err == nil {
		slog.Warn("RoCE set operation unexpected result", "data", string(resp), "action", action)

		return fmt.Errorf("unexpected response from RoCE set operation") //nolint:goerr113
	}

	return nil
}
