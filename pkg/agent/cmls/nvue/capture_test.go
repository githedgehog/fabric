// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// envCapture is the directory to capture the fixtures into from the local nvued, see testdata/README.md
const envCapture = "NVUE_CAPTURE"

// captureRequests are the requests the fixtures are captured for, relative to the API prefix and escaped like the
// client sends them
var captureRequests = []string{
	"/system/version",
	"/interface/swp63/link/breakout?rev=applied",
	"/platform/environment/fan/FAN1%2F1",
	"/interface/swp999",
	"/vrf/mgmt/router/bgp",
	"/system/forwarding",
	"/nosuchthing",
	"/interface?view=nosuchview",
}

// captureForbiddenRequests are captured into the "nobody" subdirectory by a user without an NVUE role
var captureForbiddenRequests = []string{
	"/system/version",
}

func TestCapture(t *testing.T) {
	dir := os.Getenv(envCapture)
	if dir == "" {
		t.Skip(envCapture + " isn't set")
	}

	requests := captureRequests
	if os.Geteuid() != 0 {
		dir = filepath.Join(dir, "nobody")
		requests = captureForbiddenRequests
	}
	require.NoError(t, os.MkdirAll(dir, 0o755)) //nolint:gosec // the directory to capture into is the input

	dialer := &net.Dialer{}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", DefaultSocket)
			},
		},
		Timeout: DefaultTimeout,
	}

	scrub := newScrubber()
	statuses := map[string]int{}
	docs := map[string]any{}
	for _, uri := range requests {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+uri, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err, uri)
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, err, uri)

		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		var doc any
		require.NoError(t, dec.Decode(&doc), uri)

		statuses[uri] = resp.StatusCode
		docs[uri] = scrub.doc(doc)
	}

	files, err := scrub.finish(docs)
	require.NoError(t, err)

	for uri, data := range files {
		name := fixtureName(uri)
		if status := statuses[uri]; status != http.StatusOK {
			name += fmt.Sprintf(".%d", status)
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644)) //nolint:gosec
	}
}

var (
	macRe = regexp.MustCompile(`(?i)\b[0-9a-f]{2}(?::[0-9a-f]{2}){5}\b`)
	// the null, broadcast and fabric VRR MACs don't identify a device
	keepMACs     = []string{"00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "00:00:00:11:11:11"}
	serialKeys   = []string{"vendor-sn", "serial", "serial-number"}
	uuidKeys     = []string{"system-uuid"}
	placeholders = []string{"", "n/a", "N/A", "NA", "none", "None", "Not Specified", "Unknown", "unknown"}
	secretKeys   = []string{"hashed-password", "password", "secret", "authorized-key", "private-key"}
)

// scrubber replaces the MACs, serial numbers and system UUIDs of the captured switch with fakes, consistently across
// all responses, the MACs also where they're embedded into IPv6 link-local addresses or system UUIDs
type scrubber struct {
	macs    map[string]string
	serials map[string]string
	uuids   map[string]string
	secrets []string
}

func newScrubber() *scrubber {
	return &scrubber{
		macs:    map[string]string{},
		serials: map[string]string{},
		uuids:   map[string]string{},
	}
}

func (s *scrubber) mac(orig string) string {
	if slices.Contains(keepMACs, strings.ToLower(orig)) {
		return orig
	}

	key := strings.ToLower(orig)
	fake, ok := s.macs[key]
	if !ok {
		n := len(s.macs) + 1
		// locally administered, and the EUI-64 interface id of it has no zero groups to compress
		fake = fmt.Sprintf("0a:1b:2c:00:%02x:%02x", n>>8, n&0xff)
		s.macs[key] = fake
	}
	if orig == strings.ToUpper(orig) {
		return strings.ToUpper(fake)
	}

	return fake
}

func (s *scrubber) str(v string) string {
	return macRe.ReplaceAllStringFunc(v, s.mac)
}

func (s *scrubber) doc(doc any) any {
	return s.value("", doc)
}

func (s *scrubber) value(key string, v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			if slices.Contains(secretKeys, k) {
				s.secrets = append(s.secrets, k)
			}
			out[s.str(k)] = s.value(k, item)
		}

		return out
	case []any:
		out := make([]any, 0, len(val))
		for _, item := range val {
			out = append(out, s.value(key, item))
		}

		return out
	case string:
		switch {
		case slices.Contains(placeholders, val):
			return val
		case slices.Contains(serialKeys, key):
			return fake(s.serials, val, func(n int) string { return fmt.Sprintf("SN%08d", n) })
		case slices.Contains(uuidKeys, key):
			return fake(s.uuids, val, func(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) })
		default:
			return s.str(val)
		}
	default:
		return v
	}
}

func fake(seen map[string]string, orig string, gen func(int) string) string {
	if f, ok := seen[orig]; ok {
		return f
	}
	f := gen(len(seen) + 1)
	seen[orig] = f

	return f
}

// eui64 is the interface id of an IPv6 address derived from the MAC, as nvued prints it
func eui64(mac string) string {
	b := make([]byte, 0, 6)
	for part := range strings.SplitSeq(mac, ":") {
		var v byte
		_, _ = fmt.Sscanf(part, "%02x", &v)
		b = append(b, v)
	}
	b[0] ^= 0x02

	return fmt.Sprintf("%x:%x:%x:%x", uint16(b[0])<<8|uint16(b[1]), uint16(b[2])<<8|0xff, 0xfe00|uint16(b[3]),
		uint16(b[4])<<8|uint16(b[5]))
}

// finish renders the scrubbed responses, replacing the remaining forms of the MACs, and checks that none of the
// original values is left anywhere
func (s *scrubber) finish(docs map[string]any) (map[string][]byte, error) {
	if len(s.secrets) > 0 {
		return nil, fmt.Errorf("responses with secrets: %v", s.secrets) //nolint:err113
	}

	forms := map[string]string{}
	for orig, f := range s.macs {
		forms[eui64(orig)] = eui64(f)
		forms[strings.ReplaceAll(orig, ":", "")] = strings.ReplaceAll(f, ":", "")
	}
	replacer := make([]string, 0, 2*len(forms))
	for orig, f := range forms {
		replacer = append(replacer, orig, f)
	}

	leftovers := []string{}
	for orig := range s.macs {
		leftovers = append(leftovers, orig)
	}
	for orig := range forms {
		leftovers = append(leftovers, orig)
	}
	for orig := range s.serials {
		leftovers = append(leftovers, strings.ToLower(orig))
	}
	for orig := range s.uuids {
		leftovers = append(leftovers, strings.ToLower(orig))
	}

	files := map[string][]byte{}
	for uri, doc := range docs {
		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			return nil, fmt.Errorf("encoding %s: %w", uri, err)
		}

		text := caseInsensitiveReplace(buf.String(), replacer)
		lower := strings.ToLower(text)
		for _, orig := range leftovers {
			if strings.Contains(lower, orig) {
				return nil, fmt.Errorf("%s still has %q", uri, orig) //nolint:err113
			}
		}

		files[uri] = []byte(text)
	}

	return files, nil
}

func caseInsensitiveReplace(text string, pairs []string) string {
	for i := 0; i < len(pairs); i += 2 {
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(pairs[i]))
		text = re.ReplaceAllString(text, pairs[i+1])
	}

	return text
}

// the values are made up, the MAC is from the documentation range (RFC 7042)
func TestScrubber(t *testing.T) {
	scrub := newScrubber()
	docs := map[string]any{
		"/interface": scrub.doc(map[string]any{
			"swp1": map[string]any{
				"link": map[string]any{"mac-address": "00:00:5e:00:53:01"},
				"ipv6": map[string]any{"address": map[string]any{"fe80::200:5eff:fe00:5301/64": map[string]any{}}},
			},
			"vlan10": map[string]any{
				"link": map[string]any{"mac-address": "00:00:00:11:11:11"},
			},
		}),
		"/platform": scrub.doc(map[string]any{
			"system-mac":    "00:00:5E:00:53:01",
			"system-uuid":   "12345678-0000-4000-8000-00005e005301",
			"serial-number": "TESTSERIAL001",
			"part-number":   "PN-TEST-0001",
		}),
		"/platform/transceiver/swp1": scrub.doc(map[string]any{
			"vendor-sn":   "TESTSERIAL002",
			"vendor-oui":  "00:00:5e",
			"vendor-name": "Example",
		}),
		"/platform/transceiver/swp2": scrub.doc(map[string]any{
			"vendor-sn": "N/A",
		}),
	}

	files, err := scrub.finish(docs)
	require.NoError(t, err)

	all := ""
	for _, data := range files {
		all += strings.ToLower(string(data))
	}
	originals := []string{"00:00:5e:00:53:01", "200:5eff:fe00:5301", "00005e005301", "testserial", "12345678-"}
	for _, orig := range originals {
		require.NotContains(t, all, orig)
	}

	require.JSONEq(t, `{
		"swp1": {
			"link": {"mac-address": "0a:1b:2c:00:00:01"},
			"ipv6": {"address": {"fe80::81b:2cff:fe00:1/64": {}}}
		},
		"vlan10": {"link": {"mac-address": "00:00:00:11:11:11"}}
	}`, string(files["/interface"]))
	require.JSONEq(t, `{
		"system-mac": "0A:1B:2C:00:00:01",
		"system-uuid": "00000000-0000-0000-0000-000000000001",
		"serial-number": "SN00000001",
		"part-number": "PN-TEST-0001"
	}`, string(files["/platform"]))
	require.JSONEq(t, `{"vendor-sn": "SN00000002", "vendor-oui": "00:00:5e", "vendor-name": "Example"}`,
		string(files["/platform/transceiver/swp1"]))
	require.JSONEq(t, `{"vendor-sn": "N/A"}`, string(files["/platform/transceiver/swp2"]))

	scrub = newScrubber()
	_, err = scrub.finish(map[string]any{"/system/aaa/user/admin": scrub.doc(map[string]any{"hashed-password": "*"})})
	require.ErrorContains(t, err, "secrets")
}
