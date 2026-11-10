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
	"strings"
)

// pricing answers the three Vonage price list endpoints with the shapes the Vonage SDKs document.
func (m *mockVonage) pricing(w http.ResponseWriter, r *http.Request, p string) {
	service := p[strings.LastIndex(p, "/")+1:]
	if service != "sms" && service != "sms-transit" && service != "voice" {
		writeJSON(w, 400, map[string]any{"error-code": "3", "error-code-label": "Invalid service"})
		return
	}
	country := func(code, name, prefix, price string) map[string]any {
		return map[string]any{"countryCode": code, "countryName": name, "countryDisplayName": name, "currency": "EUR",
			"defaultPrice": price, "dialingPrefix": prefix, "networks": []any{
				map[string]any{"type": "mobile", "price": price, "currency": "EUR", "mcc": "234", "mnc": "10",
					"networkCode": "23410", "networkName": "O2"},
			}}
	}
	all := []any{country("GB", "United Kingdom", "44", "0.03330000"), country("US", "United States", "1", "0.00809000"),
		country("GG", "Guernsey", "44", "0.04")}
	switch {
	case strings.HasPrefix(p, "/account/get-pricing/outbound/"):
		code := r.URL.Query().Get("country")
		for _, c := range all {
			if c.(map[string]any)["countryCode"] == code {
				writeJSON(w, 200, c)
				return
			}
		}
		writeJSON(w, 200, map[string]any{})
	case strings.HasPrefix(p, "/account/get-prefix-pricing/outbound/"):
		prefix := r.URL.Query().Get("prefix")
		list := []any{}
		for _, c := range all {
			if c.(map[string]any)["dialingPrefix"] == prefix {
				list = append(list, c)
			}
		}
		writeJSON(w, 200, map[string]any{"count": len(list), "countries": list})
	default:
		if m.pricingShort {
			writeJSON(w, 200, map[string]any{"count": len(all) + 5, "countries": all})
			return
		}
		writeJSON(w, 200, map[string]any{"count": len(all), "countries": all})
	}
}
