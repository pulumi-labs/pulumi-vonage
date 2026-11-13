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
	"sort"
	"strconv"
)

// serveTenDlcReference serves the fake reseller, enum, terms and throughput endpoints. Resellers can only
// be created and read (the API has no update or delete).
func (m *mockVonage) serveTenDlcReference(w http.ResponseWriter, r *http.Request, body []byte, parts []string) {
	size := m.tdPageSize
	if size <= 0 {
		size = 50
	}
	switch parts[0] {
	case "enum":
		writeJSON(w, 200, map[string]any{
			"brand_status":    []any{map[string]any{"id": "ACTIVE"}, map[string]any{"id": "DELETED"}},
			"campaign_status": []any{map[string]any{"id": "ACTIVE", "description": "Traffic flows"}},
			"mno":             []any{map[string]any{"id": "10017", "mno": "AT&T"}},
			"usecase": []any{
				map[string]any{"id": "CUSTOMER_CARE", "display_name": "Customer Care", "description": "Account notifications", "classification": "STANDARD", "min_sub_usecases": 1, "max_sub_usecases": 5, "valid_sub_usecase": 1, "min_msg_samples": 2},
				map[string]any{"id": "MIXED", "display_name": "Mixed", "description": "Several use cases", "classification": "STANDARD"},
			},
			"vertical": []any{map[string]any{"id": "RETAIL", "display_name": "Retail", "description": "Shops", "industry_id": "I1"}},
		})
	case "campaigns":
		writeJSON(w, 200, map[string]any{"sub_terms": []any{"Term one.", "Term two."}})
	case "throughputs":
		start, _ := strconv.Atoi(r.URL.Query().Get("page_token"))
		var items []any
		for _, t := range m.tdThroughputs {
			if c := r.URL.Query().Get("campaign_id"); c != "" && t["campaign_id"] != c {
				continue
			}
			items = append(items, t)
		}
		if start > len(items) {
			start = len(items)
		}
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		resp := map[string]any{"_embedded": map[string]any{"throughputs": append([]any{}, items[start:end]...)}}
		if end < len(items) {
			resp["next_page"] = strconv.Itoa(end)
		}
		writeJSON(w, 200, resp)
	case "resellers":
		if m.tdResellers == nil {
			m.tdResellers = map[string]map[string]any{}
		}
		if len(parts) == 1 {
			switch r.Method {
			case "POST":
				var in map[string]any
				_ = json.Unmarshal(body, &in)
				for _, k := range []string{"company_name", "phone", "email"} {
					if in[k] == nil {
						writeJSON(w, 422, map[string]any{"title": k + " is required"})
						return
					}
				}
				m.tdResellerNext++
				id := fmt.Sprintf("res-%d", m.tdResellerNext)
				in["id"] = id
				in["status"] = m.tdResellerStatus
				if m.tdResellerStatus == "" {
					in["status"] = "PENDING"
				}
				if in["status"] == "REJECTED" {
					in["rejection_reason"] = "Company name not allowed."
				}
				m.tdResellers[id] = in
				writeJSON(w, 201, in)
			case "GET":
				ids := []string{}
				for id := range m.tdResellers {
					ids = append(ids, id)
				}
				sort.Strings(ids)
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				if page < 1 {
					page = 1
				}
				start, end := (page-1)*size, page*size
				if start > len(ids) {
					start = len(ids)
				}
				if end > len(ids) {
					end = len(ids)
				}
				items := []any{}
				for _, id := range ids[start:end] {
					items = append(items, m.tdResellers[id])
				}
				pages := (len(ids) + size - 1) / size
				if pages < 1 {
					pages = 1
				}
				writeJSON(w, 200, map[string]any{"page": page, "page_size": size, "total_pages": pages, "_embedded": map[string]any{"resellers": items}})
			default:
				w.WriteHeader(405)
			}
			return
		}
		res, ok := m.tdResellers[parts[1]]
		if !ok {
			writeJSON(w, 404, map[string]any{"title": "Reseller not found"})
			return
		}
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		writeJSON(w, 200, res)
	}
}
