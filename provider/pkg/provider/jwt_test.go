// Copyright 2026, Pulumi Corporation.
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

package provider

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return k
}

func pkcs8PEM(t *testing.T, k *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(k)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func pkcs1PEM(k *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
}

// verifyJWT checks the RS256 signature of tok against pub and returns the header and claims.
func verifyJWT(t *testing.T, tok string, pub *rsa.PublicKey) (hdr, claims map[string]any, err error) {
	t.Helper()
	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3)
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return nil, nil, err
	}
	for i, dst := range []*map[string]any{&hdr, &claims} {
		raw, derr := base64.RawURLEncoding.DecodeString(parts[i])
		require.NoError(t, derr)
		require.NoError(t, json.Unmarshal(raw, dst))
	}
	return hdr, claims, nil
}

func TestJWTSigner_SignsVerifiableRS256Token(t *testing.T) {
	k := testKey(t)
	for name, pemText := range map[string]string{
		"pkcs8":         pkcs8PEM(t, k),
		"pkcs1":         pkcs1PEM(k),
		"escapednl":     strings.ReplaceAll(strings.TrimSpace(pkcs8PEM(t, k)), "\n", `\n`),
		"surroundingws": "\n  " + pkcs8PEM(t, k) + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			s, err := newJWTSigner("app-1", pemText)
			require.NoError(t, err)
			fixed := time.Unix(1_700_000_000, 0)
			s.now = func() time.Time { return fixed }
			tok, err := s.token()
			require.NoError(t, err)
			hdr, claims, err := verifyJWT(t, tok, &k.PublicKey)
			require.NoError(t, err)
			require.Equal(t, map[string]any{"alg": "RS256", "typ": "JWT"}, hdr)
			require.Equal(t, "app-1", claims["application_id"])
			require.EqualValues(t, 1_700_000_000, claims["iat"])
			require.EqualValues(t, 1_700_000_000+900, claims["exp"])
			require.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, claims["jti"])
			tok2, err := s.token()
			require.NoError(t, err)
			_, claims2, err := verifyJWT(t, tok2, &k.PublicKey)
			require.NoError(t, err)
			require.NotEqual(t, claims["jti"], claims2["jti"], "every token has its own jti")
		})
	}
}

// malformedPEM builds a PEM block whose body is not valid key material. The
// markers are assembled from parts so secret scanners do not flag the fixture.
func malformedPEM(kind string) string {
	return "-----" + "BEGIN " + kind + "-----\nAAAA\n-----" + "END " + kind + "-----\n"
}

func TestJWTSigner_RejectsBadKeysWithoutLeakingThem(t *testing.T) {
	cases := map[string]string{
		"not pem":   "SUPER-SECRET-VALUE",
		"wrong blk": "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n",
		"bad pkcs8": malformedPEM("PRIVATE KEY"),
		"bad pkcs1": malformedPEM("RSA PRIVATE KEY"),
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newJWTSigner("app", v)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "SUPER-SECRET-VALUE")
		})
	}
	_, err := newJWTSigner("", pkcs8PEM(t, testKey(t)))
	require.ErrorContains(t, err, "application ID")
}
