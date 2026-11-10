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

// servePools is the fake Number Pools API (spec v2.0.4). Called with the mock's lock held.
func (m *mockVonage) servePools(w http.ResponseWriter, r *http.Request, body []byte) {
	if m.pools == nil {
		m.pools = map[string]map[string]any{}
		m.poolNumbers = map[string][]string{}
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v2/numberpools/accounts/"), "/")
	// parts: acct, "pools", [poolID, ["numbers", [number|"delete"]]]
	if len(parts) < 2 || parts[0] != "key12345" || parts[1] != "pools" {
		writeJSON(w, 404, map[string]any{"title": "bad path"})
		return
	}
	page := func(items []any, key string) map[string]any {
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if m.poolPageLimit > 0 && (size == 0 || size > m.poolPageLimit) {
			size = m.poolPageLimit
		}
		if size <= 0 {
			size = 10
		}
		pg, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if pg < 1 {
			pg = 1
		}
		pages := (len(items) + size - 1) / size
		lo, hi := (pg-1)*size, pg*size
		if lo > len(items) {
			lo = len(items)
		}
		if hi > len(items) {
			hi = len(items)
		}
		out := map[string]any{"page": pg, "page_size": size, "total_items": len(items),
			"_embedded": map[string]any{key: append([]any{}, items[lo:hi]...)}}
		if !m.poolOmitTotal {
			out["total_pages"] = pages
		}
		return out
	}
	if len(parts) == 2 {
		switch r.Method {
		case "POST":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			id, _ := in["pool_id"].(string)
			if _, ok := m.pools[id]; ok {
				writeJSON(w, 409, map[string]any{"title": "Pool id already exists"})
				return
			}
			in["account_id"] = "key12345"
			m.pools[id] = in
			writeJSON(w, 201, in)
		case "GET":
			ids := []string{}
			for id := range m.pools {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			items := []any{}
			for _, id := range ids {
				items = append(items, m.pools[id])
			}
			writeJSON(w, 200, page(items, "pools"))
		default:
			w.WriteHeader(405)
		}
		return
	}
	id := parts[2]
	pool, ok := m.pools[id]
	if !ok {
		writeJSON(w, 404, map[string]any{"title": "Pool not found"})
		return
	}
	if len(parts) == 3 {
		switch r.Method {
		case "GET":
			writeJSON(w, 200, pool)
		case "PATCH":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			for k, v := range in {
				pool[k] = v
			}
			writeJSON(w, 201, pool)
		case "DELETE":
			delete(m.pools, id)
			delete(m.poolNumbers, id)
			writeJSON(w, 200, map[string]any{})
		default:
			w.WriteHeader(405)
		}
		return
	}
	if parts[3] != "numbers" {
		writeJSON(w, 404, map[string]any{"title": "bad path"})
		return
	}
	switch {
	case len(parts) == 4 && r.Method == "GET":
		items := []any{}
		for _, n := range m.poolNumbers[id] {
			items = append(items, n)
		}
		writeJSON(w, 200, page(items, "numbers"))
	case len(parts) == 4 && r.Method == "POST":
		var in struct {
			Numbers []string `json:"numbers"`
		}
		_ = json.Unmarshal(body, &in)
		m.poolNumbers[id] = append(m.poolNumbers[id], in.Numbers...)
		writeJSON(w, 201, map[string]any{})
	case len(parts) == 5 && r.Method == "DELETE":
		cur := m.poolNumbers[id]
		for i, n := range cur {
			if n == parts[4] {
				m.poolNumbers[id] = append(cur[:i:i], cur[i+1:]...)
				w.WriteHeader(204)
				return
			}
		}
		writeJSON(w, 404, map[string]any{"title": "Number not in pool"})
	default:
		w.WriteHeader(405)
	}
}
