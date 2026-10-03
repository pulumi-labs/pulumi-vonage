package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	mu       sync.Mutex
	t        *testing.T
	apps     map[string]map[string]any
	nextApp  int
	numbers  map[string]map[string]any // key country:msisdn
	subs     map[string]map[string]any
	secrets  map[string]map[string]string // account -> id -> created
	nextSec  int
	calls    []string
	fail5xx  int // number of upcoming GET /v2/applications calls to fail with 503
	authSeen bool
}

func newMock(t *testing.T) (*mockVonage, *httptest.Server) {
	m := &mockVonage{t: t, apps: map[string]map[string]any{}, numbers: map[string]map[string]any{},
		subs: map[string]map[string]any{}, secrets: map[string]map[string]string{}}
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
		m.apps[id] = in
		writeJSON(w, 201, in)
	case p == "/v2/applications" && r.Method == "GET":
		if m.fail5xx > 0 {
			m.fail5xx--
			writeJSON(w, 503, map[string]any{"title": "down"})
			return
		}
		list := []any{}
		for i := 1; i <= m.nextApp; i++ {
			if a, ok := m.apps[fmt.Sprintf("app-%d", i)]; ok {
				c := map[string]any{}
				for k, v := range a {
					c[k] = v
				}
				if k, ok := c["keys"].(map[string]any); ok {
					c["keys"] = map[string]any{"public_key": k["public_key"]}
				}
				list = append(list, c)
			}
		}
		writeJSON(w, 200, map[string]any{"page_size": 100, "page": 1, "total_items": len(list), "total_pages": 1,
			"_embedded": map[string]any{"applications": list}})
	case strings.HasPrefix(p, "/v2/applications/"):
		id := strings.TrimPrefix(p, "/v2/applications/")
		app, ok := m.apps[id]
		if !ok {
			writeJSON(w, 404, map[string]any{"title": "Not Found"})
			return
		}
		switch r.Method {
		case "GET":
			c := map[string]any{}
			for k, v := range app {
				c[k] = v
			}
			if k, ok := c["keys"].(map[string]any); ok {
				c["keys"] = map[string]any{"public_key": k["public_key"]}
			}
			writeJSON(w, 200, c)
		case "PUT":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			in["id"] = id
			in["keys"] = map[string]any{"public_key": app["keys"].(map[string]any)["public_key"]}
			m.apps[id] = in
			writeJSON(w, 200, in)
		case "DELETE":
			delete(m.apps, id)
			w.WriteHeader(204)
		}
	case p == "/account/numbers":
		list := []any{}
		for k, n := range m.numbers {
			if strings.HasSuffix(k, ":"+r.URL.Query().Get("pattern")) || r.URL.Query().Get("pattern") == "" {
				list = append(list, n)
			}
		}
		writeJSON(w, 200, map[string]any{"count": len(list), "numbers": list})
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
