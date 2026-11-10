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
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// serveNetworkRegistry is the fake Network Registry API (spec v1.0.0). Called with the mock's
// lock held. Business profiles delete softly (delete_at), accept changes only as change
// requests, and listings page with an opaque next token.
func (m *mockVonage) serveNetworkRegistry(w http.ResponseWriter, r *http.Request, body []byte) {
	if m.nrProfiles == nil {
		m.nrProfiles = map[string]map[string]any{}
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/network-registry/")
	parts := strings.Split(rest, "/")
	page := func(items []any, key string) {
		size := m.nrPageLimit
		if size <= 0 {
			size = 1000
		}
		if l, _ := strconv.Atoi(r.URL.Query().Get("limit")); l > 0 && l < size {
			size = l
		}
		start := 0
		if tok := r.URL.Query().Get("next"); tok != "" {
			start, _ = strconv.Atoi(strings.TrimPrefix(tok, "tok"))
		}
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		links := map[string]any{"self": map[string]any{"href": "https://x" + r.URL.Path}}
		if end < len(items) && !m.nrNoNextToken {
			q := url.Values{"next": {fmt.Sprintf("tok%d", end)}}
			links["next"] = map[string]any{"href": "https://x" + r.URL.Path + "?" + q.Encode()}
		} else if end < len(items) {
			links["next"] = map[string]any{"href": "https://x" + r.URL.Path}
		}
		writeJSON(w, 200, map[string]any{"_links": links, "_embedded": map[string]any{key: append([]any{}, items[start:end]...)}})
	}
	switch {
	case parts[0] == "catalogue" && r.Method == "GET":
		name := strings.Join(parts[1:], "/")
		if len(parts) == 3 {
			for _, it := range m.nrCatalogue[parts[1]] {
				if mm, _ := it.(map[string]any); mm["id"] == parts[2] {
					writeJSON(w, 200, mm)
					return
				}
			}
			writeJSON(w, 404, map[string]any{"title": "not found"})
			return
		}
		m.nrCatalogueQueries = append(m.nrCatalogueQueries, name+"?"+r.URL.Query().Encode())
		key := strings.ReplaceAll(parts[1], "-", "_")
		page(m.nrCatalogue[parts[1]], key)
	case parts[0] == "business-profiles" && len(parts) == 1:
		switch r.Method {
		case "POST":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			for _, req := range []string{"countries", "contact_role", "name", "organisation_type", "tax_number", "privacy_policy_url", "privacy_policy_manager_email", "business_information"} {
				if in[req] == nil {
					writeJSON(w, 400, map[string]any{"title": "missing " + req})
					return
				}
			}
			m.nrNext++
			id := fmt.Sprintf("bp-%d", m.nrNext)
			in["id"], in["account_id"] = id, "key12345"
			in["status"] = map[string]any{"status": "pending"}
			in["created_at"], in["updated_at"] = "2026-11-10T00:00:00Z", "2026-11-10T00:00:00Z"
			m.nrProfiles[id] = in
			writeJSON(w, 201, in)
		case "GET":
			ids := []string{}
			for id := range m.nrProfiles {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			items := []any{}
			for _, id := range ids {
				if m.nrShared == (r.URL.Query().Get("shared") == "true") {
					items = append(items, m.nrProfiles[id])
				}
			}
			page(items, "business_profiles")
		default:
			w.WriteHeader(405)
		}
	case parts[0] == "business-profiles" && len(parts) >= 2:
		prof, ok := m.nrProfiles[parts[1]]
		if !ok {
			writeJSON(w, 404, map[string]any{"title": "Business profile not found"})
			return
		}
		switch {
		case len(parts) == 2 && r.Method == "GET":
			writeJSON(w, 200, prof)
		case len(parts) == 2 && r.Method == "DELETE":
			prof["delete_at"] = "2026-12-10T00:00:00Z"
			w.WriteHeader(202)
		case len(parts) == 3 && parts[2] == "change-request" && r.Method == "PUT":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			in["status"] = map[string]any{"status": "pending"}
			prof["change_request"] = in
			writeJSON(w, 202, prof)
		default:
			w.WriteHeader(405)
		}
	case parts[0] == "application-profiles":
		m.serveNRAppProfiles(w, r, body, parts, page)
	default:
		writeJSON(w, 404, map[string]any{"title": "unhandled " + r.Method + " " + r.URL.Path})
	}
}

// approveNRChange applies a pending change request to the profile, as Vonage does after review.
func (m *mockVonage) approveNRChange(id string) {
	m.mutate(func() {
		prof := m.nrProfiles[id]
		cr, _ := prof["change_request"].(map[string]any)
		for k, v := range cr {
			if k != "status" {
				prof[k] = v
			}
		}
		prof["change_request"] = map[string]any{"status": map[string]any{"status": "accepted"}}
	})
}
