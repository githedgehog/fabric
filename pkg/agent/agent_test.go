// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

func TestVirtualSwitchPorts(t *testing.T) {
	dir := t.TempDir()
	for _, nic := range []string{"eth0", "eth1", "eth12", "swp1"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, nic, "device"), 0o755))
	}
	for _, virt := range []string{"Ethernet0", "lo", "docker0", "eth1.100", "Vrf1"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, virt), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bonding_masters"), nil, 0o600))

	ports, err := virtualSwitchPorts(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"eth1", "eth12", "swp1"}, ports)
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
metadata:
  name: leaf-01
spec:
  connections:
    leaf-01--external:
      external:
        link:
          switch:
            port: leaf-01/E1/1
status:
  lastAppliedGen: 7
`), 0o600))

	svc := &Service{Basedir: dir}
	agent, err := svc.loadConfigFromFile()
	require.NoError(t, err)
	require.Equal(t, "leaf-01", agent.Name)
	require.Len(t, agent.Spec.Connections, 1)
	require.Contains(t, agent.Spec.Connections, "leaf-01--external")
	require.Zero(t, agent.Status.LastAppliedGen)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
metadata:
  name: leaf-01
spec:
  config:
    noSuchField: true
`), 0o600))
	_, err = svc.loadConfigFromFile()
	require.ErrorContains(t, err, "unknown field")
}

func TestKnownAnnotations(t *testing.T) {
	require.Empty(t, knownAnnotations(&agentapi.Agent{}))

	require.Equal(t, map[string]string{
		wiringapi.AnnotationSwitchSyslogDebug: "true",
		wiringapi.AnnotationSwitchRouteSumm:   "",
		wiringapi.AnnotationSwitchBCMMutate:   "rules: []",
	}, knownAnnotations(&agentapi.Agent{Annotations: map[string]string{
		wiringapi.AnnotationSwitchSyslogDebug: "true",
		wiringapi.AnnotationSwitchRouteSumm:   "",
		wiringapi.AnnotationSwitchBCMMutate:   "rules: []",
		"fabric.githedgehog.com/topology":     "spine-leaf",
		"other":                               "value",
	}}))
}
