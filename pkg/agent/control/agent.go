package control

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/common"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	KUBECONFIG_FILE = "/etc/rancher/k3s/k3s.yaml"
	NETWORK_FILES   = "/etc/systemd/network"
)

type Service struct {
	Version string

	DryRun    bool
	ApplyOnce bool
}

func (svc *Service) Run(ctx context.Context) error {
	hostname, err := os.Hostname()
	if err != nil {
		return errors.Wrap(err, "failed to get hostname")
	}

	slog.Info("Starting control agent", "hostname", hostname, "version", svc.Version)

	kube, err := svc.kubeClient()
	if err != nil {
		return errors.Wrap(err, "failed to create kube client")
	}

	agent := &agentapi.ControlAgent{}
	err = kube.Get(ctx, client.ObjectKey{Name: hostname, Namespace: "default"}, agent) // TODO ns
	if err != nil {
		return errors.Wrapf(err, "failed to get initial control agent config from k8s")
	}

	if svc.DryRun {
		slog.Info("Dry run, exiting")
		return nil
	}

	if svc.ApplyOnce {
		slog.Info("Applying config once")
		return errors.Wrapf(svc.process(ctx, agent), "failed to apply once")
	}

	currentGen := int64(0)

	// reset observability state
	now := metav1.Time{Time: time.Now()}
	agent.Status.LastHeartbeat = now
	agent.Status.LastAttemptTime = now
	agent.Status.LastAttemptGen = currentGen
	agent.Status.LastAppliedTime = now
	agent.Status.LastAppliedGen = currentGen
	agent.Status.Version = svc.Version
	if agent.Status.Conditions == nil {
		agent.Status.Conditions = []metav1.Condition{}
	}

	err = kube.Status().Update(ctx, agent) // TODO maybe use patch for such status updates?
	if err != nil {
		return errors.Wrapf(err, "failed to reset control agent observability status") // TODO gracefully handle case if resourceVersion changed
	}

	slog.Info("Starting watch for config changes in K8s")

	watcher, err := kube.Watch(context.TODO(), &agentapi.ControlAgentList{}, client.InNamespace("default"), client.MatchingFields{ // TODO ns
		"metadata.name": hostname,
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	for {
		// process currently loaded agent from K8s
		err = svc.processKubeUpdate(ctx, kube, agent, &currentGen)
		if err != nil {
			return errors.Wrap(err, "failed to process control agent config from k8s")
		}

		select {
		case <-ctx.Done():
			slog.Info("Context done, exiting")
			return nil
		case <-time.After(30 * time.Second):
			slog.Debug("Sending heartbeat")

			agent.Status.LastHeartbeat = metav1.Time{Time: time.Now()}

			err = kube.Status().Update(ctx, agent)
			if err != nil {
				return errors.Wrapf(err, "failed to update control agent heartbeat") // TODO gracefully handle case if resourceVersion changed
			}
		case event, ok := <-watcher.ResultChan():
			// TODO check why channel gets closed
			if !ok {
				slog.Warn("K8s watch channel closed, restarting control agent")
				os.Exit(1)
			}

			// TODO why are we getting nil events?
			if event.Object == nil {
				slog.Warn("Received nil object from K8s, restarting control agent")
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

			agent = event.Object.(*agentapi.ControlAgent)
		}
	}
}

func (svc *Service) processKubeUpdate(ctx context.Context, kube client.Client, agent *agentapi.ControlAgent, currentGen *int64) error {
	if agent.Generation == *currentGen {
		return nil
	}

	slog.Info("Control agent config changed", "current", *currentGen, "new", agent.Generation)

	if agent.Status.Conditions == nil {
		agent.Status.Conditions = []metav1.Condition{}
	}
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
		return errors.Wrapf(err, "error updating control agent last attempt") // TODO gracefully handle case if resourceVersion changed
	}

	err = svc.process(ctx, agent)
	if err != nil {
		return errors.Wrapf(err, "failed to process control agent config")
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

	err = kube.Status().Update(ctx, agent)
	if err != nil {
		return err // TODO gracefully handle case if resourceVersion changed
	}

	*currentGen = agent.Generation

	return nil
}

func (svc *Service) process(ctx context.Context, agent *agentapi.ControlAgent) error {
	slog.Info("Processing control agent config", "name", agent.Name, "gen", agent.Generation, "res", agent.ResourceVersion)

	upgraded, err := common.AgentUpgrade(ctx, svc.Version, agent.Spec.Version, false, []string{"control", "apply", "--dry-run=true"})
	if err != nil {
		slog.Warn("Failed to upgrade Agent", "err", err)
	} else if upgraded {
		slog.Info("Agent upgraded, restarting")
		os.Exit(0) // TODO graceful agent restart
	}

	slog.Debug("Recreating networkd config")
	files, err := filepath.Glob(filepath.Join(NETWORK_FILES, "00-hh-*"))
	if err != nil {
		return errors.Wrapf(err, "failed to list network files")
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			return errors.Wrapf(err, "failed to remove network file %s", f)
		}
	}
	for name, content := range agent.Spec.Networkd {
		err = os.WriteFile(filepath.Join(NETWORK_FILES, name), []byte(content), 0o644)
		if err != nil {
			return errors.Wrapf(err, "failed to write network file %s", name)
		}
	}

	slog.Debug("Reloading networkd")
	cmd := exec.CommandContext(ctx, "networkctl", "reload")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	err = cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to reload networkd")
	}

	hostsFile, err := os.Open("/etc/hosts")
	if err != nil {
		return errors.Wrapf(err, "failed to open /etc/hosts")
	}
	defer hostsFile.Close()

	hosts := bytes.NewBuffer(nil)
	scanner := bufio.NewScanner(hostsFile)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasSuffix(line, "# hedgehog") {
			continue
		}
		hosts.WriteString(line + "\n")
	}
	for hostname, ip := range agent.Spec.Hosts {
		hosts.WriteString(fmt.Sprintf("%s %s # hedgehog\n", ip, hostname))
	}

	err = os.WriteFile("/etc/hosts", hosts.Bytes(), 0o644)
	if err != nil {
		return errors.Wrapf(err, "failed to write /etc/hosts")
	}

	slog.Info("Config applied", "name", agent.Name, "gen", agent.Generation, "res", agent.ResourceVersion)

	return nil
}

func (svc *Service) kubeClient() (client.WithWatch, error) {
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: KUBECONFIG_FILE},
		nil,
	).ClientConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load kubeconfig from %s", KUBECONFIG_FILE)
	}

	scheme := runtime.NewScheme()
	err = agentapi.AddToScheme(scheme)
	if err != nil {
		return nil, errors.Wrap(err, "failed to add agent scheme")
	}

	kubeClient, err := client.NewWithWatch(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kube client")
	}

	return kubeClient, nil
}
