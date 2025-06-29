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

package hhfctl

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"strings"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	HHFabCfgPrefix          = ".hhfab.githedgehog.com"
	HHFabCfgSerial          = "serial" + HHFabCfgPrefix
	HHFabCfgPower           = "power" + HHFabCfgPrefix
	HHFctlCfgPrefix         = ".fabric.githedgehog.com"
	HHFctlCfgSerial         = "serial" + HHFctlCfgPrefix
	HHFabCfgSerialSchemeSSH = "ssh://"
)

var SSHQuietFlags = []string{
	"-o", "GlobalKnownHostsFile=/dev/null",
	"-o", "UserKnownHostsFile=/dev/null",
	"-o", "StrictHostKeyChecking=no",
	"-o", "LogLevel=ERROR",
}

func getAgent(ctx context.Context, kube kclient.Reader, name string) (*agentapi.Agent, error) {
	agent := &agentapi.Agent{}
	err := kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, agent)
	if err != nil {
		return nil, fmt.Errorf("getting agent: %w", err)
	}

	return agent, nil
}

func SwitchReboot(ctx context.Context, name string) error {
	kube, err := kubeutil.NewClient(ctx, "", agentapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	agent, err := getAgent(ctx, kube, name)
	if err != nil {
		return err
	}

	if agent.Status.BootID == "" {
		return fmt.Errorf("agent is not running (missing .status.bootID)") //nolint:goerr113
	}

	agent.Spec.Reboot = agent.Status.BootID
	err = kube.Update(ctx, agent)
	if err != nil {
		return fmt.Errorf("updating agent object: %w", err)
	}

	return nil
}

func SwitchPowerReset(ctx context.Context, name string) error {
	kube, err := kubeutil.NewClient(ctx, "", agentapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	agent, err := getAgent(ctx, kube, name)
	if err != nil {
		return err
	}

	if agent.Status.BootID == "" {
		return fmt.Errorf("agent is not running (missing .status.bootID)") //nolint:goerr113
	}

	agent.Spec.PowerReset = agent.Status.BootID
	err = kube.Update(ctx, agent)
	if err != nil {
		return fmt.Errorf("updating agent object: %w", err)
	}

	return nil
}

func SwitchReinstall(ctx context.Context, name string) error {
	kube, err := kubeutil.NewClient(ctx, "", agentapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	agent, err := getAgent(ctx, kube, name)
	if err != nil {
		return err
	}

	if agent.Status.InstallID == "" {
		return fmt.Errorf("agent is not installed (missing .status.installID)") //nolint:goerr113
	}

	agent.Spec.Reinstall = agent.Status.InstallID
	err = kube.Update(ctx, agent)
	if err != nil {
		return fmt.Errorf("updating agent object: %w", err)
	}

	return nil
}

func SwitchIP(ctx context.Context, name string) error {
	kube, err := kubeutil.NewClient(ctx, "", wiringapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	sw := &wiringapi.Switch{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, sw); err != nil {
		return fmt.Errorf("getting switch %q: %w", name, err)
	}

	if sw.Spec.IP == "" {
		return fmt.Errorf("switch %q has no management IP address", name) //nolint:goerr113
	}

	fmt.Println(sw.Spec.IP)

	return nil
}

func SwitchSSH(ctx context.Context, name, username, run string) error {
	if username == "" {
		return fmt.Errorf("username is required") //nolint:goerr113
	}

	kube, err := kubeutil.NewClient(ctx, "", wiringapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	sw := &wiringapi.Switch{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, sw); err != nil {
		return fmt.Errorf("getting switch %q: %w", name, err)
	}

	if sw.Spec.IP == "" {
		return fmt.Errorf("switch %q has no management IP address", name) //nolint:goerr113
	}

	ip, err := netip.ParsePrefix(sw.Spec.IP)
	if err != nil {
		return fmt.Errorf("parsing switch IP address: %w", err)
	}

	cmd := exec.CommandContext(ctx, "ssh", append(SSHQuietFlags, username+"@"+ip.Addr().String(), run)...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running ssh: %w", err)
	}

	return nil
}

func SwitchSerial(ctx context.Context, name string) error {
	kube, err := kubeutil.NewClient(ctx, "", wiringapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	sw := &wiringapi.Switch{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, sw); err != nil {
		return fmt.Errorf("getting switch %q: %w", name, err)
	}

	serial := GetSerialInfo(sw)
	if serial == "" {
		return fmt.Errorf("switch %q has no serial connection information", name) //nolint:goerr113
	}

	parts := strings.SplitN(serial, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid remote serial (expected host:port): %s", serial) //nolint:goerr113
	}

	cmd := exec.CommandContext(ctx, "ssh", append(SSHQuietFlags, "-p", parts[1], parts[0])...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running ssh for serial: %w", err)
	}

	return nil
}

func GetSerialInfo(sw *wiringapi.Switch) string {
	if sw.GetAnnotations() != nil {
		if v, exist := sw.GetAnnotations()[HHFabCfgSerial]; exist {
			if strings.HasPrefix(v, HHFabCfgSerialSchemeSSH) {
				return v[len(HHFabCfgSerialSchemeSSH):]
			}

			return ""
		}

		if v, exist := sw.GetAnnotations()[HHFctlCfgSerial]; exist {
			if strings.HasPrefix(v, HHFabCfgSerialSchemeSSH) {
				return v[len(HHFabCfgSerialSchemeSSH):]
			}

			return ""
		}
	}

	return ""
}

func GetPowerInfo(sw *wiringapi.Switch) map[string]string {
	powerInfo := make(map[string]string)
	if annotations := sw.GetAnnotations(); annotations != nil {
		for key, value := range annotations {
			if strings.HasPrefix(key, HHFabCfgPower+"/") {
				psuName := strings.TrimPrefix(key, HHFabCfgPower+"/")
				powerInfo[psuName] = value
			}
		}
	}

	return powerInfo
}

func SwitchRoCE(ctx context.Context, name string, value *bool) error {
	kube, err := kubeutil.NewClient(ctx, "", wiringapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	sw := &wiringapi.Switch{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, sw); err != nil {
		return fmt.Errorf("getting switch %q: %w", name, err)
	}

	if value == nil {
		sw.Spec.RoCE = !sw.Spec.RoCE
	} else {
		sw.Spec.RoCE = *value
	}

	slog.Info("Setting RoCE mode", "switch", name, "roce", sw.Spec.RoCE)

	err = kube.Update(ctx, sw)
	if err != nil {
		return fmt.Errorf("updating switch object: %w", err)
	}

	return nil
}

func SwitchECMPRoCEQPN(ctx context.Context, name string, value *bool) error {
	kube, err := kubeutil.NewClient(ctx, "", wiringapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	sw := &wiringapi.Switch{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, sw); err != nil {
		return fmt.Errorf("getting switch %q: %w", name, err)
	}

	if value == nil {
		sw.Spec.ECMP.RoCEQPN = !sw.Spec.ECMP.RoCEQPN
	} else {
		sw.Spec.ECMP.RoCEQPN = *value
	}

	slog.Info("Setting ECMP RoCE QPN", "switch", name, "qpn", sw.Spec.ECMP.RoCEQPN)

	err = kube.Update(ctx, sw)
	if err != nil {
		return fmt.Errorf("updating switch object: %w", err)
	}

	return nil
}
