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
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/logutil"
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

	return true, UpgradeBin(ctx, version.Repo, desiredVersion, version.CA, version.Username, version.Password, "/opt/hedgehog/bin", "agent", func(ctx context.Context, binPath string) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, binPath, testArgs...)
		cmd.Stdout = logutil.NewSink(ctx, slog.Info, "newagent: ")
		cmd.Stderr = logutil.NewSink(ctx, slog.Warn, "newagent: ")
		err := cmd.Run()

		return errors.Wrap(err, "failed to run new agent")
	})
}

func UpgradeBin(ctx context.Context, source, version, ca, username, password, target, name string, testFunc func(ctx context.Context, binPath string) error) error {
	tmpPath, err := os.MkdirTemp(target, "bin-upgrade-*")
	if err != nil {
		return errors.Wrap(err, "failed to create temp dir")
	}
	defer os.RemoveAll(tmpPath)

	fs, err := file.New(tmpPath)
	if err != nil {
		return errors.Wrapf(err, "error creating oras file store in %s", tmpPath)
	}
	defer fs.Close()

	repo, err := remote.NewRepository(source)
	if err != nil {
		return errors.Wrapf(err, "error creating oras remote repo %s", source)
	}

	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM([]byte(ca)) {
		return errors.New("failed to append CA cert to rootCAs")
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
		return errors.Wrapf(err, "error downloading new %s bin %s from %s", name, version, source)
	}

	binPath := filepath.Join(tmpPath, name)

	err = os.Chmod(binPath, 0o755)
	if err != nil {
		return errors.Wrapf(err, "failed to chmod new %s bin in %s", name, tmpPath)
	}

	if err := testFunc(ctx, binPath); err != nil {
		return errors.Wrapf(err, "failed to test new %s bin in %s", name, tmpPath)
	}

	targetPath := filepath.Join(target, name)
	err = os.Rename(binPath, targetPath)
	if err != nil {
		return errors.Wrapf(err, "failed to move new bin to %s", targetPath)
	}

	return nil
}
