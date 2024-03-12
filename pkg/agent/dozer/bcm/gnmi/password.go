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
)

const (
	PASSWD_LOWER   = "abcdefghijklmnopqrstuvwxyz"
	PASSWD_UPPER   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	PASSWD_DIGITS  = "0123456789"
	PASSWD_SYMBOLS = "~!@#$%^&*()_+`-={}|[]\\:\"<>?,./"
	PASSWD_LENGTH  = 32
)

var PASSWD_ALPHABET = PASSWD_LOWER + PASSWD_UPPER + PASSWD_DIGITS + PASSWD_SYMBOLS

func RandomPassword() (string, error) {
	var res bytes.Buffer
	for idx := 0; idx < PASSWD_LENGTH; idx++ {
		r, err := rand.Int(rand.Reader, big.NewInt(int64(len(PASSWD_ALPHABET))))
		if err != nil {
			return "", err
		}
		err = res.WriteByte(PASSWD_ALPHABET[r.Int64()])
		if err != nil {
			return "", err
		}
	}

	return res.String(), nil
}
