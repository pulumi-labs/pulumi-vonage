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
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blang/semver"
	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

// mockUsers is a stateful fake of the Vonage Users API. Every request must carry a Bearer JWT
// whose RS256 signature verifies against the application's public key; account Basic credentials
// are refused, as the real API does.
type mockUsers struct {
	t     *testing.T
	mu    sync.Mutex
	pub   *rsa.PublicKey
	appID string

	users  map[string]map[string]any
	next   int
	pageSz int // when > 0, caps the page size so tests can force several pages
	// brokenCursor makes every listing link to the same cursor.
	brokenCursor bool
	// flaky counts upcoming GET /v1/users/{id} calls to fail with 503.
	flaky      int
	calls      []string
	lastPatch  map[string]any
	lastCreate map[string]any
	jwtSeen    int
}

func (m *mockUsers) authorized(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(h, "Bearer "), ".")
	if len(parts) != 3 {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(m.pub, crypto.SHA256, sum[:], sig) != nil {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		AppID string `json:"application_id"`
		Iat   int64  `json:"iat"`
		Exp   int64  `json:"exp"`
		JTI   string `json:"jti"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return false
	}
	now := time.Now().Unix()
	m.jwtSeen++
	return claims.AppID == m.appID && claims.JTI != "" && claims.Iat <= now+5 && claims.Exp > now
}

func (m *mockUsers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.authorized(r) {
		writeJSON(w, 401, map[string]any{"title": "Unauthorized"})
		return
	}
	m.calls = append(m.calls, r.Method+" "+r.URL.Path)
	body, _ := io.ReadAll(r.Body)
	notFound := func() {
		writeJSON(w, 404, map[string]any{"title": "Not found.", "detail": "User does not exist, or you do not have access.",
			"type": "https://developer.vonage.com/api/conversation#user:error:not-found"})
	}
	if r.URL.Path == "/v1/users" {
		switch r.Method {
		case "POST":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			if in == nil {
				in = map[string]any{}
			}
			m.lastCreate = clone(in)
			if n, _ := in["name"].(string); n != "" {
				for _, u := range m.users {
					if u["name"] == n {
						writeJSON(w, 409, map[string]any{"title": "Conflict", "detail": "name already in use"})
						return
					}
				}
			}
			m.next++
			id := fmt.Sprintf("USR-%08d", m.next)
			in["id"] = id
			if in["name"] == nil {
				in["name"] = fmt.Sprintf("NAM-%08d", m.next)
			}
			in["_links"] = map[string]any{"self": map[string]any{"href": "https://api.nexmo.com/v1/users/" + id}}
			m.users[id] = in
			writeJSON(w, 201, in)
		case "GET":
			ids := []string{}
			for id := range m.users {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
			if m.pageSz > 0 && (size == 0 || size > m.pageSz) {
				size = m.pageSz
			}
			if size == 0 {
				size = 10
			}
			filter := r.URL.Query().Get("name")
			items := []any{}
			for _, id := range ids {
				if filter != "" && m.users[id]["name"] != filter {
					continue
				}
				u := m.users[id]
				items = append(items, map[string]any{"id": id, "name": u["name"], "display_name": u["display_name"]})
			}
			start := 0
			if c := r.URL.Query().Get("cursor"); c != "" {
				start, _ = strconv.Atoi(c)
			}
			end := start + size
			if end > len(items) {
				end = len(items)
			}
			if start > len(items) {
				start = len(items)
			}
			out := map[string]any{"page_size": size, "_embedded": map[string]any{"users": items[start:end]}}
			if end < len(items) {
				c := strconv.Itoa(end)
				if m.brokenCursor {
					c = "same"
				}
				out["_links"] = map[string]any{"next": map[string]any{"href": "https://api.nexmo.com/v1/users?page_size=" + strconv.Itoa(size) + "&cursor=" + c}}
			}
			writeJSON(w, 200, out)
		default:
			w.WriteHeader(405)
		}
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/users/")
	u, ok := m.users[id]
	if !ok {
		notFound()
		return
	}
	switch r.Method {
	case "GET":
		if m.flaky > 0 {
			m.flaky--
			writeJSON(w, 503, map[string]any{"title": "down"})
			return
		}
		out := clone(u)
		// The API never returns SIP passwords.
		if ch, ok := out["channels"].(map[string]any); ok {
			if sips, ok := ch["sip"].([]any); ok {
				for _, s := range sips {
					delete(s.(map[string]any), "password")
				}
			}
		}
		writeJSON(w, 200, out)
	case "PATCH":
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		m.lastPatch = clone(in)
		for k, v := range in {
			switch k {
			case "channels":
				ch, _ := u["channels"].(map[string]any)
				if ch == nil {
					ch = map[string]any{}
				}
				for ck, cv := range v.(map[string]any) {
					if l, _ := cv.([]any); len(l) == 0 {
						delete(ch, ck)
					} else {
						ch[ck] = cv
					}
				}
				u["channels"] = ch
			default:
				if v == nil {
					delete(u, k)
				} else {
					u[k] = v
				}
			}
		}
		writeJSON(w, 200, u)
	case "DELETE":
		delete(m.users, id)
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}

func (m *mockUsers) mutate(f func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f()
}

func (m *mockUsers) callCount(c string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, x := range m.calls {
		if x == c {
			n++
		}
	}
	return n
}

// newUserLC starts a fake Users API and a provider configured with only applicationId and
// privateKey (no apiKey or apiSecret).
func newUserLC(t *testing.T, name string) (*vlc, *mockUsers) {
	t.Helper()
	k := testKey(t)
	m := &mockUsers{t: t, pub: &k.PublicKey, appID: "app-1234", users: map[string]map[string]any{}}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	s := newJWTServer(t, map[string]property.Value{
		"applicationId": property.New("app-1234"),
		"privateKey":    property.New(pkcs8PEM(t, k)),
		"apiBaseUrl":    property.New(srv.URL),
	})
	return &vlc{t: t, s: s, urn: vonageURN("User", name), srvURL: srv.URL}, m
}

func newJWTServer(t *testing.T, cfg map[string]property.Value) integration.Server {
	t.Helper()
	prov, err := New()
	require.NoError(t, err)
	s, err := integration.NewServer(t.Context(), "vonage", semver.MustParse("0.0.1"), integration.WithProvider(prov))
	require.NoError(t, err)
	require.NoError(t, s.Configure(pgp.ConfigureRequest{Args: property.NewMap(cfg)}))
	return s
}
