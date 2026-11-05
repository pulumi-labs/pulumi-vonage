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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIBaseURL  = "https://api.nexmo.com"
	defaultRESTBaseURL = "https://rest.nexmo.com"

	// maxRetries bounds automatic retries for transient failures.
	maxRetries = 4
	// maxResponseBytes caps how much of a response body is read.
	maxResponseBytes = 8 << 20
)

// retryBaseDelay is the initial backoff between retries. It is a variable so
// that the mocked lifecycle tests can shrink it.
var retryBaseDelay = 500 * time.Millisecond

// apiError is returned for any non-2xx response from the Vonage APIs.
type apiError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *apiError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 500 {
		body = body[:500] + "..."
	}
	return fmt.Sprintf("vonage: %s %s returned HTTP %d: %s", e.Method, e.Path, e.Status, body)
}

// isNotFound reports whether err is an HTTP 404 from the Vonage API.
func isNotFound(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

// isDefinitiveRefusal reports whether err proves the request was not applied: an HTTP
// 4xx, or an error-code refusal in the legacy number API envelope. Transport errors,
// unreadable responses and 5xx leave the outcome unknown.
func isDefinitiveRefusal(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status >= 400 && ae.Status < 500
	}
	var ne *numberStatusError
	return errors.As(err, &ne)
}

// Client is a small HTTP client for the Vonage account-management APIs. All
// operations the provider performs authenticate with HTTP Basic (api key and
// api secret).
type Client struct {
	apiKey      string
	apiSecret   string
	apiBaseURL  string
	restBaseURL string
	http        *http.Client
	userAgent   string
}

// newClient builds a Client. Empty base URLs fall back to the public hosts.
func newClient(apiKey, apiSecret, apiBaseURL, restBaseURL string) *Client {
	if apiBaseURL == "" {
		apiBaseURL = defaultAPIBaseURL
	}
	if restBaseURL == "" {
		restBaseURL = defaultRESTBaseURL
	}
	return &Client{
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		apiBaseURL:  strings.TrimRight(apiBaseURL, "/"),
		restBaseURL: strings.TrimRight(restBaseURL, "/"),
		http:        &http.Client{Timeout: 60 * time.Second},
		userAgent:   "pulumi-vonage",
	}
}

// do performs one logical request with bounded retries. Requests are retried
// on HTTP 429 (never processed) for every method, and additionally on 5xx and
// transport errors for idempotent methods (GET, PUT, DELETE). POST and PATCH
// are never replayed after a 5xx or transport error because the first attempt
// may have been applied (for example a number purchase, which is billed).
func (c *Client) do(ctx context.Context, method, base, path string, query url.Values, contentType string, body []byte, out any) error {
	full := base + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	idempotent := method == http.MethodGet || method == http.MethodPut || method == http.MethodDelete
	delay := retryBaseDelay
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, full, rdr)
		if err != nil {
			return err
		}
		req.SetBasicAuth(c.apiKey, c.apiSecret)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = fmt.Errorf("vonage: %s %s: %w", method, path, err)
			if idempotent {
				continue
			}
			return lastErr
		}
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
		if rerr != nil {
			lastErr = fmt.Errorf("vonage: %s %s: reading response: %w", method, path, rerr)
			if idempotent {
				continue
			}
			return lastErr
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = &apiError{Method: method, Path: path, Status: resp.StatusCode, Body: string(raw)}
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, perr := strconv.Atoi(ra); perr == nil && secs >= 0 && secs <= 30 {
					delay = time.Duration(secs) * time.Second
				}
			}
			continue
		}
		if resp.StatusCode >= 500 && idempotent {
			lastErr = &apiError{Method: method, Path: path, Status: resp.StatusCode, Body: string(raw)}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return &apiError{Method: method, Path: path, Status: resp.StatusCode, Body: string(raw)}
		}
		if out == nil || len(bytes.TrimSpace(raw)) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("vonage: %s %s: decoding response: %w", method, path, err)
		}
		return nil
	}
	return lastErr
}

func (c *Client) apiJSON(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var body []byte
	ct := ""
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body, ct = b, "application/json"
	}
	return c.do(ctx, method, c.apiBaseURL, path, query, ct, body, out)
}

func (c *Client) restForm(ctx context.Context, method, path string, query, form url.Values, out any) error {
	var body []byte
	ct := ""
	if form != nil {
		body, ct = []byte(form.Encode()), "application/x-www-form-urlencoded"
	}
	return c.do(ctx, method, c.restBaseURL, path, query, ct, body, out)
}
