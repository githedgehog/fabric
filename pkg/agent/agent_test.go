// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
