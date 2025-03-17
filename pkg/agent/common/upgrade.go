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

package common

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/logutil"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

var (
	ErrAgentUpgradeDownloadFailed = errors.New("agent upgrade download failed")
	ErrAgentUpgradeCheckFailed    = errors.New("agent upgrade check failed")
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

	return true, UpgradeBin(ctx, version.Repo, desiredVersion, version.CA, version.Username, version.Password, "/opt/hedgehog/bin", "agent", func(ctx context.Context, binPath string) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, binPath, testArgs...)
		cmd.Stdout = logutil.NewSink(ctx, slog.Info, "newagent: ")
		cmd.Stderr = logutil.NewSink(ctx, slog.Warn, "newagent: ")

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run new agent: %w", err)
		}

		return nil
	})
}

func UpgradeBin(ctx context.Context, source, version, ca, username, password, target, name string, testFunc func(ctx context.Context, binPath string) error) error {
	tmpPath, err := os.MkdirTemp(target, "bin-upgrade-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpPath)

	fs, err := file.New(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create oras file store: %w", err)
	}
	defer fs.Close()

	repo, err := remote.NewRepository(source)
	if err != nil {
		return fmt.Errorf("failed to create oras remote repo: %w", err)
	}

	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM([]byte(ca)) {
		return fmt.Errorf("failed to append CA cert to rootCAs") //nolint:goerr113
	}

	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		RootCAs:            rootCAs,
	}

	repo.Client = &auth.Client{
		Client: &http.Client{
			Transport: baseTransport,
		},
		Cache: auth.DefaultCache,
		Credential: func(_ context.Context, _ string) (auth.Credential, error) {
			return auth.Credential{
				Username: username,
				Password: password,
			}, nil
		},
	}

	_, err = oras.Copy(ctx, repo, version, fs, version, oras.CopyOptions{
		CopyGraphOptions: oras.CopyGraphOptions{
			Concurrency: 2,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to download new %s bin %s from %s: %w", name, version, source, errors.Join(ErrAgentUpgradeDownloadFailed, err))
	}

	binPath := filepath.Join(tmpPath, name)

	err = os.Chmod(binPath, 0o755)
	if err != nil {
		return fmt.Errorf("failed to chmod new %s bin in %s: %w", name, tmpPath, err)
	}

	if err := testFunc(ctx, binPath); err != nil {
		return fmt.Errorf("failed to test new %s bin in %s: %w", name, tmpPath, errors.Join(ErrAgentUpgradeCheckFailed, err))
	}

	targetPath := filepath.Join(target, name)
	err = os.Rename(binPath, targetPath)
	if err != nil {
		return fmt.Errorf("failed to move new %s bin to %s: %w", name, targetPath, err)
	}

	return nil
}
