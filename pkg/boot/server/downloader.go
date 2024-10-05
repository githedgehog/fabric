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

package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

func newORASClient(credsPath, caPath string) (*auth.Client, error) {
	storeOpts := credentials.StoreOptions{}
	credStore, err := credentials.NewStore(credsPath, storeOpts)
	if err != nil {
		return nil, fmt.Errorf("creating docker credential store for %s: %w", credsPath, err)
	}

	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert %s: %w", caPath, err)
	}

	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("appending CA cert to rootCAs: %w", err)
	}

	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		RootCAs:            rootCAs,
	}

	return &auth.Client{
		Client: &http.Client{
			Transport: retry.NewTransport(baseTransport),
		},
		Cache:      auth.DefaultCache,
		Credential: credentials.Credential(credStore),
	}, nil
}

func (svc *service) getCachedOrDownload(ctx context.Context, repo, version string, download bool) (string, error) {
	cacheName := getCacheName(repo, version)
	cachePath := filepath.Join(svc.cacheDir, cacheName)

	if _, err := os.Stat(cachePath); err != nil {
		if download && errors.Is(err, os.ErrNotExist) {
			if err := svc.downloadFiles(ctx, svc.cacheDir, cacheName, repo, version); err != nil {
				return "", fmt.Errorf("downloading files: %w", err)
			}
		} else {
			return "", fmt.Errorf("stat %s: %w", cachePath, err)
		}
	}

	entries, err := os.ReadDir(cachePath)
	if err != nil {
		return "", fmt.Errorf("reading dir %s: %w", cachePath, err)
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("empty cache dir %s", cachePath) //nolint:goerr113
	}
	if len(entries) > 1 {
		return "", fmt.Errorf("multiple entries in cache dir %s", cachePath) //nolint:goerr113
	}

	return filepath.Join(cachePath, entries[0].Name()), nil
}

func (svc *service) downloadFiles(ctx context.Context, cacheDir, cacheName, ref, tag string) error {
	// TODO need a per-cache lock, move downloadLock back to downloadFiles
	svc.downloadLock.Lock()
	defer svc.downloadLock.Unlock()

	tmp, err := os.MkdirTemp(cacheDir, "download-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	fs, err := file.New(tmp)
	if err != nil {
		return fmt.Errorf("creating oras file store %s: %w", tmp, err)
	}
	defer fs.Close()

	repo, err := remote.NewRepository(ref)
	if err != nil {
		return fmt.Errorf("creating oras remote repo %s: %w", ref, err)
	}

	repo.Client = svc.orasClient

	logProgress := func(stage string) func(context.Context, ocispec.Descriptor) error {
		return func(_ context.Context, desc ocispec.Descriptor) error {
			if desc.Annotations == nil || desc.Annotations["org.opencontainers.image.title"] == "" {
				return nil
			}

			slog.Info(stage, "name", desc.Annotations["org.opencontainers.image.title"],
				"size", humanize.IBytes(uint64(desc.Size)), "digest", desc.Digest.Encoded()[:12], //nolint:gosec
				"ref", ref, "tag", tag)

			return nil
		}
	}

	_, err = oras.Copy(ctx, repo, tag, fs, "", oras.CopyOptions{
		CopyGraphOptions: oras.CopyGraphOptions{
			Concurrency:   4,
			PreCopy:       logProgress("Downloading"),
			PostCopy:      logProgress("Downloaded"),
			OnCopySkipped: logProgress("Skipped"),
		},
	})
	if err != nil {
		return fmt.Errorf("downloading files from %s:%s: %w", ref, tag, err)
	}

	cachePath := filepath.Join(cacheDir, cacheName)
	if err := os.Rename(tmp, cachePath); err != nil {
		return fmt.Errorf("moving %s to %s: %w", tmp, cachePath, err)
	}

	return nil
}

func getCacheName(ref, tag string) string {
	res := ref + "@" + tag
	res = strings.TrimPrefix(res, "https://")
	res = strings.TrimPrefix(res, "http://")
	res = strings.ReplaceAll(res, "/", "_")
	res = strings.ReplaceAll(res, ":", "_")

	return res
}
