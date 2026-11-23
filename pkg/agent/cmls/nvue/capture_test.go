// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue/nvuetest"
)

const (
	// envCapture is the directory to capture the fixtures into from the local nvued, see testdata/README.md
	envCapture = "NVUE_CAPTURE"
	// envImport is a directory captured into to import into testdata
	envImport = "NVUE_IMPORT"
	// envImportRefresh replaces the fixtures that are there already when importing
	envImportRefresh = "NVUE_IMPORT_REFRESH"
)

// captures are the requests the fixtures are captured for, from each test file, per model, the name of the directory
// to capture into. The requests are relative to the API prefix and escaped like the client sends them.
var captures = []map[string][]string{
	clientCaptures,
	interfaceCaptures,
	interfaceCountersCaptures,
	interfaceRatesCaptures,
	breakoutCaptures,
	transceiverCaptures,
	systemCaptures,
	platformCaptures,
	fanCaptures,
	psuCaptures,
	temperatureCaptures,
	firmwareCaptures,
	asicCaptures,
	roceCaptures,
	vrfCaptures,
	bgpCaptures,
	bfdCaptures,
	stateCaptures,
}

// captureForbiddenRequests are captured into the "nobody" subdirectory by a user without an NVUE role, for any model
var captureForbiddenRequests = []string{
	"/system/version",
}

func captureRequests(model string) []string {
	requests := []string{}
	for _, files := range captures {
		for _, uri := range files[model] {
			if !slices.Contains(requests, uri) {
				requests = append(requests, uri)
			}
		}
	}

	return requests
}

// TestFixturesCaptured makes sure that the fixtures are the ones a capture produces, no more and no less
func TestFixturesCaptured(t *testing.T) {
	if isLive() {
		t.Skip("the fixtures aren't used when running live")
	}
	if os.Getenv(envImport) != "" {
		t.Skip("the fixtures are being imported")
	}

	models, err := os.ReadDir("testdata")
	require.NoError(t, err)

	for _, model := range models {
		if !model.IsDir() || model.Name() == goldenDir {
			continue
		}

		for dir, requests := range map[string][]string{
			filepath.Join("testdata", model.Name()):           captureRequests(model.Name()),
			filepath.Join("testdata", model.Name(), "nobody"): captureForbiddenRequests,
		} {
			want := []string{}
			for _, uri := range requests {
				path, _, err := nvuetest.FixtureFile(dir, uri)
				require.NoError(t, err, "no fixture for %s in %s", uri, dir)
				want = append(want, filepath.Base(path))
			}

			files, err := os.ReadDir(dir)
			require.NoError(t, err)
			got := []string{}
			for _, file := range files {
				if !file.IsDir() {
					got = append(got, file.Name())
				}
			}
			require.ElementsMatch(t, want, got, dir)
		}
	}
}

// TestImportCapture brings the fixtures captured into the directory named by envImport into testdata, only the
// missing ones unless envImportRefresh is set, as refreshing the others changes the values the tests expect
func TestImportCapture(t *testing.T) {
	src := os.Getenv(envImport)
	if src == "" {
		t.Skip(envImport + " isn't set")
	}
	refresh := envEnabled(envImportRefresh)

	model := filepath.Base(src)
	requests := captureRequests(model)
	require.NotEmpty(t, requests, "no requests to capture for %s", model)

	changed := []string{}
	for sub, uris := range map[string][]string{"": requests, "nobody": captureForbiddenRequests} {
		srcDir, dstDir := filepath.Join(src, sub), filepath.Join("testdata", model, sub)
		require.NoError(t, os.MkdirAll(dstDir, 0o755))

		for _, uri := range uris {
			srcPath, srcStatus, err := nvuetest.FixtureFile(srcDir, uri)
			require.NoError(t, err, "%s wasn't captured into %s", uri, srcDir)

			dstPath, dstStatus, err := nvuetest.FixtureFile(dstDir, uri)
			if err == nil {
				if !refresh {
					continue
				}
				if dstStatus != srcStatus {
					changed = append(changed, fmt.Sprintf("%s: %d -> %d", uri, dstStatus, srcStatus))

					continue
				}
				require.NoError(t, os.Remove(dstPath))
			}

			data, err := os.ReadFile(srcPath) //nolint:gosec // the directory to import from is the input
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dstDir, filepath.Base(srcPath)), data, 0o644)) //nolint:gosec
			t.Logf("imported %s", filepath.Join(dstDir, filepath.Base(srcPath)))
		}
	}

	// usually the switch isn't set up like the fixtures expect, see testdata/README.md
	require.Empty(t, changed, "the status of some requests changed, remove their fixtures to take the new ones")
}

func TestCapture(t *testing.T) {
	dir := os.Getenv(envCapture)
	if dir == "" {
		t.Skip(envCapture + " isn't set")
	}

	requests := captureRequests(filepath.Base(dir))
	require.NotEmpty(t, requests, "no requests to capture for %s", filepath.Base(dir))
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
		name := nvuetest.FixtureName(uri)
		if status := statuses[uri]; status != http.StatusOK {
			name += fmt.Sprintf(".%d", status)
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644)) //nolint:gosec
	}
}

var (
	// fakeMACPrefix is locally administered, and the EUI-64 interface ids of the fakes have no zero groups to compress
	fakeMACPrefix = "0a:1b:2c:00:"
	macRe         = regexp.MustCompile(`(?i)\b[0-9a-f]{2}(?::[0-9a-f]{2}){5}\b`)
	// eui64Re is the interface id of an IPv6 address derived from a MAC: the MAC with ff:fe in the middle
	eui64Re = regexp.MustCompile(`(?i)\b([0-9a-f]{1,4}):([0-9a-f]{0,2})ff:fe([0-9a-f]{2}):([0-9a-f]{1,4})\b`)
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
		fake = fmt.Sprintf("%s%02x:%02x", fakeMACPrefix, n>>8, n&0xff)
		s.macs[key] = fake
	}
	if orig == strings.ToUpper(orig) {
		return strings.ToUpper(fake)
	}

	return fake
}

// euiID replaces the EUI-64 interface id of an IPv6 address with the one of the fake MAC, for the MACs the switch
// reports only that way too, such as in the LLDP management address of a neighbor
func (s *scrubber) euiID(id string) string {
	return eui64(s.mac(euiMAC(id)))
}

// euiMAC is the MAC an EUI-64 interface id is derived from
func euiMAC(id string) string {
	m := eui64Re.FindStringSubmatch(strings.ToLower(id))
	high, mid, low := padHex(m[1], 4), padHex(m[2], 2), padHex(m[4], 4)
	first, _ := strconv.ParseUint(high[:2], 16, 8) // two hex digits by the regexp

	return fmt.Sprintf("%02x:%s:%s:%s:%s:%s", first^0x02, high[2:], mid, m[3], low[:2], low[2:])
}

func padHex(s string, n int) string {
	return strings.Repeat("0", n-len(s)) + s
}

func (s *scrubber) str(v string) string {
	return macRe.ReplaceAllStringFunc(eui64Re.ReplaceAllStringFunc(v, s.euiID), s.mac)
}

func (s *scrubber) doc(doc any) any {
	return s.value("", doc)
}

func (s *scrubber) value(key string, v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		// in order, so that the same capture gets the same fakes
		for _, k := range slices.Sorted(maps.Keys(val)) {
			if slices.Contains(secretKeys, k) {
				s.secrets = append(s.secrets, k)
			}
			out[s.str(k)] = s.value(k, val[k])
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

	// the EUI-64 forms are replaced with the strings already
	forms := map[string]string{}
	for orig, f := range s.macs {
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
	for orig := range s.macs {
		leftovers = append(leftovers, eui64(orig))
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
		for _, id := range eui64Re.FindAllString(lower, -1) {
			if mac := euiMAC(id); !strings.HasPrefix(mac, fakeMACPrefix) && !slices.Contains(keepMACs, mac) {
				return nil, fmt.Errorf("%s still has an IPv6 address of MAC %s", uri, mac) //nolint:err113
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
			"swp2": map[string]any{
				"lldp": map[string]any{"neighbor": map[string]any{"spine": map[string]any{
					"management-address-ipv6": "fe80::200:5eff:fe00:5302",
				}}},
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
	originals := []string{"00:00:5e:00:53:01", "200:5eff:fe00:530", "00005e005301", "testserial", "12345678-"}
	for _, orig := range originals {
		require.NotContains(t, all, orig)
	}

	require.JSONEq(t, `{
		"swp1": {
			"link": {"mac-address": "0a:1b:2c:00:00:01"},
			"ipv6": {"address": {"fe80::81b:2cff:fe00:1/64": {}}}
		},
		"swp2": {
			"lldp": {"neighbor": {"spine": {"management-address-ipv6": "fe80::81b:2cff:fe00:2"}}}
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
