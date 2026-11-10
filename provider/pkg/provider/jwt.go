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
	"errors"
	"fmt"
	"strings"
	"time"
)

// jwtLifetime is how long a signed token is valid. Vonage accepts up to 24 hours; a short
// lifetime limits what a leaked log line is worth, and a fresh token is signed for every
// request attempt, so a long retry never outlives it.
const jwtLifetime = 15 * time.Minute

// jwtSigner signs the RS256 application JWTs that authenticate the Vonage runtime APIs
// (Users, Conversations, Voice, Messages, Video). The claims are the ones the Vonage SDKs send:
// application_id, iat, jti and exp.
type jwtSigner struct {
	appID string
	key   *rsa.PrivateKey
	now   func() time.Time
}

// newJWTSigner parses a PEM private key (PKCS#8 "PRIVATE KEY", as downloaded from the Vonage
// dashboard and returned by Application, or PKCS#1 "RSA PRIVATE KEY"). A value whose newlines
// were flattened to the two characters backslash-n, as happens in many env-file and CI
// secret stores, is accepted. Error messages never contain key material.
func newJWTSigner(appID, privateKey string) (*jwtSigner, error) {
	if strings.TrimSpace(appID) == "" {
		return nil, errors.New("application ID is empty")
	}
	pemText := strings.TrimSpace(privateKey)
	if strings.Contains(pemText, `\n`) && !strings.Contains(pemText, "\n") {
		pemText = strings.ReplaceAll(pemText, `\n`, "\n")
	}
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("private key is not PEM encoded (expected a -----BEGIN PRIVATE KEY----- block)")
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.New("private key is not a valid PKCS#1 RSA key")
		}
		key = k
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.New("private key is not a valid PKCS#8 key")
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not an RSA key (Vonage application keys are RSA)")
		}
		key = rk
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q (expected PRIVATE KEY or RSA PRIVATE KEY)", block.Type)
	}
	return &jwtSigner{appID: appID, key: key, now: time.Now}, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// newJTI returns a random RFC 4122 version 4 UUID for the jti claim.
func newJTI() (string, error) {
	var u [16]byte
	if _, err := rand.Read(u[:]); err != nil {
		return "", err
	}
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16]), nil
}

// token returns a freshly signed JWT.
func (s *jwtSigner) token() (string, error) {
	jti, err := newJTI()
	if err != nil {
		return "", fmt.Errorf("generating jti: %w", err)
	}
	iat := s.now().Unix()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"application_id": s.appID,
		"iat":            iat,
		"jti":            jti,
		"exp":            iat + int64(jwtLifetime/time.Second),
	})
	signingInput := b64(header) + "." + b64(claims)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}
	return signingInput + "." + b64(sig), nil
}
