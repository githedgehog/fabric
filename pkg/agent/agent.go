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
	"go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	"go.githedgehog.com/fabric/pkg/util/logutil"
	"go.githedgehog.com/fabric/pkg/util/uefiutil"
	"go.githedgehog.com/fabric/pkg/version"
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kyaml "sigs.k8s.io/yaml"
)

const (
	ConfigFile      = "agent-config.yaml"
	KubeconfigFile  = "agent-kubeconfig"
	HeartbeatPeriod = 15 * time.Second
	EnforcePeriod   = 2 * time.Minute
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
	bootID     string

	reg *switchstate.Registry

	lastHeartbeat time.Time
	lastApplied   time.Time
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

	retriesStart := time.Now()
	for time.Since(retriesStart) < 10*time.Minute {
		svc.gnmiClient, err = getClient()
		if err != nil {
			slog.Warn("Failed to create gNMI client", "err", err)
			time.Sleep(15 * time.Second)

			continue
		}

		break
	}
	if err != nil {
		return errors.Wrap(err, "failed to create gNMI client after retries")
	}
	defer svc.gnmiClient.Close()

	svc.processor, err = bcm.Processor(svc.gnmiClient)
	if err != nil {
		return errors.Wrap(err, "failed to create BCM processor")
	}

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

	kubeconfigPath := filepath.Join(svc.Basedir, KubeconfigFile)
	kube, err := kubeutil.NewClient(ctx, kubeconfigPath, agentapi.SchemeBuilder)
	if err != nil {
		return errors.Wrapf(err, "failed to create K8s client")
	}

	currentGen := agent.Generation

	err = kube.Get(ctx, kclient.ObjectKey{Name: agent.Name, Namespace: kmetav1.NamespaceDefault}, agent)
	if err != nil {
		return errors.Wrapf(err, "failed to get initial agent config from k8s")
	}

	// reset observability state
	now := kmetav1.Time{Time: time.Now()}
	agent.Status.LastAttemptTime = now
	agent.Status.LastAttemptGen = currentGen
	agent.Status.LastAppliedTime = now
	svc.lastApplied = agent.Status.LastAppliedTime.Time
	agent.Status.LastAppliedGen = currentGen
	agent.Status.InstallID = svc.installID
	agent.Status.RunID = svc.runID
	agent.Status.BootID = svc.bootID
	agent.Status.Version = version.Version
	agent.Status.StatusUpdates = agent.Spec.StatusUpdates
	if agent.Status.Conditions == nil {
		agent.Status.Conditions = []kmetav1.Condition{}
	}

	if err := svc.processor.UpdateSwitchState(ctx, agent, svc.reg); err != nil {
		return errors.Wrapf(err, "failed to update switch state")
	}
	if st := svc.reg.GetSwitchState(); st != nil {
		agent.Status.State = *st
	}

	agent.Status.LastHeartbeat = kmetav1.Time{Time: time.Now()}
	svc.lastHeartbeat = agent.Status.LastHeartbeat.Time

	err = kube.Status().Update(ctx, agent) // TODO maybe use patch for such status updates?
	if err != nil {
		return errors.Wrapf(err, "failed to reset agent observability status") // TODO gracefully handle case if resourceVersion changed
	}

	slog.Debug("Starting watch for config changes in K8s")

	watcher, err := kube.Watch(ctx, &agentapi.AgentList{}, kclient.InNamespace(kmetav1.NamespaceDefault), kclient.MatchingFields{
		"metadata.name": svc.name,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to watch agent config in k8s")
	}
	defer watcher.Stop()

	enforceTicker := time.NewTicker(EnforcePeriod)
	defer enforceTicker.Stop()

	heartbeatTicker := time.NewTicker(HeartbeatPeriod)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Context done, exiting")

			return nil
		case <-enforceTicker.C:
			if time.Since(svc.lastApplied) < EnforcePeriod/2 {
				slog.Debug("Skipping config enforcement, already applied recently", "name", agent.Name)

				continue
			}

			if err := svc.processAgent(ctx, agent, false); err != nil {
				return errors.Wrap(err, "failed to process agent config (enforce)")
			}

			svc.lastApplied = time.Now()
		case <-heartbeatTicker.C:
			if time.Since(svc.lastHeartbeat) < HeartbeatPeriod/2 {
				slog.Debug("Skipping heartbeat, already sent recently", "name", agent.Name)

				continue
			}

			slog.Debug("Sending heartbeat", "name", agent.Name)
			hbStart := time.Now()

			if err := svc.processor.UpdateSwitchState(ctx, agent, svc.reg); err != nil {
				return errors.Wrapf(err, "failed to update switch state")
			}
			if st := svc.reg.GetSwitchState(); st != nil {
				agent.Status.State = *st
			}

			agent.Status.LastHeartbeat = kmetav1.Time{Time: time.Now()}
			svc.lastHeartbeat = agent.Status.LastHeartbeat.Time

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

				return errors.New("k8s watch channel closed")
			}

			// TODO why are we getting nil events?
			if event.Object == nil {
				slog.Warn("Received nil object from K8s, restarting agent")

				return errors.New("k8s watch event object is nil")
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

			err = svc.processAgentFromKube(ctx, kube, agent, &currentGen)
			if err != nil {
				return errors.Wrap(err, "failed to process agent config from k8s")
			}
		}
	}
}

func (svc *Service) setInstallAndRunIDs() error {
	svc.runID = uuid.New().String()

	installIDFile := filepath.Join(svc.Basedir, "install-id")
	installID, err := os.ReadFile(installIDFile)
	if os.IsNotExist(err) { //nolint:gocritic
		newInstallID := uuid.New().String()
		err = os.WriteFile(installIDFile, []byte(newInstallID), 0o644) //nolint:gosec
		if err != nil {
			return errors.Wrapf(err, "failed to write install ID file %q", installIDFile)
		}
		svc.installID = newInstallID
	} else if err != nil {
		return errors.Wrapf(err, "failed to read install ID file %q", installIDFile)
	} else {
		svc.installID = strings.TrimSpace(string(installID))
	}

	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return errors.Wrapf(err, "failed to read boot ID")
	}
	svc.bootID = strings.TrimSpace(string(bootID))

	slog.Info("IDs", "install", svc.installID, "boot", svc.bootID, "run", svc.runID)

	return nil
}

func (svc *Service) processAgent(ctx context.Context, agent *agentapi.Agent, readyCheck bool) error {
	start := time.Now()
	slog.Info("Processing agent config", "name", agent.Name, "gen", agent.Generation, "res", agent.ResourceVersion)

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

	svc.reg.AgentMetrics.Generation.Set(float64(agent.Generation))
	svc.reg.AgentMetrics.ConfigApplyDuration.Observe(time.Since(start).Seconds())

	return errors.Wrapf(alloy.EnsureInstalled(ctx, agent, exporterPort), "failed to ensure alloy installed")
}

func (svc *Service) processAgentFromKube(ctx context.Context, kube kclient.Client, agent *agentapi.Agent, currentGen *int64) error {
	if agent.Generation == *currentGen {
		return nil
	}

	start := time.Now()

	slog.Info("Agent config changed", "current", *currentGen, "new", agent.Generation)

	if agent.Status.Conditions == nil {
		agent.Status.Conditions = []kmetav1.Condition{}
	}

	// TODO better handle status condtions
	kmeta.SetStatusCondition(&agent.Status.Conditions, kmetav1.Condition{
		Type:               "Applied",
		Status:             kmetav1.ConditionFalse,
		Reason:             "ApplyPending",
		LastTransitionTime: kmetav1.Time{Time: time.Now()},
		Message:            fmt.Sprintf("Config will be applied, gen=%d", agent.Generation),
	})

	// demonstrating that we're going to try to apply config
	agent.Status.LastAttemptGen = agent.Generation
	agent.Status.LastAttemptTime = kmetav1.Time{Time: time.Now()}

	if err := kube.Status().Update(ctx, agent); err != nil {
		return errors.Wrapf(err, "error updating agent last attempt") // TODO gracefully handle case if resourceVersion changed
	}

	if err := svc.processActions(ctx, agent); err != nil {
		return errors.Wrap(err, "failed to process agent actions from k8s")
	}

	if err := svc.processAgent(ctx, agent, false); err != nil {
		return errors.Wrap(err, "failed to process agent config loaded from k8s")
	}

	if err := svc.saveConfigToFile(agent); err != nil {
		return errors.Wrap(err, "failed to save agent config to file")
	}

	// report that we've been able to apply config
	agent.Status.LastAppliedGen = agent.Generation
	agent.Status.LastAppliedTime = kmetav1.Time{Time: time.Now()}
	svc.lastApplied = agent.Status.LastAppliedTime.Time

	// TODO not the best way to use conditions, but it's the easiest way to then wait for agents
	kmeta.SetStatusCondition(&agent.Status.Conditions, kmetav1.Condition{
		Type:               "Applied",
		Status:             kmetav1.ConditionTrue,
		Reason:             "ApplySucceeded",
		LastTransitionTime: kmetav1.Time{Time: time.Now()},
		Message:            fmt.Sprintf("Config applied, gen=%d", agent.Generation),
	})

	svc.reg.AgentMetrics.KubeApplyDuration.Observe(time.Since(start).Seconds())

	if err := svc.processor.UpdateSwitchState(ctx, agent, svc.reg); err != nil {
		return errors.Wrapf(err, "failed to update switch state")
	}
	if st := svc.reg.GetSwitchState(); st != nil {
		agent.Status.State = *st
	}

	agent.Status.LastHeartbeat = kmetav1.Time{Time: time.Now()}
	svc.lastHeartbeat = agent.Status.LastHeartbeat.Time

	if err := kube.Status().Update(ctx, agent); err != nil {
		return errors.Wrapf(err, "failed to update status") // TODO gracefully handle case if resourceVersion changed
	}

	*currentGen = agent.Generation

	return nil
}

func (svc *Service) processActions(ctx context.Context, agent *agentapi.Agent) error {
	if agent.Spec.PowerReset != "" && agent.Spec.PowerReset == svc.bootID {
		slog.Info("Power reset requested, executing in 5 seconds", "bootID", agent.Spec.PowerReset)
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
			slog.Info("Making ONIE next boot entry")

			if err := uefiutil.MakeONIEDefaultBootEntryAndCleanup(); err != nil {
				return fmt.Errorf("failed to make ONIE default boot entry: %w", err)
			}

			slog.Info("Make ONIE grub NOS Install entry default")

			if err := svc.setONIENOSInstall(ctx); err != nil {
				return fmt.Errorf("failed to set ONIE to install: %w", err)
			}

			reboot = true
		}
	}

	if agent.Spec.Reboot != "" && agent.Spec.Reboot == svc.bootID {
		slog.Info("Reboot requested", "bootID", agent.Spec.Reboot)
		if !svc.SkipActions {
			reboot = true
		}
	}

	if reboot {
		slog.Info("Rebooting in 5 seconds")
		time.Sleep(5 * time.Second)

		cmd := exec.CommandContext(ctx, "wall", "Hedgehog Agent initiated reboot")
		err := cmd.Run()
		if err != nil {
			slog.Warn("Failed to send wall message", "err", err)
		}

		if err := doRebootNow(ctx); err != nil {
			return err
		}
	}

	upgraded, err := common.AgentUpgrade(ctx, version.Version, agent.Spec.Version, svc.SkipActions, []string{"apply", "--dry-run=true"})
	if err != nil {
		if errors.Is(err, common.ErrAgentUpgradeDownloadFailed) { //nolint:gocritic
			// TODO properly retry it without restarting the agent
			slog.Warn("Failed to download new agent version, restarting agent to retry", "err", err)

			return errors.New("failed to download new agent version")
		} else if errors.Is(err, common.ErrAgentUpgradeCheckFailed) {
			// TODO properly report it in the Agent object status
			slog.Warn("Failed to check new agent version, skipping upgrade", "err", err)

			// not failing here, as new agent seems to be not working
		} else {
			slog.Warn("Failed to upgrade agent", "err", err)

			return errors.Wrap(err, "failed to upgrade agent")
		}
	} else if upgraded {
		slog.Info("Agent upgraded, restarting")

		os.Exit(0) //nolint:gocritic // TODO graceful agent restart
	}

	return nil
}

func doRebootNow(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, "reboot")
	cmd.Stdout = logutil.NewSink(ctx, slog.Debug, "reboot: ")
	cmd.Stderr = logutil.NewSink(ctx, slog.Debug, "reboot: ")

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to reboot")
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
	err = kyaml.UnmarshalStrict(data, config)
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

	data, err := kyaml.Marshal(agent)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal config")
	}

	err = os.WriteFile(svc.configFilePath(), data, 0o644) //nolint:gosec
	if err != nil {
		return errors.Wrapf(err, "failed to write config file %s", svc.configFilePath())
	}

	return nil
}

func (svc *Service) setONIENOSInstall(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if err := nosinstall.EnsureONIEBootPartition(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure ONIE boot partition")
	}

	cmd := exec.CommandContext(ctx, "/mnt/onie-boot/onie/tools/bin/onie-boot-mode", "-o", "install")
	cmd.Stdout = logutil.NewSink(ctx, slog.Debug, "onie-boot-mode: ")
	cmd.Stderr = logutil.NewSink(ctx, slog.Debug, "onie-boot-mode: ")

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to run onie-boot-mode set install")
	}

	cmd = exec.CommandContext(ctx, "/mnt/onie-boot/onie/tools/bin/onie-boot-mode", "-l")
	cmd.Stdout = logutil.NewSink(ctx, slog.Debug, "onie-boot-mode: ")
	cmd.Stderr = logutil.NewSink(ctx, slog.Debug, "onie-boot-mode: ")

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to run onie-boot-mode list")
	}

	return nil
}
