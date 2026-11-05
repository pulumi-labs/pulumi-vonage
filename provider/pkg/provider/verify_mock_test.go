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
	"strings"
)

// serveVerify is the fake of the Verify v2 template endpoints, following the shapes in
// the Vonage Node SDK (template_id/name/is_default; template_fragment_id/channel/locale/
// text/date_created/date_updated; HAL _embedded lists with page, page_size, total_pages).
// It runs under the mock's lock.
func (m *mockVonage) serveVerify(w http.ResponseWriter, r *http.Request, body []byte) {
	if m.templates == nil {
		m.templates = map[string]map[string]any{}
		m.fragments = map[string]map[string]map[string]any{}
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v2/verify/templates"), "/"), "/")
	if parts[0] == "" {
		parts = nil
	}
	var in map[string]any
	_ = json.Unmarshal(body, &in)
	notFound := func() { writeJSON(w, 404, map[string]any{"title": "Not Found"}) }
	paged := func(key string, items []any) {
		size, page := 100, 1
		if v, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && v > 0 {
			size = v
		}
		if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
			page = v
		}
		lo, hi := (page-1)*size, page*size
		if lo > len(items) {
			lo = len(items)
		}
		if hi > len(items) {
			hi = len(items)
		}
		writeJSON(w, 200, map[string]any{"page": page, "page_size": size, "total_pages": (len(items) + size - 1) / size,
			"total_items": len(items), "_embedded": map[string]any{key: items[lo:hi]}})
	}
	switch {
	case len(parts) == 0 && r.Method == "POST":
		m.nextTpl++
		id := fmt.Sprintf("tpl-%d", m.nextTpl)
		t := map[string]any{"template_id": id, "name": in["name"], "is_default": false}
		m.templates[id] = t
		m.fragments[id] = map[string]map[string]any{}
		writeJSON(w, 201, t)
	case len(parts) == 0 && r.Method == "GET":
		ids := []string{}
		for id := range m.templates {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		items := []any{}
		for _, id := range ids {
			items = append(items, m.templates[id])
		}
		paged("templates", items)
	case len(parts) == 1:
		t, ok := m.templates[parts[0]]
		if !ok {
			notFound()
			return
		}
		switch r.Method {
		case "GET":
			writeJSON(w, 200, t)
		case "PATCH":
			if v, ok := in["is_default"].(bool); ok && v {
				for _, o := range m.templates {
					o["is_default"] = false
				}
			}
			for k, v := range in {
				t[k] = v
			}
			writeJSON(w, 200, t)
		case "DELETE":
			delete(m.templates, parts[0])
			delete(m.fragments, parts[0])
			w.WriteHeader(204)
		}
	case len(parts) == 2 && parts[1] == "template_fragments":
		if _, ok := m.templates[parts[0]]; !ok {
			notFound()
			return
		}
		fr := m.fragments[parts[0]]
		switch r.Method {
		case "POST":
			for _, f := range fr {
				if f["channel"] == in["channel"] && f["locale"] == in["locale"] {
					writeJSON(w, 409, map[string]any{"title": "Conflict"})
					return
				}
			}
			id := fmt.Sprintf("frag-%d-%d", m.nextTpl, len(fr)+1)
			f := map[string]any{"template_fragment_id": id, "channel": in["channel"], "locale": in["locale"], "text": in["text"],
				"date_created": "2026-01-01T00:00:00Z", "date_updated": "2026-01-01T00:00:00Z"}
			fr[id] = f
			writeJSON(w, 201, f)
		case "GET":
			ids := []string{}
			for id := range fr {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			items := []any{}
			for _, id := range ids {
				items = append(items, fr[id])
			}
			paged("template_fragments", items)
		}
	case len(parts) == 3 && parts[1] == "template_fragments":
		f, ok := m.fragments[parts[0]][parts[2]]
		if !ok {
			notFound()
			return
		}
		switch r.Method {
		case "GET":
			writeJSON(w, 200, f)
		case "PATCH":
			f["text"] = in["text"]
			f["date_updated"] = "2026-02-02T00:00:00Z"
			writeJSON(w, 200, f)
		case "DELETE":
			delete(m.fragments[parts[0]], parts[2])
			w.WriteHeader(204)
		}
	default:
		notFound()
	}
}
