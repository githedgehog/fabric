package common

import (
	"context"
	"crypto/tls"
	_ "embed"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func AgentUpgrade(ctx context.Context, currentVersion string, version agentapi.AgentVersion, dryRun bool, testArgs []string) (bool, error) {
	desiredVersion := ""
	if version.Default != "" {
		desiredVersion = version.Default
	}
	if version.Override != "" {
		desiredVersion = version.Override
	}

	if desiredVersion == "" || currentVersion == desiredVersion {
		return false, nil
	}

	slog.Info("Desired version is different from current", "desired", desiredVersion, "current", currentVersion)

	if dryRun {
		slog.Info("Dry run, not upgrading")
		return false, nil
	}

	slog.Info("Attempting to upgrade Agent")

	path, err := os.MkdirTemp("/opt/hedgehog/bin", "agent-upgrade-*")
	if err != nil {
		return false, errors.Wrap(err, "failed to create temp dir")
	}
	defer os.RemoveAll(path)

	fs, err := file.New(path)
	if err != nil {
		return false, errors.Wrapf(err, "error creating oras file store in %s", path)
	}
	defer fs.Close()

	repo, err := remote.NewRepository(version.Repo)
	if err != nil {
		return false, errors.Wrapf(err, "error creating oras remote repo %s", version.Repo)
	}

	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	// TODO load CA
	// config.RootCAs, err = crypto.LoadCertPool(opts.CACertFilePath)
	// 	if err != nil {
	// 		return nil, err
	// 	}

	repo.Client = &auth.Client{
		Client: &http.Client{
			Transport: baseTransport,
		},
	}

	_, err = oras.Copy(context.Background(), repo, desiredVersion, fs, desiredVersion, oras.CopyOptions{
		CopyGraphOptions: oras.CopyGraphOptions{
			Concurrency: 2,
		},
	})
	if err != nil {
		return false, errors.Wrapf(err, "error downloading new agent %s from %s", desiredVersion, version.Repo)
	}

	agentPath := filepath.Join(path, "agent")

	err = os.Chmod(agentPath, 0o755)
	if err != nil {
		return false, errors.Wrapf(err, "failed to chmod new agent binary in %s", path)
	}

	cmd := exec.CommandContext(ctx, agentPath, testArgs...)
	err = cmd.Run()
	if err != nil {
		return false, errors.Wrapf(err, "failed to run new agent binary in %s", path)
	}

	// TODO const?
	err = os.Rename(agentPath, "/opt/hedgehog/bin/agent")
	if err != nil {
		return false, errors.Wrapf(err, "failed to move new agent binary from %s to /opt/hedgehog/bin/agent", path)
	}

	return true, nil
}
