// © 2022 Nokia.
//
// This code is a Contribution to the gNMIc project (“Work”) made under the Google Software Grant and Corporate Contributor License Agreement (“CLA”) and governed by the Apache License 2.0.
// No other rights or licenses in or to any of Nokia’s intellectual property are granted for any other purpose.
// This code is provided on an “as is” basis without any warranties of any kind.
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// NewTLSConfig generates a *tls.Config based on given CA, certificate, key files and skipVerify flag
// if certificate and key are missing a self signed key pair is generated.
// The certificates paths can be local or remote, http(s) and (s)ftp are supported for remote files.
func NewTLSConfig(ca, cert, key, clientAuth string, skipVerify, genSelfSigned bool) (*tls.Config, error) {
	if !(skipVerify || ca != "" || (cert != "" && key != "")) {
		return nil, nil
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: skipVerify,
	}
	// set clientAuth
	switch clientAuth {
	case "":
		if ca != "" {
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}
	case "request":
		tlsConfig.ClientAuth = tls.RequestClientCert
	case "require":
		tlsConfig.ClientAuth = tls.RequireAnyClientCert
	case "verify-if-given":
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	case "require-verify":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	default:
		return nil, fmt.Errorf("unknown client-auth mode: %s", clientAuth)
	}
	if cert != "" && key != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var certBytes, keyBytes []byte

		errCh := make(chan error, 2)
		wg := new(sync.WaitGroup)
		wg.Add(2)
		go func() {
			defer wg.Done()
			var err error
			certBytes, err = ReadFile(ctx, cert)
			if err != nil {
				errCh <- err
				return
			}
		}()
		go func() {
			defer wg.Done()
			var err error
			keyBytes, err = ReadFile(ctx, key)
			if err != nil {
				errCh <- err
				return
			}
		}()
		wg.Wait()
		close(errCh)
		for err := range errCh {
			return nil, err
		}
		certificate, err := tls.X509KeyPair(certBytes, keyBytes)
		if err != nil {
			return nil, err
		}

		tlsConfig.Certificates = []tls.Certificate{certificate}
	} else if genSelfSigned {
		cert, err := SelfSignedCerts()
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if ca != "" {
		certPool := x509.NewCertPool()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		caFile, err := ReadFile(ctx, ca)
		if err != nil {
			return nil, err
		}
		if ok := certPool.AppendCertsFromPEM(caFile); !ok {
			return nil, errors.New("failed to append certificate")
		}
		tlsConfig.RootCAs = certPool
		tlsConfig.ClientCAs = certPool
	}
	return tlsConfig, nil
}

func SelfSignedCerts() (tls.Certificate, error) {
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, nil
	}
	certTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"openconfig.net"},
		},
		DNSNames:              []string{"openconfig.net"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return tls.Certificate{}, nil
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, nil
	}
	certBuff := new(bytes.Buffer)
	keyBuff := new(bytes.Buffer)
	pem.Encode(certBuff, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	pem.Encode(keyBuff, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return tls.X509KeyPair(certBuff.Bytes(), keyBuff.Bytes())
}
