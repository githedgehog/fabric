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

func TestLoadConfigFromFileDroppedFields(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
metadata:
  name: leaf-01
spec:
  catalog:
    loopbackWorkaroundLinks:
      vpc@vpc-1--vpc-2: E1/20--E1/21
    loopbackWorkaroundVLANs:
      vpc@vpc-1--vpc-2: 100
  config:
    loopbackWorkaround: true
    vpcLoopbackSubnet: 172.30.96.0/19
  connections:
    leaf-01--external:
      external:
        link:
          switch:
            port: leaf-01/E1/1
    leaf-01--vpc-loopback:
      vpcLoopback:
        links:
          - switch1:
              port: leaf-01/E1/20
            switch2:
              port: leaf-01/E1/21
status:
  lastAppliedGen: 7
`), 0o600))

	svc := &Service{Basedir: dir}
	agent, err := svc.loadConfigFromFile()
	require.NoError(t, err)
	require.Equal(t, "leaf-01", agent.Name)
	require.Len(t, agent.Spec.Connections, 1)
	require.Contains(t, agent.Spec.Connections, "leaf-01--external")

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
