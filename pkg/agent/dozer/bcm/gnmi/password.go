// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gnmi

import (
	"bytes"
	"crypto/rand"
	"math/big"

	"github.com/pkg/errors"
)

const (
	PasswdLower   = "abcdefghijklmnopqrstuvwxyz"
	PasswdUpper   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	PasswdDigits  = "0123456789"
	PasswdSymbols = "~!@#$%^&*()_+`-={}|[]\\:\"<>?,./"
	PasswdLength  = 32
)

var PasswdAlphabet = PasswdLower + PasswdUpper + PasswdDigits + PasswdSymbols

func RandomPassword() (string, error) {
	var res bytes.Buffer
	for idx := 0; idx < PasswdLength; idx++ {
		r, err := rand.Int(rand.Reader, big.NewInt(int64(len(PasswdAlphabet))))
		if err != nil {
			return "", errors.Wrapf(err, "failed to generate random password")
		}
		err = res.WriteByte(PasswdAlphabet[r.Int64()])
		if err != nil {
			return "", errors.Wrapf(err, "failed to generate random password")
		}
	}

	return res.String(), nil
}
