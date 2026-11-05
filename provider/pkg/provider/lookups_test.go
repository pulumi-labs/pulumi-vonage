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

	"github.com/stretchr/testify/require"
)

func TestGetSubAccount_FoundAndNotFound(t *testing.T) {
	l := newVLC(t, "SubAccount", "sub")
	c := l.create(map[string]any{"name": "tenant", "suspended": true})
	got, err := invoke(t, l.s, "vonage:index:getSubAccount", map[string]any{"subAccountApiKey": c.ID})
	require.NoError(t, err)
	require.Equal(t, "tenant", str(got, "name"))
	require.Equal(t, "key12345", str(got, "primaryAccountApiKey"))
	require.True(t, got.Get("usePrimaryAccountBalance").AsBool())
	_, err = invoke(t, l.s, "vonage:index:getSubAccount", map[string]any{"subAccountApiKey": "nope0001"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getSubAccount", map[string]any{"subAccountApiKey": ""})
	require.ErrorContains(t, err, "must not be empty")
}

func TestGetApiSecret_FoundListAndNotFound(t *testing.T) {
	l := newVLC(t, "ApiSecret", "sec")
	l.m.secrets["key12345"] = map[string]string{"orig": "2025-01-01T00:00:00Z", "two": "2025-02-01T00:00:00Z"}
	l.m.secrets["sub00001"] = map[string]string{"s0": "2025-03-01T00:00:00Z"}
	got, err := invoke(t, l.s, "vonage:index:getApiSecret", map[string]any{"secretId": "orig"})
	require.NoError(t, err)
	require.Equal(t, "key12345", str(got, "ownerKey"))
	require.Equal(t, "2025-01-01T00:00:00Z", str(got, "createdAt"))
	got, err = invoke(t, l.s, "vonage:index:getApiSecret", map[string]any{"secretId": "s0", "accountKey": "sub00001"})
	require.NoError(t, err)
	require.Equal(t, "sub00001", str(got, "ownerKey"))
	_, err = invoke(t, l.s, "vonage:index:getApiSecret", map[string]any{"secretId": "gone"})
	require.ErrorContains(t, err, "was not found")

	list, err := invoke(t, l.s, "vonage:index:getApiSecrets", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 2, list.Get("secrets").AsArray().Len())
	list, err = invoke(t, l.s, "vonage:index:getApiSecrets", map[string]any{"accountKey": "sub00001"})
	require.NoError(t, err)
	require.Equal(t, 1, list.Get("secrets").AsArray().Len())
}

func TestGetApplications_ListsAndFiltersByName(t *testing.T) {
	l := newVLC(t, "Application", "app")
	l.m.mutate(func() {
		for i, n := range []string{"alpha", "beta", "alpha"} {
			id := "app-" + string(rune('1'+i))
			l.m.apps[id] = map[string]any{"id": id, "name": n, "capabilities": map[string]any{"voice": map[string]any{}}}
		}
		l.m.nextApp = 3
	})
	all, err := invoke(t, l.s, "vonage:index:getApplications", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 3, all.Get("applications").AsArray().Len())
	some, err := invoke(t, l.s, "vonage:index:getApplications", map[string]any{"name": "alpha"})
	require.NoError(t, err)
	require.Equal(t, 2, some.Get("applications").AsArray().Len())
	none, err := invoke(t, l.s, "vonage:index:getApplications", map[string]any{"name": "zzz"})
	require.NoError(t, err)
	require.Equal(t, 0, none.Get("applications").AsArray().Len())
}
