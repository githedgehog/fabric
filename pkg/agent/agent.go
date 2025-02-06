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

package agent

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/alloy"
	"go.githedgehog.com/fabric/pkg/agent/common"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	"go.githedgehog.com/fabric/pkg/util/uefiutil"
	"go.githedgehog.com/fabric/pkg/version"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	ConfigFile     = "agent-config.yaml"
	KubeconfigFile = "agent-kubeconfig"
)

//go:embed motd.txt
var motd []byte

type Service struct {
	Basedir string

	DryRun          bool
	SkipControlLink bool
	ApplyOnce       bool
	SkipActions     bool

	gnmiClient *gnmi.Client
	processor  dozer.Processor
	name       string
	installID  string
	runID      string

	reg *switchstate.Registry
}

const (
	exporterPort = 7042
)

func (svc *Service) Run(ctx context.Context, getClient func() (*gnmi.Client, error)) error {
	svc.reg = switchstate.NewRegistry()
	svc.reg.AgentMetrics.Version.WithLabelValues(version.Version).Set(1)

	if !svc.ApplyOnce && !svc.DryRun {
		go func() {
			if err := svc.reg.ServeMetrics(exporterPort); err != nil {
				slog.Error("Failed to serve metrics", "err", err)
				panic(err)
			}
		}()
	}

	if svc.Basedir == "" {
		return errors.New("basedir is required")
	}
	if version.Version == "" {
		return errors.New("version is required")
	}

	slog.Info("Starting", "version", version.Version, "basedir", svc.Basedir)

	agent, err := svc.loadConfigFromFile()
	if err != nil {
		return errors.Wrap(err, "failed to load config")
	}

	slog.Info("Config loaded from file", "name", agent.Name, "gen", agent.Generation, "res", agent.ResourceVersion)

	svc.gnmiClient, err = getClient()
	if err != nil {
		return errors.Wrap(err, "failed to create gNMI client")
	}
	defer svc.gnmiClient.Close()

	svc.processor = bcm.Processor(svc.gnmiClient)

	if !svc.DryRun && !svc.ApplyOnce {
		err = os.WriteFile("/etc/motd", motd, 0o644) //nolint:gosec
		if err != nil {
			slog.Warn("Failed to write motd", "err", err)
		}
	}

	err = svc.processAgent(ctx, agent, true)
	if err != nil {
		return errors.Wrap(err, "failed to process agent config from file")
	}

	if svc.DryRun {
		slog.Info("Dry run, exiting")

		return nil
	}

	if svc.ApplyOnce {
		slog.Info("Apply once, exiting")

		return nil
	}

	if err := svc.setInstallAndRunIDs(); err != nil {
		return errors.Wrap(err, "failed to set install and run IDs")
	}

	if err := svc.celesticaWorkaround(agent); err != nil {
		return errors.Wrap(err, "failed to apply Celestica workaround")
	}

	kubeconfigPath := filepath.Join(svc.Basedir, KubeconfigFile)
	kube, err := kubeutil.NewClient(ctx, kubeconfigPath, agentapi.SchemeBuilder)
	if err != nil {
		return errors.Wrapf(err, "failed to create K8s client")
	}

	currentGen := agent.Generation

	err = kube.Get(ctx, client.ObjectKey{Name: agent.Name, Namespace: metav1.NamespaceDefault}, agent)
	if err != nil {
		return errors.Wrapf(err, "failed to get initial agent config from k8s")
	}

	// reset observability state
	now := metav1.Time{Time: time.Now()}
	agent.Status.LastHeartbeat = now
	agent.Status.LastAttemptTime = now
	agent.Status.LastAttemptGen = currentGen
	agent.Status.LastAppliedTime = now
	agent.Status.LastAppliedGen = currentGen
	agent.Status.InstallID = svc.installID
	agent.Status.RunID = svc.runID
	agent.Status.Version = version.Version
	agent.Status.StatusUpdates = agent.Spec.StatusUpdates
	if agent.Status.Conditions == nil {
		agent.Status.Conditions = []metav1.Condition{}
	}

	if err := svc.processor.UpdateSwitchState(ctx, agent, svc.reg); err != nil {
		return errors.Wrapf(err, "failed to update switch state")
	}
	if st := svc.reg.GetSwitchState(); st != nil {
		agent.Status.State = *st
	}
	agent.Status.LastHeartbeat = metav1.Time{Time: time.Now()}

	err = kube.Status().Update(ctx, agent) // TODO maybe use patch for such status updates?
	if err != nil {
		return errors.Wrapf(err, "failed to reset agent observability status") // TODO gracefully handle case if resourceVersion changed
	}

	slog.Debug("Starting watch for config changes in K8s")

	watcher, err := kube.Watch(ctx, &agentapi.AgentList{}, client.InNamespace(metav1.NamespaceDefault), client.MatchingFields{
		"metadata.name": svc.name,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to watch agent config in k8s")
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Context done, exiting")

			return nil
		case <-time.After(2 * time.Minute):
			slog.Debug("Enforcing config", "name", agent.Name)
			err = svc.processAgentFromKube(ctx, kube, agent, &currentGen, true)
			if err != nil {
				return errors.Wrap(err, "failed to process agent config from k8s (enforce)")
			}
		case <-time.After(15 * time.Second):
			slog.Debug("Sending heartbeat", "name", agent.Name)
			hbStart := time.Now()

			if err := svc.processor.UpdateSwitchState(ctx, agent, svc.reg); err != nil {
				return errors.Wrapf(err, "failed to update switch state")
			}
			if st := svc.reg.GetSwitchState(); st != nil {
				agent.Status.State = *st
			}
			agent.Status.LastHeartbeat = metav1.Time{Time: time.Now()}

			err := kube.Status().Update(ctx, agent)
			if err != nil {
				return errors.Wrapf(err, "failed to update agent heartbeat") // TODO gracefully handle case if resourceVersion changed
			}

			svc.reg.AgentMetrics.HeartbeatDuration.Observe(time.Since(hbStart).Seconds())
			svc.reg.AgentMetrics.HeartbeatsTotal.Inc()
		case event, ok := <-watcher.ResultChan():
			// TODO check why channel gets closed
			if !ok {
				slog.Warn("K8s watch channel closed, restarting agent")
				os.Exit(1)
			}

			// TODO why are we getting nil events?
			if event.Object == nil {
				slog.Warn("Received nil object from K8s, restarting agent")
				os.Exit(1)
			}

			// TODO handle bookmarks and delete events
			if event.Type == watch.Deleted || event.Type == watch.Bookmark {
				slog.Info("Received watch event, ignoring", "event", event.Type)

				continue
			}
			if event.Type == watch.Error {
				slog.Error("Received watch error", "event", event.Type, "object", event.Object)
				if err, ok := event.Object.(error); ok {
					return errors.Wrapf(err, "watch error")
				}

				return errors.New("watch error")
			}

			agent = event.Object.(*agentapi.Agent)

			err = svc.processAgentFromKube(ctx, kube, agent, &currentGen, false)
			if err != nil {
				return errors.Wrap(err, "failed to process agent config from k8s")
			}
		}
	}
}

func (svc *Service) celesticaWorkaround(agent *agentapi.Agent) error {
	// TODO remove this after we know the root cause for Celestica switches having non-working ports after power-cycling
	if agent.Spec.SwitchProfile != nil && strings.HasPrefix(agent.Spec.SwitchProfile.Platform, "x86_64-cel_") {
		marker := filepath.Join(svc.Basedir, ".celestica-fix")
		if _, err := os.Stat(marker); err != nil {
			if os.IsNotExist(err) {
				slog.Warn("First boot with Celestica switch profile, rebooting in 5 seconds")

				if err = os.WriteFile(marker, []byte{}, 0o644); err != nil { //nolint:gosec
					return errors.Wrapf(err, "failed to create celestica marker file %q", marker)
				}

				time.Sleep(5 * time.Second)

				file, err := os.OpenFile("/proc/sysrq-trigger", os.O_WRONLY, 0o200)
				if err != nil {
					return errors.Wrapf(err, "error opening /proc/sysrq-trigger")
				}
				defer file.Close()

				// If we're failed to reboot, we'll try again on the next agent start
				defer func() {
					if err := os.Remove(marker); err != nil {
						slog.Warn("Failed to remove celestica marker file", "err", err)
					}
				}()

				if _, err := file.WriteString("b"); err != nil {
					return errors.Wrapf(err, "error writing to /proc/sysrq-trigger")
				}

				time.Sleep(5 * time.Minute)

				return errors.New("failed to reboot to apply Celestica workaround")
			}

			return errors.Wrapf(err, "failed to check celestica marker file %q", marker)
		}
	}

	return nil
}

func (svc *Service) setInstallAndRunIDs() error {
	svc.runID = uuid.New().String()

	installIDFile := filepath.Join(svc.Basedir, "install-id")
	installID, err := os.ReadFile(installIDFile)
	if os.IsNotExist(err) {
		newInstallID := uuid.New().String()
		err = os.WriteFile(installIDFile, []byte(newInstallID), 0o644) //nolint:gosec
		if err != nil {
			return errors.Wrapf(err, "failed to write install ID file %q", installIDFile)
		}
		svc.installID = newInstallID
	} else if err != nil {
		return errors.Wrapf(err, "failed to read install ID file %q", installIDFile)
	} else {
		svc.installID = string(installID)
	}

	slog.Info("IDs ready", "install", svc.installID, "run", svc.runID)

	return nil
}

func (svc *Service) processAgent(ctx context.Context, agent *agentapi.Agent, readyCheck bool) error {
	start := time.Now()
	slog.Info("Processing agent config", "name", agent.Name, "gen", agent.Generation, "res", agent.ResourceVersion)

	svc.reg.AgentMetrics.Generation.Set(float64(agent.Generation))

	if !svc.SkipControlLink {
		if err := svc.processor.EnsureControlLink(ctx, agent); err != nil {
			return errors.Wrap(err, "failed to ensure control link")
		}
		slog.Info("Control link configuration applied")
	} else {
		slog.Info("Control link configuration is skipped")
	}

	if readyCheck {
		if err := svc.processor.WaitReady(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for system status ready")
		}
	}

	desired, err := svc.processor.PlanDesiredState(ctx, agent)
	if err != nil {
		return errors.Wrapf(err, "failed to plan spec")
	}
	slog.Debug("Desired state generated")

	startActual := time.Now()
	actual, err := svc.processor.LoadActualState(ctx, agent)
	if err != nil {
		return errors.Wrapf(err, "failed to load actual state")
	}
	slog.Debug("Actual state loaded", "took", time.Since(startActual))

	actions, err := svc.processor.CalculateActions(ctx, actual, desired)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate spec")
	}
	slog.Debug("Actions calculated", "count", len(actions))

	actual.CleanupSensetive()
	desired.CleanupSensetive()

	desiredData, err := desired.MarshalYAML()
	if err != nil {
		return errors.Wrapf(err, "failed to marshal desired spec")
	}

	err = os.WriteFile(filepath.Join(svc.Basedir, "last-desired.yaml"), desiredData, 0o644) //nolint:gosec
	if err != nil {
		return errors.Wrapf(err, "failed to write desired spec")
	}

	actualData, err := actual.MarshalYAML()
	if err != nil {
		return errors.Wrapf(err, "failed to marshal actual spec")
	}

	err = os.WriteFile(filepath.Join(svc.Basedir, "last-actual.yaml"), actualData, 0o644) //nolint:gosec
	if err != nil {
		return errors.Wrapf(err, "failed to write actual spec")
	}

	diff, err := dozer.SpecTextDiff(actualData, desiredData)
	if err != nil {
		return errors.Wrapf(err, "failed to generate diff")
	}

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		// TODO skip if diff is empty
		for _, line := range strings.SplitAfter(string(diff), "\n") {
			line = strings.TrimRight(line, "\n")
			if strings.ReplaceAll(line, " ", "") == "" {
				continue
			}
			slog.Debug("Actual <> Desired", "diff", line)
		}
	}

	if svc.DryRun {
		slog.Warn("Dry run, exiting")

		return nil
	}

	slog.Info("Applying actions", "count", len(actions))

	warnings, err := svc.processor.ApplyActions(ctx, actions)
	if err != nil {
		return errors.Wrapf(err, "failed to apply actions")
	}
	for _, warning := range warnings {
		slog.Warn("Action warning: " + warning)
	}

	slog.Info("Config applied", "name", agent.Name, "gen", agent.Generation, "res", agent.ResourceVersion, "took", time.Since(start))

	svc.reg.AgentMetrics.ConfigApplyDuration.Observe(time.Since(start).Seconds())

	return errors.Wrapf(alloy.EnsureInstalled(ctx, agent, exporterPort), "failed to ensure alloy installed")
}

func (svc *Service) processAgentFromKube(ctx context.Context, kube client.Client, agent *agentapi.Agent, currentGen *int64, enforce bool) error {
	svc.reg.AgentMetrics.Generation.Set(float64(agent.Generation))

	if !enforce && agent.Generation == *currentGen {
		return nil
	}

	start := time.Now()

	if agent.Generation != *currentGen {
		slog.Info("Agent config changed", "current", *currentGen, "new", agent.Generation)
	} else if enforce {
		slog.Info("Enforcing agent config", "gen", agent.Generation)
	}

	if agent.Status.Conditions == nil {
		agent.Status.Conditions = []metav1.Condition{}
	}

	if !enforce {
		// TODO better handle status condtions
		apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               "Applied",
			Status:             metav1.ConditionFalse,
			Reason:             "ApplyPending",
			LastTransitionTime: metav1.Time{Time: time.Now()},
			Message:            fmt.Sprintf("Config will be applied, gen=%d", agent.Generation),
		})

		// demonstrating that we're going to try to apply config
		agent.Status.LastAttemptGen = agent.Generation
		agent.Status.LastAttemptTime = metav1.Time{Time: time.Now()}

		err := kube.Status().Update(ctx, agent)
		if err != nil {
			return errors.Wrapf(err, "error updating agent last attempt") // TODO gracefully handle case if resourceVersion changed
		}
	}

	err := svc.processActions(ctx, agent)
	if err != nil {
		return errors.Wrap(err, "failed to process agent actions from k8s")
	}

	err = svc.processAgent(ctx, agent, false)
	if err != nil {
		return errors.Wrap(err, "failed to process agent config loaded from k8s")
	}

	err = svc.saveConfigToFile(agent)
	if err != nil {
		return errors.Wrap(err, "failed to save agent config to file")
	}

	// report that we've been able to apply config
	agent.Status.LastAppliedGen = agent.Generation
	agent.Status.LastAppliedTime = metav1.Time{Time: time.Now()}

	// TODO not the best way to use conditions, but it's the easiest way to then wait for agents
	apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
		Type:               "Applied",
		Status:             metav1.ConditionTrue,
		Reason:             "ApplySucceeded",
		LastTransitionTime: metav1.Time{Time: time.Now()},
		Message:            fmt.Sprintf("Config applied, gen=%d", agent.Generation),
	})

	svc.reg.AgentMetrics.KubeApplyDuration.Observe(time.Since(start).Seconds())

	if err := svc.processor.UpdateSwitchState(ctx, agent, svc.reg); err != nil {
		return errors.Wrapf(err, "failed to update switch state")
	}
	if st := svc.reg.GetSwitchState(); st != nil {
		agent.Status.State = *st
	}
	agent.Status.LastHeartbeat = metav1.Time{Time: time.Now()}

	err = kube.Status().Update(ctx, agent)
	if err != nil {
		return errors.Wrapf(err, "failed to update status") // TODO gracefully handle case if resourceVersion changed
	}

	*currentGen = agent.Generation

	return nil
}

func (svc *Service) processActions(ctx context.Context, agent *agentapi.Agent) error {
	if agent.Spec.PowerReset != "" && agent.Spec.PowerReset == svc.runID {
		slog.Info("Power reset requested, executing in 5 seconds", "runID", agent.Spec.PowerReset)
		time.Sleep(5 * time.Second)
		if !svc.SkipActions {
			slog.Info("Power resetting")

			file, err := os.OpenFile("/proc/sysrq-trigger", os.O_WRONLY, 0o200)
			if err != nil {
				return errors.Wrapf(err, "error opening /proc/sysrq-trigger")
			}
			defer file.Close()

			if _, err := file.WriteString("b"); err != nil {
				if !os.IsExist(err) {
					return errors.Wrapf(err, "error writing to /proc/sysrq-trigger")
				}
			}
		}
	}

	reboot := false
	if agent.Spec.Reinstall != "" && agent.Spec.Reinstall == svc.installID {
		slog.Info("Reinstall requested", "installID", agent.Spec.Reinstall)
		if !svc.SkipActions {
			err := uefiutil.MakeONIEDefaultBootEntryAndCleanup()
			if err != nil {
				slog.Warn("Failed to make ONIE default boot entry", "err", err)
			} else {
				slog.Info("Rebooting into ONIE")
				reboot = true
			}
		}
	}

	if agent.Spec.Reboot != "" && agent.Spec.Reboot == svc.runID {
		slog.Info("Reboot requested", "runID", agent.Spec.Reboot)
		if !svc.SkipActions {
			slog.Info("Rebooting")
			reboot = true
		}
	}

	if reboot {
		cmd := exec.CommandContext(ctx, "wall", "Hedgehog Agent initiated reboot")
		err := cmd.Run()
		if err != nil {
			slog.Warn("Failed to send wall message", "err", err)
		}

		cmd = exec.CommandContext(ctx, "reboot")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			return errors.Wrap(err, "failed to reboot")
		}
	}

	upgraded, err := common.AgentUpgrade(ctx, version.Version, agent.Spec.Version, svc.SkipActions, []string{"apply", "--dry-run=true"})
	if err != nil {
		slog.Warn("Failed to upgrade Agent", "err", err)
	} else if upgraded {
		slog.Info("Agent upgraded, restarting")
		os.Exit(0) // TODO graceful agent restart
	}

	return nil
}

func (svc *Service) configFilePath() string {
	return filepath.Join(svc.Basedir, ConfigFile)
}

func (svc *Service) loadConfigFromFile() (*agentapi.Agent, error) {
	data, err := os.ReadFile(svc.configFilePath())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file %s", svc.configFilePath())
	}

	config := &agentapi.Agent{}
	err = yaml.UnmarshalStrict(data, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal config file %s", svc.configFilePath())
	}
	svc.name = config.Name

	return config, nil
}

func (svc *Service) saveConfigToFile(agent *agentapi.Agent) error {
	if agent == nil {
		return errors.New("no config to save")
	}

	data, err := yaml.Marshal(agent)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal config")
	}

	err = os.WriteFile(svc.configFilePath(), data, 0o644) //nolint:gosec
	if err != nil {
		return errors.Wrapf(err, "failed to write config file %s", svc.configFilePath())
	}

	return nil
}
