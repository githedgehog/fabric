// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"errors"
	"fmt"
	"math"
)

const (
	VPCIDEncodeAlphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	VPCIDEncodeLength   = 5
)

var VPCID *PaddedEncoder

func init() {
	var err error
	VPCID, err = NewPaddedEncoder(VPCIDEncodeAlphabet, VPCIDEncodeLength)
	if err != nil {
		panic(fmt.Errorf("failed to create VPCID encoder: %w", err))
	}
}

var (
	ErrInvalidEncoder = errors.New("invalid encoder")
	ErrTooLarge       = errors.New("value is too large")
)

type PaddedEncoder struct {
	alphabet       string
	alphabetLength uint32
	length         int
	maxValue       uint32
	decoderMap     [256]uint8
}

func NewPaddedEncoder(alphabet string, length int) (*PaddedEncoder, error) {
	if length < 2 {
		return nil, fmt.Errorf("%w: length %d < 2", ErrInvalidEncoder, length)
	}

	maxValue := math.Pow(float64(len(alphabet)), float64(length)) - 1
	if maxValue > math.MaxUint32 {
		return nil, fmt.Errorf("%w: encoder max value %f > uint32 max value %d", ErrInvalidEncoder, maxValue, math.MaxUint32)
	}

	if len(alphabet) > math.MaxUint8 {
		return nil, fmt.Errorf("%w: alphabet length %d > 255", ErrInvalidEncoder, len(alphabet))
	}

	decoderMap := [256]uint8{}
	for idx, r := range alphabet {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return nil, fmt.Errorf("%w: invalid character %c in alphabet", ErrInvalidEncoder, r)
		}

		decoderMap[r] = uint8(idx) //nolint:gosec
	}

	return &PaddedEncoder{
		alphabet:       alphabet,
		alphabetLength: uint32(len(alphabet)), //nolint:gosec
		length:         length,
		maxValue:       uint32(maxValue),
		decoderMap:     decoderMap,
	}, nil
}

func (pe *PaddedEncoder) Encode(val uint32) (string, error) {
	if val > pe.maxValue {
		return "", fmt.Errorf("%w: %d > %d", ErrTooLarge, val, pe.maxValue)
	}

	idStr := ""
	for val > 0 {
		idStr = string(pe.alphabet[val%pe.alphabetLength]) + idStr
		val /= pe.alphabetLength
	}

	for len(idStr) < pe.length {
		idStr = pe.alphabet[0:1] + idStr
	}

	return idStr, nil
}

func (pe *PaddedEncoder) Decode(idStr string) (uint32, error) {
	if len(idStr) != pe.length {
		return 0, fmt.Errorf("%w: invalid length %d, expected %d", ErrInvalidEncoder, len(idStr), pe.length)
	}

	val := uint32(0)
	for _, r := range idStr {
		if r >= 256 || pe.decoderMap[r] == 0 && r != rune(pe.alphabet[0]) {
			return 0, fmt.Errorf("%w: invalid character %c", ErrInvalidEncoder, r)
		}
		val = val*pe.alphabetLength + uint32(pe.decoderMap[r])
	}

	return val, nil
}

func (pe *PaddedEncoder) GetMaxValue() uint32 {
	return pe.maxValue
}
