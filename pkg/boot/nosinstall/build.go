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

package nosinstall

import (
	"archive/tar"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"go.githedgehog.com/fabric/pkg/boot/nosinstall/bin"
)

const (
	Magic               = "hedgehog"
	NOSInstallerName    = "nos-install"
	AgentBinaryName     = "agent"
	AgentConfigName     = "agent-config.yaml"
	AgentKubeConfigName = "agent-kubeconfig"
	AgentUnitName       = "hedgehog-agent.service"
	AgentUnitContent    = `
[Unit]
Description=Hedgehog Fabric Agent

[Service]
User=root
ExecStart=/opt/hedgehog/bin/agent start

Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
)

type Builder struct {
	AgentConfig []byte
	KubeConfig  []byte
	NOSPath     string
	AgentPath   string
}

func (b *Builder) Build(w io.Writer) error {
	if err := bin.WriteNOSInstall(w); err != nil {
		return fmt.Errorf("writing nos-install binary: %w", err)
	}

	cw := NewCountingWriter(w)
	tw := tar.NewWriter(cw)

	if err := addFileFromPath(tw, NOSInstallerName, b.NOSPath); err != nil {
		return fmt.Errorf("adding NOS installer: %w", err)
	}

	if err := addFileFromPath(tw, AgentBinaryName, b.AgentPath); err != nil {
		return fmt.Errorf("adding agent binary: %w", err)
	}

	if err := addFileFromData(tw, AgentKubeConfigName, b.KubeConfig); err != nil {
		return fmt.Errorf("adding agent kubeconfig: %w", err)
	}

	if err := addFileFromData(tw, AgentConfigName, b.AgentConfig); err != nil {
		return fmt.Errorf("adding agent config: %w", err)
	}

	if err := addFileFromData(tw, AgentUnitName, []byte(AgentUnitContent)); err != nil {
		return fmt.Errorf("adding agent systemd unit: %w", err)
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing tar writer: %w", err)
	}

	payloadSize := binary.BigEndian.AppendUint64(nil, cw.Bytes())
	if _, err := w.Write(payloadSize); err != nil {
		return fmt.Errorf("writing payload size: %w", err)
	}

	// TODO: add embedded tar hash? or complete file hash?

	if _, err := w.Write([]byte(Magic)); err != nil {
		return fmt.Errorf("writing magic: %w", err)
	}

	return nil
}

func addFileFromPath(tw *tar.Writer, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening file %s: %w", path, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stating file %s: %w", path, err)
	}

	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: stat.Size(),
	}); err != nil {
		return fmt.Errorf("writing tar header for %s: %w", name, err)
	}

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("writing file %s: %w", name, err)
	}

	return nil
}

func addFileFromData(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(data)),
	}); err != nil {
		return fmt.Errorf("writing tar header for %s: %w", name, err)
	}

	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("writing data for %s: %w", name, err)
	}

	return nil
}

type CountingWriter struct {
	w io.Writer
	n uint64
}

var _ io.Writer = &CountingWriter{}

func NewCountingWriter(w io.Writer) *CountingWriter {
	return &CountingWriter{w: w}
}

func (cw *CountingWriter) Write(b []byte) (int, error) {
	cw.n += uint64(len(b))

	return cw.w.Write(b) //nolint: wrapcheck
}

func (cw *CountingWriter) Bytes() uint64 {
	return cw.n
}
