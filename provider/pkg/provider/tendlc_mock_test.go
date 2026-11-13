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
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// serveTenDlc is the fake 10DLC API (spec v1.0.34). Called with the mock's lock held. It follows the
// documented semantics: a deleted brand stays listable but every other call answers 502 "Brand not
// found", a brand with campaigns cannot be deleted (409), and the account PATCH answers 204.
func (m *mockVonage) serveTenDlc(w http.ResponseWriter, r *http.Request, body []byte) {
	if m.tdBrands == nil {
		m.tdBrands = map[string]map[string]any{}
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/10dlc/"), "/")
	switch {
	case parts[0] == "account" && len(parts) == 1:
		switch r.Method {
		case "GET":
			writeJSON(w, 200, map[string]any{"opt_out_assist": m.tdOptOut})
		case "PATCH":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			v, ok := in["opt_out_assist"].(bool)
			if !ok {
				writeJSON(w, 422, map[string]any{"title": "opt_out_assist is required"})
				return
			}
			m.tdOptOut = v
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	case parts[0] == "brands" && len(parts) == 1:
		switch r.Method {
		case "POST":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			if in["account_id"] == nil || in["entity_type"] == nil {
				writeJSON(w, 422, map[string]any{"title": "account_id and entity_type are required"})
				return
			}
			m.tdNext++
			id := fmt.Sprintf("B%06X", m.tdNext)
			in["brand_id"], in["status"] = id, "ACTIVE"
			in["campaign_count"] = 0
			in["owner"], in["partner"], in["shared"] = true, false, false
			in["brand_relationship"] = "BASIC_ACCOUNT"
			in["created_date"], in["last_updated"] = "2026-11-13 10:00:00", "2026-11-13 10:00:00"
			m.tdBrands[id] = in
			writeJSON(w, 201, in)
		case "GET":
			ids := []string{}
			for id := range m.tdBrands {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			items := []any{}
			for _, id := range ids {
				if st := r.URL.Query().Get("status"); st != "" && m.tdBrands[id]["status"] != st {
					continue
				}
				items = append(items, m.tdBrands[id])
			}
			size := m.tdPageSize
			if size <= 0 {
				size = 50
			}
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			start, end := (page-1)*size, page*size
			if start > len(items) {
				start = len(items)
			}
			if end > len(items) {
				end = len(items)
			}
			pages := (len(items) + size - 1) / size
			if pages < 1 {
				pages = 1
			}
			writeJSON(w, 200, map[string]any{"page": page, "page_size": size, "total_pages": pages, "total_items": len(items),
				"_embedded": map[string]any{"brands": append([]any{}, items[start:end]...)}})
		default:
			w.WriteHeader(405)
		}
	case parts[0] == "brands" && len(parts) == 2:
		b, ok := m.tdBrands[parts[1]]
		if !ok {
			writeJSON(w, 404, map[string]any{"title": "Brand not found"})
			return
		}
		if b["status"] == "DELETED" && r.Method != "DELETE" {
			writeJSON(w, 502, map[string]any{"title": "502 - Brand not found"})
			return
		}
		switch r.Method {
		case "GET":
			writeJSON(w, 200, b)
		case "PATCH":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			for _, k := range []string{"account_id", "primary_account_id", "universal_ein", "alt_business_id_type", "alt_business_id", "verification_email"} {
				if _, bad := in[k]; bad {
					writeJSON(w, 422, map[string]any{"title": k + " cannot be patched"})
					return
				}
			}
			for k, v := range in {
				b[k] = v
			}
			b["last_updated"] = "2026-11-13 11:00:00"
			writeJSON(w, 200, b)
		case "DELETE":
			if n, _ := b["campaign_count"].(int); n > 0 {
				writeJSON(w, 409, map[string]any{"title": "brand has active campaigns"})
				return
			}
			b["status"] = "DELETED"
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	case parts[0] == "brands" && len(parts) >= 3 && parts[2] == "campaigns":
		m.serveTenDlcCampaigns(w, r, body, parts)
	case parts[0] == "resellers" || parts[0] == "enum" || parts[0] == "throughputs" || (parts[0] == "campaigns" && len(parts) == 2 && parts[1] == "terms_and_conditions"):
		m.serveTenDlcReference(w, r, body, parts)
	default:
		writeJSON(w, 404, map[string]any{"title": "unhandled " + r.Method + " " + r.URL.Path})
	}
}

// serveTenDlcCampaigns serves /brands/{brand}/campaigns[/{campaign}[/numbers[/{number}]]]. Every call on
// a missing brand answers 404 and on a deleted brand 502 "Brand not found", as the real API does.
func (m *mockVonage) serveTenDlcCampaigns(w http.ResponseWriter, r *http.Request, body []byte, parts []string) {
	if m.tdCampaigns == nil {
		m.tdCampaigns = map[string]map[string]any{}
	}
	if m.tdNumbers == nil {
		m.tdNumbers = map[string]map[string]any{}
	}
	brand, ok := m.tdBrands[parts[1]]
	if !ok {
		writeJSON(w, 404, map[string]any{"title": "Brand not found"})
		return
	}
	if brand["status"] == "DELETED" {
		writeJSON(w, 502, map[string]any{"title": "502 - Brand not found"})
		return
	}
	pageOf := func(items []any, key string) {
		size := m.tdPageSize
		if size <= 0 {
			size = 50
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		start, end := (page-1)*size, page*size
		if start > len(items) {
			start = len(items)
		}
		if end > len(items) {
			end = len(items)
		}
		pages := (len(items) + size - 1) / size
		if pages < 1 {
			pages = 1
		}
		writeJSON(w, 200, map[string]any{"page": page, "page_size": size, "total_pages": pages, "total_items": len(items),
			"_embedded": map[string]any{key: append([]any{}, items[start:end]...)}})
	}
	prefix := parts[1] + "/"
	if len(parts) == 3 {
		switch r.Method {
		case "POST":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			for _, k := range []string{"account_id", "usecase", "description", "help_message", "opt_in_message", "opt_out_message", "sample_one"} {
				if in[k] == nil {
					writeJSON(w, 422, map[string]any{"title": k + " is required"})
					return
				}
			}
			m.tdBodies = append(m.tdBodies, maps.Clone(in))
			if !applyMessageFlow(in) {
				writeJSON(w, 422, map[string]any{"title": "set message_flow or message_flow_details, not both or neither"})
				return
			}
			if in["tc_agreed"] != true {
				writeJSON(w, 422, map[string]any{"title": "tc_agreed must be true"})
				return
			}
			delete(in, "tc_agreed")
			m.tdCampNext++
			id := fmt.Sprintf("C%d", m.tdCampNext)
			in["campaign_id"], in["brand_id"], in["status"] = id, parts[1], "PENDING_REVIEW"
			in["traffic_enabled"], in["auto_renewal"] = false, true
			in["created_date"], in["last_updated"] = "2026-11-13T10:00:00Z", "2026-11-13T10:00:00Z"
			m.tdCampaigns[prefix+id] = in
			brand["campaign_count"] = len(m.campaignsOf(parts[1]))
			writeJSON(w, 201, in)
		case "GET":
			items := []any{}
			for _, c := range m.campaignsOf(parts[1]) {
				if st := r.URL.Query().Get("status"); st != "" && c["status"] != st {
					continue
				}
				items = append(items, c)
			}
			pageOf(items, "campaigns")
		default:
			w.WriteHeader(405)
		}
		return
	}
	camp, ok := m.tdCampaigns[prefix+parts[3]]
	if !ok {
		writeJSON(w, 404, map[string]any{"title": "Campaign not found"})
		return
	}
	if len(parts) == 4 {
		switch r.Method {
		case "GET":
			writeJSON(w, 200, camp)
		case "PATCH":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			if st, _ := camp["status"].(string); st == "TERMINATED" || st == "CANCELED" {
				writeJSON(w, 422, map[string]any{"title": "the campaign may not be updated"})
				return
			}
			if st, _ := camp["status"].(string); st != "PENDING_REVIEW" && st != "UPDATES_REQUIRED" {
				for _, k := range []string{"vertical", "usecase", "sub_usecases", "age_gated", "direct_lending", "embedded_link", "embedded_phone", "hipaa"} {
					if _, bad := in[k]; bad {
						writeJSON(w, 422, map[string]any{"title": k + " is read only after approval"})
						return
					}
				}
			}
			if _, hasFlow := in["message_flow"]; hasFlow || in["message_flow_details"] != nil {
				m.tdBodies = append(m.tdBodies, maps.Clone(in))
				if !applyMessageFlow(in) {
					writeJSON(w, 422, map[string]any{"title": "set message_flow or message_flow_details, not both"})
					return
				}
				if hasFlow {
					delete(camp, "message_flow_details")
				}
			}
			for k, v := range in {
				camp[k] = v
			}
			if v, ok := in["auto_renewal"].(bool); ok && !v && camp["status"] == "ACTIVE" {
				camp["status"] = "PENDING_CANCELLATION"
			}
			camp["last_updated"] = "2026-11-13T11:00:00Z"
			writeJSON(w, 200, camp)
		default:
			w.WriteHeader(405)
		}
		return
	}
	if parts[4] != "numbers" {
		writeJSON(w, 404, map[string]any{"title": "unhandled " + r.Method + " " + r.URL.Path})
		return
	}
	nprefix := prefix + parts[3] + "/"
	if len(parts) == 5 {
		switch r.Method {
		case "POST":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			num, _ := in["number"].(string)
			if num == "" || in["country"] == nil {
				writeJSON(w, 422, map[string]any{"title": "country and number are required"})
				return
			}
			if n, dup := m.tdNumbers[nprefix+num]; dup && n["status"] != "UNLINKED" {
				writeJSON(w, 409, map[string]any{"title": "number already linked"})
				return
			}
			st := "LINKING"
			if m.tdRejectLinks {
				st = "REJECTED"
			}
			n := map[string]any{"number": num, "country": in["country"], "status": st, "compliance": []any{}}
			m.tdNumbers[nprefix+num] = n
			writeJSON(w, 202, n)
		case "GET":
			keys := []string{}
			for k := range m.tdNumbers {
				if strings.HasPrefix(k, nprefix) {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			items := []any{}
			for _, k := range keys {
				items = append(items, m.tdNumbers[k])
			}
			pageOf(items, "numbers")
		default:
			w.WriteHeader(405)
		}
		return
	}
	n, ok := m.tdNumbers[nprefix+parts[5]]
	if !ok {
		writeJSON(w, 404, map[string]any{"title": "Number not found"})
		return
	}
	switch r.Method {
	case "GET":
		if m.tdFailNumberGet {
			writeJSON(w, 500, map[string]any{"title": "boom"})
			return
		}
		writeJSON(w, 200, n)
	case "DELETE":
		n["status"] = "UNLINKED"
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}

func (m *mockVonage) campaignsOf(brandID string) []map[string]any {
	ids := []string{}
	for k := range m.tdCampaigns {
		if strings.HasPrefix(k, brandID+"/") {
			ids = append(ids, k)
		}
	}
	sort.Strings(ids)
	out := []map[string]any{}
	for _, k := range ids {
		out = append(out, m.tdCampaigns[k])
	}
	return out
}

// applyMessageFlow checks that a campaign body carries message_flow or message_flow_details but not both
// (PATCH may carry neither) and, like Vonage, renders the details into the message_flow text.
func applyMessageFlow(in map[string]any) bool {
	d, hasDetails := in["message_flow_details"].(map[string]any)
	_, hasFlow := in["message_flow"]
	if hasDetails && hasFlow {
		return false
	}
	if hasDetails {
		in["message_flow"] = "Rendered from details: " + fmt.Sprint(d["frequency"]) + " messages, privacy policy " + fmt.Sprint(d["privacy_policy"]) + "."
		return true
	}
	return hasFlow
}
