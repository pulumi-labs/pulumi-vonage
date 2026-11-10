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
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// serveExtAccounts is the fake External Accounts API (spec v0.1.6). Called with the mock's lock held.
func (m *mockVonage) serveExtAccounts(w http.ResponseWriter, r *http.Request, body []byte) {
	if m.extAccounts == nil {
		m.extAccounts = map[string]map[string]map[string]any{}
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/beta/chatapp-accounts"), "/")
	parts := []string{}
	if rest != "" {
		parts = strings.Split(rest, "/")
	}
	if len(parts) == 0 {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		want := r.URL.Query().Get("provider")
		all := []map[string]any{}
		chans := []string{}
		for ch := range m.extAccounts {
			chans = append(chans, ch)
		}
		sort.Strings(chans)
		for _, ch := range chans {
			if want != "" && want != ch {
				continue
			}
			ids := []string{}
			for id := range m.extAccounts[ch] {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				all = append(all, m.extAccounts[ch][id])
			}
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if m.extPageLimit > 0 && (size == 0 || size > m.extPageLimit) {
			size = m.extPageLimit
		}
		if size <= 0 {
			size = 20
		}
		pg, _ := strconv.Atoi(r.URL.Query().Get("page_number"))
		if pg < 1 {
			pg = 1
		}
		lo, hi := (pg-1)*size, pg*size
		if lo > len(all) {
			lo = len(all)
		}
		if hi > len(all) {
			hi = len(all)
		}
		out := map[string]any{"page_number": pg, "page_size": size, "_embedded": all[lo:hi]}
		if !m.extNoLinks {
			links := map[string]any{"self": map[string]any{"href": "x"}}
			if hi < len(all) {
				links["next"] = map[string]any{"href": "next"}
			}
			out["_links"] = links
		}
		writeJSON(w, 200, out)
		return
	}
	channel := parts[0]
	if channel == "messenger" && len(parts) == 1 && r.Method == "POST" {
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		id, _ := in["external_id"].(string)
		if m.extAccounts[channel][id] != nil {
			writeJSON(w, 400, map[string]any{"title": "account exists"})
			return
		}
		if m.extAccounts[channel] == nil {
			m.extAccounts[channel] = map[string]map[string]any{}
		}
		acct := map[string]any{"external_id": id, "api_key": "key12345", "provider": channel, "access_token": in["access_token"]}
		if n, ok := in["name"]; ok {
			acct["name"] = n
		}
		m.extAccounts[channel][id] = acct
		writeJSON(w, 201, acct)
		return
	}
	if len(parts) < 2 {
		writeJSON(w, 404, map[string]any{"title": "bad path"})
		return
	}
	acct := m.extAccounts[channel][parts[1]]
	if acct == nil {
		writeJSON(w, 404, map[string]any{"title": "not found"})
		return
	}
	apps, _ := acct["applications"].([]any)
	switch {
	case len(parts) == 2 && r.Method == "GET":
		writeJSON(w, 200, acct)
	case len(parts) == 2 && r.Method == "PATCH" && channel == "messenger":
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		for k, v := range in {
			acct[k] = v
		}
		writeJSON(w, 200, acct)
	case len(parts) == 2 && r.Method == "DELETE" && channel == "messenger":
		delete(m.extAccounts[channel], parts[1])
		w.WriteHeader(204)
	case len(parts) == 3 && parts[2] == "applications" && r.Method == "POST":
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		if len(apps) >= 1 {
			writeJSON(w, 409, map[string]any{"title": "Application conflict"})
			return
		}
		acct["applications"] = []any{in["application"]}
		writeJSON(w, 200, acct)
	case len(parts) == 4 && parts[2] == "applications" && r.Method == "DELETE":
		if len(apps) == 0 || apps[0] != parts[3] {
			writeJSON(w, 409, map[string]any{"title": "Application conflict"})
			return
		}
		acct["applications"] = []any{}
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}
