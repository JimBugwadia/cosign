//
// Copyright 2021 The Sigstore Authors.
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

package fulcioroots

import (
	"bytes"
	"context"
	"crypto/x509"
	"os"
	"sync"

	"github.com/pkg/errors"
	"github.com/sigstore/cosign/pkg/cosign/tuf"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
)

var (
	rootsOnce     sync.Once
	roots         *x509.CertPool
	intermediates *x509.CertPool
)

// This is the root in the fulcio project.
var fulcioTargetStr = `fulcio.crt.pem`

// This is the v1 migrated root.
var fulcioV1TargetStr = `fulcio_v1.crt.pem`

// The untrusted intermediate CA certificate, used for chain building
// TODO: Remove once this is bundled in TUF metadata.
var fulcioIntermediateV1 = `-----BEGIN CERTIFICATE-----
MIICGjCCAaGgAwIBAgIUALnViVfnU0brJasmRkHrn/UnfaQwCgYIKoZIzj0EAwMw
KjEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MREwDwYDVQQDEwhzaWdzdG9yZTAeFw0y
MjA0MTMyMDA2MTVaFw0zMTEwMDUxMzU2NThaMDcxFTATBgNVBAoTDHNpZ3N0b3Jl
LmRldjEeMBwGA1UEAxMVc2lnc3RvcmUtaW50ZXJtZWRpYXRlMHYwEAYHKoZIzj0C
AQYFK4EEACIDYgAE8RVS/ysH+NOvuDZyPIZtilgUF9NlarYpAd9HP1vBBH1U5CV7
7LSS7s0ZiH4nE7Hv7ptS6LvvR/STk798LVgMzLlJ4HeIfF3tHSaexLcYpSASr1kS
0N/RgBJz/9jWCiXno3sweTAOBgNVHQ8BAf8EBAMCAQYwEwYDVR0lBAwwCgYIKwYB
BQUHAwMwEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQU39Ppz1YkEZb5qNjp
KFWixi4YZD8wHwYDVR0jBBgwFoAUWMAeX5FFpWapesyQoZMi0CrFxfowCgYIKoZI
zj0EAwMDZwAwZAIwPCsQK4DYiZYDPIaDi5HFKnfxXx6ASSVmERfsynYBiX2X6SJR
nZU84/9DZdnFvvxmAjBOt6QpBlc4J/0DxvkTCqpclvziL6BCCPnjdlIB3Pu3BxsP
mygUY7Ii2zbdCdliiow=
-----END CERTIFICATE-----`

const (
	altRoot = "SIGSTORE_ROOT_FILE"
)

func Get() *x509.CertPool {
	rootsOnce.Do(func() {
		var err error
		roots, intermediates, err = initRoots()
		if err != nil {
			panic(err)
		}
	})
	return roots
}

func GetIntermediates() *x509.CertPool {
	rootsOnce.Do(func() {
		var err error
		roots, intermediates, err = initRoots()
		if err != nil {
			panic(err)
		}
	})
	return intermediates
}

func initRoots() (*x509.CertPool, *x509.CertPool, error) {
	rootPool := x509.NewCertPool()
	intermediatePool := x509.NewCertPool()

	rootEnv := os.Getenv(altRoot)
	if rootEnv != "" {
		raw, err := os.ReadFile(rootEnv)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error reading root PEM file")
		}
		certs, err := cryptoutils.UnmarshalCertificatesFromPEM(raw)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error unmarshalling certificates")
		}
		for _, cert := range certs {
			// root certificates are self-signed
			if bytes.Equal(cert.RawSubject, cert.RawIssuer) {
				rootPool.AddCert(cert)
			} else {
				intermediatePool.AddCert(cert)
			}
		}
	} else {
		tufClient, err := tuf.NewFromEnv(context.Background())
		if err != nil {
			return nil, nil, errors.Wrap(err, "initializing tuf")
		}
		defer tufClient.Close()
		// Retrieve from the embedded or cached TUF root. If expired, a network
		// call is made to update the root.
		targets, err := tufClient.GetTargetsByMeta(tuf.Fulcio, []string{fulcioTargetStr, fulcioV1TargetStr})
		if err != nil {
			return nil, nil, errors.New("error getting targets")
		}
		if len(targets) == 0 {
			return nil, nil, errors.New("none of the Fulcio roots have been found")
		}
		for _, t := range targets {
			certs, err := cryptoutils.UnmarshalCertificatesFromPEM(t.Target)
			if err != nil {
				return nil, nil, errors.Wrap(err, "error unmarshalling certificates")
			}
			for _, cert := range certs {
				// root certificates are self-signed
				if bytes.Equal(cert.RawSubject, cert.RawIssuer) {
					rootPool.AddCert(cert)
				} else {
					intermediatePool.AddCert(cert)
				}
			}
		}
		intermediatePool.AppendCertsFromPEM([]byte(fulcioIntermediateV1))
	}
	return rootPool, intermediatePool, nil
}
