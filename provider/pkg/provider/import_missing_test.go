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
	"testing"
)

// Regression for #1715: importing an object that does not exist used to exit 0 with an empty
// resource in state, because Read answered with the empty response that only a refresh treats as
// "deleted". An import (no prior inputs or state) of a missing object is now an error.
func TestImportOfMissingObjectFails(t *testing.T) {
	for typ, id := range map[string]string{
		"ApiSecret":              "key12345/missing",
		"Application":            "missing-app",
		"Number":                 "US:14155550199",
		"NumberPool":             "missing-pool",
		"NumberPoolNumber":       "missing-pool/14155550199",
		"SubAccount":             "missing1",
		"VerifyTemplate":         "missing-template",
		"VerifyTemplateFragment": "missing-template/missing-fragment",
	} {
		t.Run(typ, func(t *testing.T) {
			l := newVLC(t, typ, "x")
			l.requireImportFails(id)
		})
	}
}
