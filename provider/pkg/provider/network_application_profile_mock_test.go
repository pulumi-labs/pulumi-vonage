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
	"slices"
	"sort"
)

// serveNRAppProfiles is the fake application-profile part of the Network Registry API (spec v1.0.0).
// Called with the mock's lock held. A profile needs an existing business profile, deletes softly
// (delete_at), and has no update call; shares are added and removed by account ID.
func (m *mockVonage) serveNRAppProfiles(w http.ResponseWriter, r *http.Request, body []byte, parts []string, page func([]any, string)) {
	if m.nrApps == nil {
		m.nrApps = map[string]map[string]any{}
	}
	if m.nrAppShare == nil {
		m.nrAppShare = map[string][]string{}
	}
	problem := func(code int, title string) { writeJSON(w, code, map[string]any{"title": title}) }
	if len(parts) == 1 {
		switch r.Method {
		case "POST":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			for _, req := range []string{"business_profile_id", "name", "category_id", "network_apis", "infrastructure_locations"} {
				if in[req] == nil {
					problem(400, "missing "+req)
					return
				}
			}
			if _, ok := m.nrProfiles[fmt.Sprint(in["business_profile_id"])]; !ok {
				problem(422, "unknown business profile")
				return
			}
			m.nrAppNext++
			id := fmt.Sprintf("ap-%d", m.nrAppNext)
			in["id"], in["account_id"] = id, "key12345"
			in["status"] = map[string]any{"status": "pending"}
			in["created_at"], in["updated_at"] = "2026-11-10T00:00:00Z", "2026-11-10T00:00:00Z"
			m.nrApps[id] = in
			writeJSON(w, 201, in)
		case "GET":
			ids := []string{}
			for id := range m.nrApps {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			items := []any{}
			q := r.URL.Query()
			for _, id := range ids {
				a := m.nrApps[id]
				if bp := q.Get("business_profile_id"); bp != "" && a["business_profile_id"] != bp {
					continue
				}
				if st := q.Get("status"); st != "" {
					if s, _ := a["status"].(map[string]any); s["status"] != st {
						continue
					}
				}
				if m.nrShared == (q.Get("shared") == "true") {
					items = append(items, a)
				}
			}
			page(items, "application_profiles")
		default:
			w.WriteHeader(405)
		}
		return
	}
	app, ok := m.nrApps[parts[1]]
	if !ok {
		problem(404, "Application profile not found")
		return
	}
	switch {
	case len(parts) == 2 && r.Method == "GET":
		writeJSON(w, 200, app)
	case len(parts) == 2 && r.Method == "DELETE":
		app["delete_at"] = "2026-12-10T00:00:00Z"
		w.WriteHeader(202)
	case len(parts) == 3 && parts[2] == "shared" && r.Method == "POST":
		var in map[string]string
		_ = json.Unmarshal(body, &in)
		if in["account_id"] == "" {
			problem(400, "missing account_id")
			return
		}
		if slices.Contains(m.nrAppShare[parts[1]], in["account_id"]) {
			problem(409, "already shared")
			return
		}
		m.nrAppShare[parts[1]] = append(m.nrAppShare[parts[1]], in["account_id"])
		w.WriteHeader(202)
	case len(parts) == 3 && parts[2] == "shared" && r.Method == "GET":
		items := []any{}
		for _, a := range m.nrAppShare[parts[1]] {
			items = append(items, map[string]any{"account_id": a})
		}
		page(items, "account_ids")
	case len(parts) == 4 && parts[2] == "shared" && r.Method == "DELETE":
		if !slices.Contains(m.nrAppShare[parts[1]], parts[3]) {
			problem(404, "share not found")
			return
		}
		m.nrAppShare[parts[1]] = slices.DeleteFunc(m.nrAppShare[parts[1]], func(a string) bool { return a == parts[3] })
		w.WriteHeader(202)
	default:
		w.WriteHeader(405)
	}
}
