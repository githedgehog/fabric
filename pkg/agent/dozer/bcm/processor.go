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
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/openconfig/gnmic/api"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/uefiutil"
)

type BroadcomProcessor struct {
	client *gnmi.Client
}

var _ dozer.Processor = &BroadcomProcessor{}

func Processor() (*BroadcomProcessor, error) {
	return &BroadcomProcessor{}, initCompat()
}

func (p *BroadcomProcessor) SetClient(client *gnmi.Client) {
	p.client = client
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

	slog.Info("Waiting for system is ready")

	timeout := 10 * time.Minute
	retriesStart := time.Now()
	for time.Since(retriesStart) < timeout {
		resp, err := p.client.CallOperation(ctx, "openconfig-system-rpc:show-system-status", nil)
		if err != nil {
			slog.Warn("Failed to get system status", "err", err)
		} else {
			st := &SystemStatusResponse{}
			if err := json.Unmarshal(resp, st); err != nil {
				slog.Warn("Failed to parse system status", "err", err)
			} else {
				notReady, total := 0, 0
				notReadyList := []string{}
				for _, detail := range st.Output.Details {
					// skip column headers
					if strings.HasPrefix(detail, "System") || strings.HasPrefix(detail, "Service-Name") {
						continue
					}

					// skip malformed details lines
					parts := strings.Fields(detail)
					if len(parts) < 3 {
						continue
					}

					// skip agent, alloy and other potential hedgehog units
					if strings.Contains(parts[0], "hedgehog") {
						continue
					}

					total++
					if parts[1] != "OK" || parts[2] != "OK" {
						notReady++
						notReadyList = append(notReadyList, parts[0])
					}
				}

				if notReady == 0 {
					slog.Info("System is ready")

					return nil
				}

				slices.Sort(notReadyList)
				slog.Debug("System is not ready", "summary", fmt.Sprintf("%d/%d", total-notReady, total), "notReady", notReadyList)
			}
		}

		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("system is not ready after %f minutes", timeout.Minutes()) //nolint:err113
}

type SystemStatusResponse struct {
	Output struct {
		Status  int      `json:"status"`
		Details []string `json:"status-detail"`
	} `json:"openconfig-system-rpc:output"`
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
	if p.client == nil {
		return nil, errors.New("gnmi client is not set")
	}

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
	if p.client == nil {
		return nil, errors.New("gnmi client is not set")
	}

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
