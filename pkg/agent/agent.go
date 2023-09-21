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
	"sigs.k8s.io/yaml"
)

const (
	CONF_FILE = "agent-config.yaml"
)

type Service struct {
	Basedir         string
	DryRun          bool
	SkipControlLink bool
	ApplyOnce       bool

	client *gnmi.Client
}

func (svc *Service) Run(ctx context.Context, getClient func() (*gnmi.Client, error)) error {
	agent, err := svc.loadConfigFromFile()
	if err != nil {
		return errors.Wrap(err, "failed to load config")
	}

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

	// if slog.Default().Enabled(ctx, slog.LevelDebug) {
	// 	slog.Debug("Plan entries:")
	// 	spew.Dump(entries)
	// }

	svc.client, err = getClient()
	if err != nil {
		return errors.Wrap(err, "failed to create gNMI client")
	}
	defer svc.client.Close()

	err = plan.ApplyWith(ctx, svc.client)
	if err != nil {
		return errors.Wrap(err, "failed to apply config")
	}
	slog.Info("Config applied from file")

	if !svc.ApplyOnce {
		// TODO watch for changes in K8s and apply new configs in the loop
		// TODO report status & heartbeat periodically
		slog.Warn("Watching for changes is not implemented yet, just sleeping")
		time.Sleep(100500 * time.Hour)
	}

	return nil
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
