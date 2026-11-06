// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	// AgentUser is the switch account the agent reads the state with, the same name as on SONiC
	AgentUser = "hhagent"
	// AgentRole only allows nv show, a leaked password can't change the switch
	AgentRole = "nvue-monitor"

	// AgentPasswordFile in the agent basedir holds the password of AgentUser, generated on first use
	AgentPasswordFile = "nvue-passwd"
	// AgentPasswordHashFile keeps the hash the password was last applied with, so that the config doesn't change
	// on every apply as it would with a fresh salt each time
	AgentPasswordHashFile = "nvue-passwd-hash" //nolint:gosec // a file name, not a credential

	passwordLength   = 32
	passwordAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// AgentPassword returns the password of the agent's own switch account, generating and saving one on first use
func AgentPassword(basedir string) (string, error) {
	path := filepath.Join(basedir, AgentPasswordFile)

	data, err := os.ReadFile(path)
	if err == nil {
		if password := strings.TrimSpace(string(data)); password != "" {
			return password, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	password, err := randomPassword()
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, []byte(password+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}

	return password, nil
}

// AgentPasswordHash returns the hash to configure the agent's account with, as the user's hashed-password, reusing
// the saved one as long as it still matches the password
func AgentPasswordHash(basedir, password string) (string, error) {
	path := filepath.Join(basedir, AgentPasswordHashFile)

	data, err := os.ReadFile(path)
	if err == nil {
		hash := strings.TrimSpace(string(data))
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
			return hash, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, []byte(hash+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}

	return hash, nil
}

// HashPassword produces a hashed-password for a user: bcrypt, like the rest of the fabric, which Cumulus Linux
// accepts as a crypt(3) hash
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}

	return string(hash), nil
}

func randomPassword() (string, error) {
	var password strings.Builder
	for range passwordLength {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordAlphabet))))
		if err != nil {
			return "", fmt.Errorf("generating password: %w", err)
		}

		password.WriteByte(passwordAlphabet[idx.Int64()])
	}

	return password.String(), nil
}
