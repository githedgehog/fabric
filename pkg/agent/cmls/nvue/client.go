// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

// Package nvue is a read-only client for the NVUE REST API of Cumulus Linux, talking to nvued over its unix socket.
// nvued checks the NVUE role of the calling process on the socket, so it only answers root and the members of the
// nvshow and nvapply groups.
package nvue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultSocket          = "/run/nvue/nvue.sock"
	DefaultTimeout         = 30 * time.Second
	DefaultMaxResponseSize = 64 << 20

	apiPrefix = "/nvue_v1"
	// nvued ignores the host part, it only has to be a valid one
	baseURL = "http://nvue" + apiPrefix

	// error bodies are only kept for the message, there's no point in keeping a huge one
	maxErrorBodySize = 4 << 10
)

var (
	// ErrNotFound is any 404: nvued answers it both for items that don't exist and for data that doesn't apply, such as
	// the counters of a bond or the BGP instance of a VRF without one, and the problem type doesn't tell them apart
	ErrNotFound         = errors.New("not found")
	ErrResponseTooLarge = errors.New("response too large")
	ErrInvalidOption    = errors.New("invalid option")
)

type Client struct {
	http    *http.Client
	maxSize int64
}

type options struct {
	socket  string
	timeout time.Duration
	maxSize int64
}

type Option func(*options)

// WithSocket sets the path of the nvued unix socket, DefaultSocket by default
func WithSocket(path string) Option {
	return func(o *options) {
		o.socket = path
	}
}

// WithTimeout sets the limit for a whole request including reading the response, DefaultTimeout by default
func WithTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.timeout = timeout
	}
}

// WithMaxResponseSize sets the largest response body accepted, DefaultMaxResponseSize by default
func WithMaxResponseSize(size int64) Option {
	return func(o *options) {
		o.maxSize = size
	}
}

// New returns a client for the nvued socket, safe for concurrent use
func New(opts ...Option) (*Client, error) {
	o := options{
		socket:  DefaultSocket,
		timeout: DefaultTimeout,
		maxSize: DefaultMaxResponseSize,
	}
	for _, opt := range opts {
		opt(&o)
	}

	if o.socket == "" {
		return nil, fmt.Errorf("%w: empty socket path", ErrInvalidOption)
	}
	if o.timeout <= 0 {
		return nil, fmt.Errorf("%w: timeout %s isn't positive", ErrInvalidOption, o.timeout)
	}
	if o.maxSize <= 0 {
		return nil, fmt.Errorf("%w: max response size %d isn't positive", ErrInvalidOption, o.maxSize)
	}

	dialer := &net.Dialer{}

	return &Client{
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", o.socket)
				},
				// nvued closes every connection after the response anyway
				DisableKeepAlives: true,
			},
			Timeout: o.timeout,
		},
		maxSize: o.maxSize,
	}, nil
}

// Error is a failed request, nvued describes it as an RFC 7807 problem or, for some old endpoints, with a message only
type Error struct {
	Status int
	// Type is the problem type, such as "/errors/ItemDoesNotExist" or "about:blank"
	Type   string
	Title  string
	Detail string
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("nvue: %d %s", e.Status, e.Title)
	if e.Detail != "" && e.Detail != e.Title {
		msg += ": " + e.Detail
	}

	return msg
}

func (e *Error) Is(target error) bool {
	return target == ErrNotFound && e.Status == http.StatusNotFound
}

// pathOf builds the API path of the segments, escaping each one as ids such as fan "FAN1/1" contain slashes
func pathOf(segments ...string) string {
	path := strings.Builder{}
	for _, segment := range segments {
		path.WriteString("/")
		path.WriteString(url.PathEscape(segment))
	}

	return path.String()
}

// get reads the response for path, built with pathOf, and query into out
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	reqURL := baseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("preparing request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("getting %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("getting %s: %w", path, readError(resp))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxSize+1))
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if int64(len(body)) > c.maxSize {
		return fmt.Errorf("reading %s: %w: over %d bytes", path, ErrResponseTooLarge, c.maxSize)
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding %s: %w", path, err)
	}

	return nil
}

func readError(resp *http.Response) *Error {
	apiErr := &Error{Status: resp.StatusCode}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
	if err != nil {
		apiErr.Title = http.StatusText(resp.StatusCode)
		apiErr.Detail = fmt.Sprintf("reading error response: %s", err)

		return apiErr
	}

	problem := struct {
		Type    string `json:"type"`
		Title   string `json:"title"`
		Detail  string `json:"detail"`
		Message string `json:"message"`
	}{}
	if json.Unmarshal(body, &problem) == nil && (problem.Title != "" || problem.Message != "") {
		apiErr.Type = problem.Type
		apiErr.Title = problem.Title
		apiErr.Detail = problem.Detail
		if apiErr.Detail == "" {
			apiErr.Detail = problem.Message
		}
	} else {
		apiErr.Detail = strings.TrimSpace(string(body))
	}
	if apiErr.Title == "" {
		apiErr.Title = http.StatusText(resp.StatusCode)
	}

	return apiErr
}
