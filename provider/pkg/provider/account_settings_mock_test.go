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
	"net/http"
	"net/url"
	"strings"
)

// accountSettings fakes POST /account/settings: form fields update the settings, no fields
// leave them unchanged, and the response always carries the current values. A URL containing
// "reject" is refused, standing in for Vonage's "must answer 200" check.
func (m *mockVonage) accountSettings(w http.ResponseWriter, r *http.Request, body []byte) {
	form, _ := url.ParseQuery(string(body))
	m.acctPosts = append(m.acctPosts, form)
	if m.acctSettings == nil {
		m.acctSettings = map[string]string{"mo-callback-url": "", "dr-callback-url": ""}
	}
	for _, f := range []struct{ in, out string }{{"moCallBackUrl", "mo-callback-url"}, {"drCallBackUrl", "dr-callback-url"}} {
		if vs, ok := form[f.in]; ok {
			v := vs[0]
			if strings.Contains(v, "reject") {
				writeJSON(w, 400, map[string]any{"title": "callback URL rejected"})
				return
			}
			m.acctSettings[f.out] = v
		}
	}
	writeJSON(w, 200, map[string]any{
		"mo-callback-url": m.acctSettings["mo-callback-url"], "dr-callback-url": m.acctSettings["dr-callback-url"],
		"max-outbound-request": 30, "max-inbound-request": "30", "max-calls-per-second": 30,
	})
}
