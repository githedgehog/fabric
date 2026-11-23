// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var errNotScalar = errors.New("not a scalar")

// leadingNumber is the numeric part of values such as "54.0", "51.38 C", "2.00m" or "-2.22 dBm"
var leadingNumber = regexp.MustCompile(`^\s*[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?`)

// Number is a value NVUE reports as a JSON number, a number in a string ("6974", "54.0"), a number with a unit
// ("113.932 mA", "51.38 C") or a placeholder such as "N/A". OK is false for the placeholders, Raw keeps the original
// text either way, so the meaning of a non-numeric value isn't lost (a BGP remote-as of "external" for one).
type Number struct {
	Value float64
	OK    bool
	Raw   string
}

func (n *Number) UnmarshalJSON(data []byte) error {
	*n = Number{}

	raw, err := decodeScalar(data)
	if err != nil {
		return err
	}

	switch v := raw.(type) {
	case nil:
	case json.Number:
		n.Raw = v.String()
		n.Value, err = v.Float64()
		if err != nil {
			return fmt.Errorf("parsing number %q: %w", v, err)
		}
		n.OK = true
	case string:
		n.Raw = v
		n.Value, n.OK = parseLeadingNumber(v)
	default:
		return fmt.Errorf("number: %w: %s", errNotScalar, data)
	}

	return nil
}

// Counter is a packet or byte counter, which NVUE reports as a JSON number or as "n/a" when it doesn't apply, such as
// the transmit side of ACL drops. Unlike Number it keeps all 64 bits.
type Counter struct {
	Value uint64
	OK    bool
}

func (c *Counter) UnmarshalJSON(data []byte) error {
	*c = Counter{}

	raw, err := decodeScalar(data)
	if err != nil {
		return err
	}

	var text string
	switch v := raw.(type) {
	case nil:
		return nil
	case json.Number:
		text = v.String()
	case string:
		text = strings.TrimSpace(v)
	default:
		return fmt.Errorf("counter: %w: %s", errNotScalar, data)
	}

	// anything that isn't a plain unsigned integer is a placeholder, there's nothing to count
	if value, err := strconv.ParseUint(text, 10, 64); err == nil {
		c.Value, c.OK = value, true
	}

	return nil
}

func parseLeadingNumber(s string) (float64, bool) {
	match := leadingNumber.FindString(s)
	if match == "" {
		return 0, false
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(match), 64)

	return value, err == nil
}

// decodeScalar decodes a JSON scalar keeping numbers as json.Number, so that large counters aren't rounded
func decodeScalar(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", data, err)
	}

	return raw, nil
}
