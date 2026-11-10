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
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// Config holds the provider-level configuration.
//
// Most resources talk to the Vonage account-management APIs (Application API v2,
// Numbers, Subaccounts, API Secrets, Verify and Number Pools), which authenticate
// with the account API key and API secret. The User resource and the getUser and
// getUsers functions call the Users API, which only accepts a JWT signed with the
// private key of a Vonage application, so they need ApplicationID and PrivateKey
// and do not need the API key and secret. Every credential is optional at the
// provider level; each resource reports exactly which ones it is missing.
type Config struct {
	// APIKey is the Vonage account API key.
	APIKey *string `pulumi:"apiKey,optional"`

	// APISecret is the Vonage account API secret.
	APISecret *string `pulumi:"apiSecret,optional" provider:"secret"`

	// ApplicationID is the optional application ID used for JWT-authenticated APIs.
	ApplicationID *string `pulumi:"applicationId,optional"`

	// PrivateKey is the optional PEM private key paired with ApplicationID.
	PrivateKey *string `pulumi:"privateKey,optional" provider:"secret"`

	// APIBaseURL overrides https://api.nexmo.com (Application, Subaccounts and Secrets APIs).
	APIBaseURL *string `pulumi:"apiBaseUrl,optional"`

	// RestBaseURL overrides https://rest.nexmo.com (Numbers API).
	RestBaseURL *string `pulumi:"restBaseUrl,optional"`
}

// Annotate describes the provider config fields and wires their environment
// variable fallbacks; infer reads env defaults only from SetDefault, not from
// struct tags.
func (c *Config) Annotate(a infer.Annotator) {
	a.SetDefault(&c.APIKey, nil, "VONAGE_API_KEY")
	a.SetDefault(&c.APISecret, nil, "VONAGE_API_SECRET")
	a.SetDefault(&c.ApplicationID, nil, "VONAGE_APPLICATION_ID")
	a.SetDefault(&c.PrivateKey, nil, "VONAGE_PRIVATE_KEY")
	a.SetDefault(&c.APIBaseURL, nil, "VONAGE_API_BASE_URL")
	a.SetDefault(&c.RestBaseURL, nil, "VONAGE_REST_BASE_URL")
	a.Describe(&c.APIKey, "The Vonage account API key (the 8-character key shown in the Vonage API Dashboard). "+
		"Falls back to the VONAGE_API_KEY environment variable.")
	a.Describe(&c.APISecret, "The Vonage account API secret. Falls back to the VONAGE_API_SECRET environment variable.")
	a.Describe(&c.ApplicationID, "Vonage application ID used to sign the JWT for the Users API (the User resource and the getUser and getUsers functions). "+
		"Users belong to this application. Falls back to VONAGE_APPLICATION_ID.")
	a.Describe(&c.PrivateKey, "PEM private key of the application named by applicationId (PKCS#8 'BEGIN PRIVATE KEY' as downloaded from the dashboard, or PKCS#1). "+
		"Newlines flattened to a literal backslash-n are accepted. Falls back to VONAGE_PRIVATE_KEY.")
	a.Describe(&c.APIBaseURL, "Override for the Vonage API host (default https://api.nexmo.com). "+
		"Intended for testing against a mock server. Falls back to VONAGE_API_BASE_URL.")
	a.Describe(&c.RestBaseURL, "Override for the Vonage REST host used by the Numbers API (default https://rest.nexmo.com). "+
		"Intended for testing against a mock server. Falls back to VONAGE_REST_BASE_URL.")
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// clientFor builds a Client from the provider configuration in ctx.
func clientFor(ctx context.Context) (*Client, error) {
	cfg := infer.GetConfig[Config](ctx)
	return clientFromConfig(&cfg)
}

func clientFromConfig(cfg *Config) (*Client, error) {
	key, secret := deref(cfg.APIKey), deref(cfg.APISecret)
	if key == "" {
		return nil, fmt.Errorf("vonage: apiKey is required (set the provider apiKey or VONAGE_API_KEY)")
	}
	if secret == "" {
		return nil, fmt.Errorf("vonage: apiSecret is required (set the provider apiSecret or VONAGE_API_SECRET)")
	}
	for name, u := range map[string]string{"apiBaseUrl": deref(cfg.APIBaseURL), "restBaseUrl": deref(cfg.RestBaseURL)} {
		if err := checkBaseURL(u); err != nil {
			return nil, fmt.Errorf("vonage: %s: %w", name, err)
		}
	}
	return newClient(key, secret, deref(cfg.APIBaseURL), deref(cfg.RestBaseURL)), nil
}

// errNoJWTCredentials is returned when a Client without JWT credentials is asked to call the Users API.
var errNoJWTCredentials = errors.New("no application JWT credentials configured")

// jwtClientFor builds a Client for the JWT-authenticated Users API from the provider
// configuration in ctx. It needs applicationId and privateKey only.
func jwtClientFor(ctx context.Context) (*Client, error) {
	cfg := infer.GetConfig[Config](ctx)
	return jwtClientFromConfig(&cfg)
}

func jwtClientFromConfig(cfg *Config) (*Client, error) {
	appID, key := deref(cfg.ApplicationID), deref(cfg.PrivateKey)
	if appID == "" {
		return nil, fmt.Errorf("vonage: applicationId is required for the Users API (set the provider applicationId or VONAGE_APPLICATION_ID)")
	}
	if key == "" {
		return nil, fmt.Errorf("vonage: privateKey is required for the Users API (set the provider privateKey or VONAGE_PRIVATE_KEY)")
	}
	if err := checkBaseURL(deref(cfg.APIBaseURL)); err != nil {
		return nil, fmt.Errorf("vonage: apiBaseUrl: %w", err)
	}
	signer, err := newJWTSigner(appID, key)
	if err != nil {
		return nil, fmt.Errorf("vonage: privateKey: %w", err)
	}
	c := newClient("", "", deref(cfg.APIBaseURL), deref(cfg.RestBaseURL))
	c.signer = signer
	return c, nil
}

// providerAPIKey returns the configured account API key (empty if unset).
func providerAPIKey(ctx context.Context) string {
	cfg := infer.GetConfig[Config](ctx)
	return deref(cfg.APIKey)
}

// checkBaseURL refuses a plain-HTTP base URL because every request carries the
// account key and secret as HTTP Basic credentials. Loopback hosts are allowed
// so the provider can be pointed at a local fake.
func checkBaseURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return nil
		}
	}
	return fmt.Errorf("%q must use https (credentials are sent with every request); plain http is only accepted for loopback hosts", raw)
}
