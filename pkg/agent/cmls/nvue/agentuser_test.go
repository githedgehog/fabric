// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestAgentPassword(t *testing.T) {
	basedir := t.TempDir()

	password, err := AgentPassword(basedir)
	require.NoError(t, err)
	require.Len(t, password, passwordLength)

	info, err := os.Stat(filepath.Join(basedir, AgentPasswordFile))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// the same one is handed out from then on
	again, err := AgentPassword(basedir)
	require.NoError(t, err)
	require.Equal(t, password, again)

	// an empty file counts as none
	require.NoError(t, os.WriteFile(filepath.Join(basedir, AgentPasswordFile), []byte("\n"), 0o600))
	fresh, err := AgentPassword(basedir)
	require.NoError(t, err)
	require.NotEqual(t, password, fresh)
}

func TestAgentPasswordHash(t *testing.T) {
	basedir := t.TempDir()

	hash, err := AgentPasswordHash(basedir, "secret")
	require.NoError(t, err)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("secret")))

	// stable while the password is, so the switch config doesn't change on every apply
	again, err := AgentPasswordHash(basedir, "secret")
	require.NoError(t, err)
	require.Equal(t, hash, again)

	// replaced once the password changed
	changed, err := AgentPasswordHash(basedir, "other")
	require.NoError(t, err)
	require.NotEqual(t, hash, changed)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(changed), []byte("other")))
}
