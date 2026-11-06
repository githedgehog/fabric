// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

// Package nvue is a read-only client for the NVUE REST API of Cumulus Linux, the same API the nv CLI uses, see
// https://docs.nvidia.com/networking-ethernet-software/cumulus-linux/System-Configuration/NVUE-API/
package nvue

import (
	"cmp"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// DefaultAddress is where nvued listens on the switch itself
	DefaultAddress = "https://127.0.0.1:8765/nvue_v1"

	// The environment overrides the address and the agent account, e.g. to reach a switch through a port forward
	EnvAddress  = "HH_NVUE_ADDRESS"
	EnvUsername = "HH_NVUE_USERNAME"
	EnvPassword = "HH_NVUE_PASSWORD"

	requestTimeout = 30 * time.Second
	// Enough for the full interface listing of a switch with every port broken out
	maxResponseSize = 64 << 20
)

var (
	// ErrNotFound matches the 404 NVUE answers both for unknown paths and for items that exist in the schema but
	// have no data, such as counters on a port that doesn't keep them
	ErrNotFound = errors.New("not found")
	// ErrNoCredentials means there's no password in the environment and no basedir to keep one in
	ErrNoCredentials = errors.New("no NVUE credentials configured")
)

// Client talks to a single NVUE API endpoint using basic auth, it's safe for concurrent use
type Client struct {
	address  string
	username string
	password string
	http     *http.Client
}

// New returns a client for the API at address, which includes the version prefix like DefaultAddress does
func New(address, username, password string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // TODO nvued serves a self-signed certificate, pin or install it
	}

	return &Client{
		address:  strings.TrimSuffix(address, "/"),
		username: username,
		password: password,
		http: &http.Client{
			Transport: transport,
			Timeout:   requestTimeout,
		},
	}
}

// NewForAgent builds the client the agent collects the switch state with: its own account on the local API, with
// the password kept in basedir (see AgentPassword), unless the HH_NVUE_* environment says otherwise. Without a
// basedir the environment has to provide the password, or it's ErrNoCredentials.
func NewForAgent(basedir string) (*Client, error) {
	address := cmp.Or(os.Getenv(EnvAddress), DefaultAddress)
	username := cmp.Or(os.Getenv(EnvUsername), AgentUser)
	password := os.Getenv(EnvPassword)

	if password == "" {
		if basedir == "" {
			return nil, ErrNoCredentials
		}

		var err error
		password, err = AgentPassword(basedir)
		if err != nil {
			return nil, err
		}
	}

	return New(address, username, password), nil
}

// Error is the problem-details body NVUE returns on failure
type Error struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Type   string `json:"type"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("nvue: %d %s: %s", e.Status, e.Title, e.Detail)
}

// Is lets callers check errors.Is(err, ErrNotFound) without knowing about the status code
func (e *Error) Is(target error) bool {
	return errors.Is(target, ErrNotFound) && e.Status == http.StatusNotFound
}

// Get reads the operational state at path, such as "/interface/swp1/counters", and decodes it into out
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	reqURL := c.address + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request for %s: %w", path, err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("getting %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	if resp.StatusCode != http.StatusOK {
		apiErr := &Error{}
		if json.Unmarshal(body, apiErr) != nil || apiErr.Title == "" {
			// not every failure comes from nvued itself, e.g. a proxy in between
			apiErr.Title = http.StatusText(resp.StatusCode)
			apiErr.Detail = strings.TrimSpace(string(body))
		}
		apiErr.Status = resp.StatusCode

		return fmt.Errorf("getting %s: %w", path, apiErr)
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding %s: %w", path, err)
	}

	return nil
}

// interfacePath builds "/interface/<name>/<sub...>" with the interface name escaped
func interfacePath(name string, sub ...string) string {
	parts := append([]string{"/interface", url.PathEscape(name)}, sub...)

	return strings.Join(parts, "/")
}
