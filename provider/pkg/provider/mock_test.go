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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blang/semver"
	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

// mockVonage is a stateful in-memory fake of the Vonage endpoints the provider uses.
type mockVonage struct {
	mu        sync.Mutex
	t         *testing.T
	apps      map[string]map[string]any
	nextApp   int
	numbers   map[string]map[string]any // key country:msisdn
	subs      map[string]map[string]any
	available []map[string]any             // numbers for sale, served by GET /number/search
	secrets   map[string]map[string]string // account -> id -> created
	nextSec   int
	calls     []string
	fail5xx   int // number of upcoming GET /v2/applications calls to fail with 503
	authSeen  bool

	// inject maps "METHOD /path" to an HTTP status returned (without touching state) for every
	// matching request until the entry is deleted.
	inject map[string]int
	// fillDefaults makes the fake behave like Vonage and fill server-side defaults on
	// create and update of applications.
	fillDefaults bool
	// after runs a hook (under the mock's lock) once the matching "METHOD /path" request has
	// been handled, so a test can break the API at an exact point of a multi-step flow.
	after map[string]func()

	// Verify v2 templates: templates by ID and fragments by template ID then fragment ID.
	templates map[string]map[string]any
	fragments map[string]map[string]map[string]any
	nextTpl   int
}

// The fake follows the documented Vonage semantics rather than the provider's assumptions:
//   - GET /v2/applications/{id} never returns messages.version (the field is write-only) nor
//     the private key.
//   - GET /account/numbers never returns voiceStatusCallback.
//   - PUT /v2/applications/{id} replaces the whole configuration.
//   - An account holds at most two API secrets and cannot revoke its last one.

// stripWriteOnly removes the fields the real GET endpoints never return.
func stripWriteOnly(app map[string]any) map[string]any {
	c := map[string]any{}
	for k, v := range app {
		c[k] = v
	}
	if k, ok := c["keys"].(map[string]any); ok {
		c["keys"] = map[string]any{"public_key": k["public_key"]}
	}
	if caps, ok := c["capabilities"].(map[string]any); ok {
		cc := map[string]any{}
		for k, v := range caps {
			cc[k] = v
		}
		if msg, ok := cc["messages"].(map[string]any); ok {
			mm := map[string]any{}
			for k, v := range msg {
				if k != "version" {
					mm[k] = v
				}
			}
			cc["messages"] = mm
		}
		c["capabilities"] = cc
	}
	return c
}

// applyDefaults fills server-side defaults the way Vonage does for omitted optional fields.
func applyDefaults(app map[string]any) {
	caps, _ := app["capabilities"].(map[string]any)
	if v, ok := caps["voice"].(map[string]any); ok {
		if _, ok := v["signed_callbacks"]; !ok {
			v["signed_callbacks"] = true
		}
		if _, ok := v["conversations_ttl"]; !ok {
			v["conversations_ttl"] = float64(48)
		}
		if h, ok := v["webhooks"].(map[string]any); ok {
			for _, hk := range h {
				if hm, ok := hk.(map[string]any); ok {
					if _, ok := hm["http_method"]; !ok {
						hm["http_method"] = "POST"
					}
				}
			}
		}
	}
	if rt, ok := caps["rtc"].(map[string]any); ok {
		if _, ok := rt["signed_callbacks"]; !ok {
			rt["signed_callbacks"] = true
		}
	}
}

func (m *mockVonage) setNumberRaw(country, msisdn string, extra map[string]any) {
	n := map[string]any{"country": country, "msisdn": msisdn, "type": "mobile-lvn", "features": []any{"SMS", "VOICE"}}
	for k, v := range extra {
		n[k] = v
	}
	m.mu.Lock()
	m.numbers[country+":"+msisdn] = n
	m.mu.Unlock()
}

func newMock(t *testing.T) (*mockVonage, *httptest.Server) {
	m := &mockVonage{t: t, apps: map[string]map[string]any{}, numbers: map[string]map[string]any{},
		subs: map[string]map[string]any{}, secrets: map[string]map[string]string{},
		inject: map[string]int{}, after: map[string]func(){}}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return m, srv
}

func (m *mockVonage) callLog() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.calls...)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (m *mockVonage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, p, ok := r.BasicAuth(); ok && u == "key12345" && p == "Secret123" {
		m.authSeen = true
	} else {
		writeJSON(w, 401, map[string]any{"title": "Unauthorized"})
		return
	}
	m.calls = append(m.calls, r.Method+" "+r.URL.Path)
	body, _ := io.ReadAll(r.Body)
	p := r.URL.Path
	if f := m.after[r.Method+" "+p]; f != nil {
		defer f()
	}
	if code, ok := m.inject[r.Method+" "+p]; ok {
		writeJSON(w, code, map[string]any{"title": "injected failure"})
		return
	}
	switch {
	case p == "/v2/applications" && r.Method == "POST":
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		m.nextApp++
		id := fmt.Sprintf("app-%d", m.nextApp)
		in["id"] = id
		keys, _ := in["keys"].(map[string]any)
		if keys == nil {
			keys = map[string]any{"public_key": "PUB-" + id, "private_key": "PRIV-" + id}
		}
		in["keys"] = keys
		if m.fillDefaults {
			applyDefaults(in)
		}
		m.apps[id] = in
		writeJSON(w, 201, in)
	case p == "/v2/applications" && r.Method == "GET":
		if m.fail5xx > 0 {
			m.fail5xx--
			writeJSON(w, 503, map[string]any{"title": "down"})
			return
		}
		all := []any{}
		for i := 1; i <= m.nextApp; i++ {
			if a, ok := m.apps[fmt.Sprintf("app-%d", i)]; ok {
				all = append(all, stripWriteOnly(a))
			}
		}
		size := 100
		if v, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && v > 0 {
			size = v
		}
		page := 1
		if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
			page = v
		}
		pages := (len(all) + size - 1) / size
		lo, hi := (page-1)*size, page*size
		if lo > len(all) {
			lo = len(all)
		}
		if hi > len(all) {
			hi = len(all)
		}
		writeJSON(w, 200, map[string]any{"page_size": size, "page": page, "total_items": len(all), "total_pages": pages,
			"_embedded": map[string]any{"applications": all[lo:hi]}})
	case strings.HasPrefix(p, "/v2/applications/"):
		id := strings.TrimPrefix(p, "/v2/applications/")
		app, ok := m.apps[id]
		if !ok {
			writeJSON(w, 404, map[string]any{"title": "Not Found"})
			return
		}
		switch r.Method {
		case "GET":
			writeJSON(w, 200, stripWriteOnly(app))
		case "PUT":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			in["id"] = id
			if k, ok := in["keys"].(map[string]any); ok && k["public_key"] != nil {
				in["keys"] = map[string]any{"public_key": k["public_key"]}
			} else {
				in["keys"] = map[string]any{"public_key": app["keys"].(map[string]any)["public_key"]}
			}
			if m.fillDefaults {
				applyDefaults(in)
			}
			m.apps[id] = in
			writeJSON(w, 200, stripWriteOnly(in))
		case "DELETE":
			delete(m.apps, id)
			w.WriteHeader(204)
		}
	case p == "/account/get-balance":
		writeJSON(w, 200, map[string]any{"value": 12.5, "autoReload": true})
	case p == "/number/search":
		country := r.URL.Query().Get("country")
		if country == "" {
			writeJSON(w, 400, map[string]any{"error-code": "3", "error-code-label": "Invalid country"})
			return
		}
		list := []any{}
		for _, n := range m.available {
			if n["country"] != country {
				continue
			}
			if t := r.URL.Query().Get("type"); t != "" && n["type"] != t {
				continue
			}
			list = append(list, n)
		}
		total := len(list)
		size, index := 10, 1
		if v, err := strconv.Atoi(r.URL.Query().Get("size")); err == nil && v > 0 {
			size = v
		}
		if v, err := strconv.Atoi(r.URL.Query().Get("index")); err == nil && v > 0 {
			index = v
		}
		lo, hi := (index-1)*size, index*size
		if lo > len(list) {
			lo = len(list)
		}
		if hi > len(list) {
			hi = len(list)
		}
		writeJSON(w, 200, map[string]any{"count": total, "numbers": list[lo:hi]})
	case p == "/accounts/key12345/subaccounts" && r.Method == "GET":
		keys := make([]string, 0, len(m.subs))
		for k := range m.subs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		list := []any{}
		for _, k := range keys {
			list = append(list, m.subs[k])
		}
		writeJSON(w, 200, map[string]any{"_embedded": map[string]any{
			"primary_account": map[string]any{"api_key": "key12345", "name": "primary"}, "subaccounts": list}})
	case p == "/account/numbers":
		keys := []string{}
		for k := range m.numbers {
			if r.URL.Query().Get("country") != "" && !strings.HasPrefix(k, r.URL.Query().Get("country")+":") {
				continue
			}
			app, _ := m.numbers[k]["app_id"].(string)
			if want := r.URL.Query().Get("application_id"); want != "" && app != want {
				continue
			}
			if r.URL.Query().Get("has_application") == "true" && app == "" {
				continue
			}
			if r.URL.Query().Get("has_application") == "false" && app != "" {
				continue
			}
			if strings.Contains(strings.SplitN(k, ":", 2)[1], r.URL.Query().Get("pattern")) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		size, index := 10, 1
		if v, err := strconv.Atoi(r.URL.Query().Get("size")); err == nil && v > 0 {
			size = v
		}
		if v, err := strconv.Atoi(r.URL.Query().Get("index")); err == nil && v > 0 {
			index = v
		}
		list := []any{}
		for i, k := range keys {
			if i < (index-1)*size || i >= index*size {
				continue
			}
			c := map[string]any{}
			for kk, v := range m.numbers[k] {
				if kk != "voiceStatusCallback" { // the real list endpoint does not report it
					c[kk] = v
				}
			}
			list = append(list, c)
		}
		writeJSON(w, 200, map[string]any{"count": len(keys), "numbers": list})
	case p == "/number/buy" || p == "/number/cancel" || p == "/number/update":
		vals, _ := url.ParseQuery(string(body))
		key := vals.Get("country") + ":" + vals.Get("msisdn")
		switch p {
		case "/number/buy":
			if _, ok := m.numbers[key]; ok {
				writeJSON(w, 200, map[string]any{"error-code": "401", "error-code-label": "already owned"})
				return
			}
			m.numbers[key] = map[string]any{"country": vals.Get("country"), "msisdn": vals.Get("msisdn"),
				"type": "mobile-lvn", "features": []string{"SMS", "VOICE"}}
		case "/number/cancel":
			delete(m.numbers, key)
		case "/number/update":
			n, ok := m.numbers[key]
			if !ok {
				writeJSON(w, 200, map[string]any{"error-code": "420", "error-code-label": "not owned"})
				return
			}
			if v, ok := vals["app_id"]; ok {
				n["app_id"] = v[0]
				if v[0] != "" {
					n["voiceCallbackType"], n["voiceCallbackValue"] = "app", v[0]
					n["messagesCallbackType"], n["messagesCallbackValue"] = "app", v[0]
				}
			}
			if v, ok := vals["moHttpUrl"]; ok {
				n["moHttpUrl"] = v[0]
			}
			for _, f := range []string{"voiceCallbackType", "voiceCallbackValue", "voiceStatusCallback"} {
				if v, ok := vals[f]; ok {
					n[f] = v[0]
				}
			}
		}
		writeJSON(w, 200, map[string]any{"error-code": "200", "error-code-label": "success"})
	case p == "/accounts/key12345/subaccounts" && r.Method == "POST":
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		k := fmt.Sprintf("sub%05d", len(m.subs)+1)
		sub := map[string]any{"api_key": k, "name": in["name"], "primary_account_api_key": "key12345",
			"use_primary_account_balance": true, "created_at": "2026-01-01T00:00:00Z", "suspended": false,
			"balance": nil, "credit_limit": nil}
		if v, ok := in["use_primary_account_balance"]; ok {
			sub["use_primary_account_balance"] = v
		}
		m.subs[k] = sub
		out := map[string]any{"secret": "GenSecret1"}
		for kk, v := range sub {
			out[kk] = v
		}
		writeJSON(w, 200, out)
	case strings.HasPrefix(p, "/accounts/key12345/subaccounts/"):
		k := strings.TrimPrefix(p, "/accounts/key12345/subaccounts/")
		sub, ok := m.subs[k]
		if !ok {
			writeJSON(w, 404, map[string]any{"title": "Not Found"})
			return
		}
		if r.Method == "PATCH" {
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			for kk, v := range in {
				sub[kk] = v
			}
		}
		writeJSON(w, 200, sub)
	case strings.HasSuffix(p, "/secrets") && r.Method == "GET":
		acct := strings.Split(p, "/")[2]
		list := []any{}
		ids := []string{}
		for id := range m.secrets[acct] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			list = append(list, map[string]any{"id": id, "created_at": m.secrets[acct][id]})
		}
		writeJSON(w, 200, map[string]any{"_embedded": map[string]any{"secrets": list}})
	case strings.HasSuffix(p, "/secrets") && r.Method == "POST":
		acct := strings.Split(p, "/")[2]
		if m.secrets[acct] == nil {
			m.secrets[acct] = map[string]string{}
		}
		if len(m.secrets[acct]) >= 2 {
			writeJSON(w, 403, map[string]any{"title": "Secret limit reached"})
			return
		}
		m.nextSec++
		id := fmt.Sprintf("sec-%d", m.nextSec)
		m.secrets[acct][id] = "2026-01-01T00:00:00Z"
		writeJSON(w, 200, map[string]any{"id": id, "created_at": "2026-01-01T00:00:00Z"})
	case strings.Contains(p, "/secrets/"):
		parts := strings.Split(p, "/")
		acct, id := parts[2], parts[4]
		created, ok := m.secrets[acct][id]
		if !ok {
			writeJSON(w, 404, map[string]any{"title": "Not Found"})
			return
		}
		if r.Method == "DELETE" {
			if len(m.secrets[acct]) <= 1 {
				writeJSON(w, 403, map[string]any{"title": "cannot delete last secret"})
				return
			}
			delete(m.secrets[acct], id)
			w.WriteHeader(204)
			return
		}
		writeJSON(w, 200, map[string]any{"id": id, "created_at": created})
	case strings.HasPrefix(p, "/v2/verify/templates"):
		m.serveVerify(w, r, body)
	default:
		writeJSON(w, 404, map[string]any{"title": "unhandled " + r.Method + " " + p})
	}
}

func init() { retryBaseDelay = time.Millisecond }

func vonageURN(typ, name string) resource.URN {
	return resource.NewURN("stack", "proj", "", tokens.Type("vonage:index:"+typ), name)
}

func newServerFor(t *testing.T, baseURL string) integration.Server {
	t.Helper()
	prov, err := New()
	require.NoError(t, err)
	s, err := integration.NewServer(t.Context(), "vonage", semver.MustParse("0.0.1"), integration.WithProvider(prov))
	require.NoError(t, err)
	require.NoError(t, s.Configure(pgp.ConfigureRequest{Args: property.NewMap(map[string]property.Value{
		"apiKey":      property.New("key12345"),
		"apiSecret":   property.New("Secret123"),
		"apiBaseUrl":  property.New(baseURL),
		"restBaseUrl": property.New(baseURL),
	})}))
	return s
}

func pm(kv map[string]any) property.Map {
	out := map[string]property.Value{}
	for k, v := range kv {
		out[k] = toValue(v)
	}
	return property.NewMap(out)
}

func toValue(v any) property.Value {
	switch x := v.(type) {
	case map[string]any:
		return property.New(pm(x))
	case []any:
		arr := make([]property.Value, len(x))
		for i, e := range x {
			arr[i] = toValue(e)
		}
		return property.New(property.NewArray(arr))
	case int:
		return property.New(float64(x))
	case string:
		return property.New(x)
	case bool:
		return property.New(x)
	case float64:
		return property.New(x)
	default:
		panic(fmt.Sprintf("unsupported %T", v))
	}
}

func str(m property.Map, k string) string {
	v, ok := m.GetOk(k)
	if !ok || !v.IsString() {
		return ""
	}
	return v.AsString()
}
