// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	gwintapi "go.githedgehog.com/fabric/api/gwint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
	kyaml "sigs.k8s.io/yaml"
)

type GatewayValidator struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

func NewGatewayValidator(ctx context.Context, fabricCfg *meta.FabricConfig, ca []byte, credsPath string) (*GatewayValidator, error) {
	if fabricCfg == nil {
		return nil, fmt.Errorf("fabricCfg is nil") //nolint:err113
	}

	v := &GatewayValidator{}
	if fabricCfg.DataplaneValidatorRef == "" {
		slog.Info("Skipping Dataplane validator as it is not configured")

		return v, nil
	}

	if len(ca) == 0 {
		return nil, fmt.Errorf("ca is empty") //nolint:err113
	}
	if credsPath == "" {
		return nil, fmt.Errorf("credsPath is empty") //nolint:err113
	}

	colonIdx := strings.LastIndex(fabricCfg.DataplaneValidatorRef, ":")
	if colonIdx == -1 {
		return nil, fmt.Errorf("invalid ref format: %s", fabricCfg.DataplaneValidatorRef) //nolint:err113
	}
	ref := fabricCfg.DataplaneValidatorRef[:colonIdx]
	version := fabricCfg.DataplaneValidatorRef[colonIdx+1:]

	slog.Debug("Downloading dataplane validator", "ref", ref, "version", version)

	credStore, err := credentials.NewStore(credsPath, credentials.StoreOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating docker credential store for %s: %w", credsPath, err)
	}

	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("failed to append CA cert to rootCAs") //nolint:err113
	}

	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		RootCAs:            rootCAs,
	}

	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("creating oras remote repo %s: %w", ref, err)
	}

	repo.Client = &auth.Client{
		Client: &http.Client{
			Transport: retry.NewTransport(baseTransport),
		},
		Cache:      auth.DefaultCache,
		Credential: credentials.Credential(credStore),
	}

	tmp, err := os.MkdirTemp("", "download-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	fs, err := file.New(tmp)
	if err != nil {
		return nil, fmt.Errorf("creating oras file store %s: %w", tmp, err)
	}
	defer fs.Close()

	_, err = oras.Copy(ctx, repo, version, fs, "", oras.CopyOptions{
		CopyGraphOptions: oras.CopyGraphOptions{
			Concurrency: 1,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("downloading files from %s:%s: %w", ref, version, err)
	}

	wasmBytes, err := os.ReadFile(filepath.Join(tmp, "validator.wasm"))
	if err != nil {
		return nil, fmt.Errorf("reading WASM file: %w", err)
	}

	slog.Debug("Setting up WASM runtime")

	v.runtime = wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, v.runtime)

	slog.Debug("Compiling dataplane validator")

	v.compiled, err = v.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compiling WASM module: %w", err)
	}

	slog.Info("Dataplane validator loaded", "version", version)

	return v, nil
}

func (v *GatewayValidator) Close(ctx context.Context) {
	if v == nil {
		return
	}

	if v.compiled != nil {
		if err := v.compiled.Close(ctx); err != nil {
			slog.Warn("Error closing compiled validator module", "err", err.Error())
		}
	}

	if v.runtime != nil {
		if err := v.runtime.Close(ctx); err != nil {
			slog.Warn("Error closing validator runtime", "err", err.Error())
		}
	}
}

func (v *GatewayValidator) Validate(ctx context.Context, gwAg *gwintapi.GatewayAgent) error {
	if v == nil {
		return nil
	}
	if v.compiled == nil || v.runtime == nil {
		return fmt.Errorf("validator uninitialized") //nolint:err113
	}

	if gwAg.Generation == 0 {
		gwAg.Generation = 1
	}

	data, err := kyaml.Marshal(gwAg)
	if err != nil {
		return fmt.Errorf("marshalling gateway agent: %w", err)
	}

	stdin := bytes.NewBuffer(data)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cfg := wazero.NewModuleConfig().
		WithStdin(stdin).
		WithStdout(stdout).
		WithStderr(stderr).
		WithName("")
	mod, err := v.runtime.InstantiateModule(ctx, v.compiled, cfg)
	if stderr.Len() > 0 {
		slog.Warn("Dp-validator stderr non-empty", "stderr", stderr.String())
	}
	if err != nil {
		return fmt.Errorf("running dp-validator: %w", err)
	}
	defer mod.Close(ctx)

	output := &DataplaneValidatorOutput{}
	if err := kyaml.Unmarshal(stdout.Bytes(), output); err != nil {
		return fmt.Errorf("unmarshalling dp-validator output: %w", err)
	}
	if !output.Success {
		msgs := make([]string, 0, len(output.Errors))
		for _, err := range output.Errors {
			msgs = append(msgs, err.Message)
		}

		return fmt.Errorf("dp-validator: %s", strings.Join(msgs, ", ")) //nolint:err113
	}

	return nil
}

type DataplaneValidatorOutput struct {
	Success bool                      `json:"success"`
	Errors  []DataplaneValidatorError `json:"errors"`
}

type DataplaneValidatorError struct {
	Message string `json:"message"`
}
