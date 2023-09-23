package agent

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/gnmi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	CONF_FILE       = "agent-config.yaml"
	KUBECONFIG_FILE = "agent-kubeconfig"
)

type Service struct {
	Basedir         string
	DryRun          bool
	SkipControlLink bool
	ApplyOnce       bool

	client *gnmi.Client
	name   string
}

func (svc *Service) Run(ctx context.Context, getClient func() (*gnmi.Client, error)) error {
	agent, err := svc.loadConfigFromFile()
	if err != nil {
		return errors.Wrap(err, "failed to load config")
	}

	svc.client, err = getClient()
	if err != nil {
		return errors.Wrap(err, "failed to create gNMI client")
	}
	defer svc.client.Close()

	err = svc.processAgent(ctx, agent)
	if err != nil {
		return errors.Wrap(err, "failed to process agent config")
	}

	if !svc.ApplyOnce {
		slog.Info("Starting watch for config changes in K8s")

		kube, err := svc.kubeClient()
		if err != nil {
			return err
		}

		watcher, err := kube.Watch(context.TODO(), &agentapi.AgentList{}, client.InNamespace("default"), client.MatchingFields{ // TODO ns
			"metadata.name": svc.name,
		})
		if err != nil {
			return err
		}

		// TODO send regular heartbeats to K8s

		currentGen := int64(-1)
		for event := range watcher.ResultChan() {
			agent, ok := event.Object.(*agentapi.Agent)
			if !ok {
				return errors.New("can't cast to agent")
			}

			if agent.Generation == currentGen {
				slog.Debug("Skipping agent with same generation", "name", agent.Name, "generation", agent.Generation)
				continue
			}

			slog.Info("Agent config changed", "event", event.Type)
			spew.Dump(agent)

			nosInfo, err := svc.client.GetNOSInfo(ctx)
			if err != nil {
				return err
			}
			agent.Status.NOSInfo = *nosInfo

			// demonstrating that we're going to try to apply config
			agent.Status.LastAttemptGen = agent.Generation
			agent.Status.LastAttemptTime = metav1.Time{Time: time.Now()}

			err = kube.Status().Update(context.TODO(), agent)
			if err != nil {
				return err
			}

			// TODO add lastApplied (time), lastAppliedGeneration
			// TODO apply and save config

			err = svc.processAgent(ctx, agent)
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

			err = kube.Status().Update(context.TODO(), agent)
			if err != nil {
				return err
			}

			currentGen = agent.Generation
		}
	}

	return nil
}

func (svc *Service) processAgent(ctx context.Context, agent *agentapi.Agent) error {
	slog.Info("Processing agent config", "name", agent.Name, "gen", agent.Generation, "res", agent.ResourceVersion)

	plan, err := PreparePlan(agent)
	if err != nil {
		return errors.Wrap(err, "failed to process config to prepare plan")
	}

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.Debug("Plan:")
		spew.Dump(plan)
	}

	if svc.DryRun {
		slog.Warn("Dry run, exiting")
		return nil
	}

	if !svc.SkipControlLink {
		if err := svc.ensureControlLink(agent); err != nil {
			return errors.Wrap(err, "failed to ensure control link")
		}
		slog.Info("Control link configuration applied")
	} else {
		slog.Info("Control link configuration is skipped")
	}

	_, err = plan.Entries()
	if err != nil {
		return errors.Wrap(err, "failed to generate plan entries")
	}
	slog.Info("Plan entries generated")

	// TODO
	// if slog.Default().Enabled(ctx, slog.LevelDebug) {
	// 	slog.Debug("Plan entries:")
	// 	spew.Dump(entries)
	// }

	err = plan.ApplyWith(ctx, svc.client)
	if err != nil {
		return errors.Wrap(err, "failed to apply config")
	}
	slog.Info("Config applied", "name", agent.Name, "gen", agent.Generation, "res", agent.ResourceVersion)

	return nil
}

func (svc *Service) kubeClient() (client.WithWatch, error) {
	kubeconfigPath := filepath.Join(svc.Basedir, KUBECONFIG_FILE)
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		nil,
	).ClientConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load kubeconfig from %s", kubeconfigPath)
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

func (s *Service) configFilePath() string {
	return filepath.Join(s.Basedir, CONF_FILE)
}

func (s *Service) loadConfigFromFile() (*agentapi.Agent, error) {
	data, err := os.ReadFile(s.configFilePath())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file %s", s.configFilePath())
	}

	config := &agentapi.Agent{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal config file %s", s.configFilePath())
	}
	s.name = config.Name

	return config, nil
}

func (s *Service) saveConfigToFile(agent *agentapi.Agent) error {
	if agent == nil {
		return errors.New("no config to save")
	}

	data, err := yaml.Marshal(agent)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal config")
	}

	err = os.WriteFile(s.configFilePath(), data, 0o640)
	if err != nil {
		return errors.Wrapf(err, "failed to write config file %s", s.configFilePath())
	}

	return nil
}
