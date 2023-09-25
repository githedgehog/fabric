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
