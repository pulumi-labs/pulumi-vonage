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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAccountBalance(t *testing.T) {
	l := newVLC(t, "SubAccount", "sub")
	got, err := invoke(t, l.s, "vonage:index:getAccountBalance", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 12.5, got.Get("value").AsNumber())
	require.True(t, got.Get("autoReload").AsBool())
	require.Contains(t, l.m.callLog(), "GET /account/get-balance")
}

func TestGetNumbers_ListsFiltersAndFollowsPages(t *testing.T) {
	l := newVLC(t, "Number", "n")
	l.m.setNumberRaw("US", "14155550001", map[string]any{"app_id": "app-1"})
	l.m.setNumberRaw("US", "14155550002", nil)
	l.m.setNumberRaw("GB", "447700900003", map[string]any{"type": "landline", "features": []any{"SMS"}})
	all, err := invoke(t, l.s, "vonage:index:getNumbers", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 3, all.Get("numbers").AsArray().Len())

	us, err := invoke(t, l.s, "vonage:index:getNumbers", map[string]any{"country": "US"})
	require.NoError(t, err)
	require.Equal(t, 2, us.Get("numbers").AsArray().Len())
	first := us.Get("numbers").AsArray().Get(0).AsMap()
	require.Equal(t, "14155550001", str(first, "msisdn"))
	require.Equal(t, "app-1", str(first, "appId"))

	linked, err := invoke(t, l.s, "vonage:index:getNumbers", map[string]any{"hasApplication": true})
	require.NoError(t, err)
	require.Equal(t, 1, linked.Get("numbers").AsArray().Len())
	unlinked, err := invoke(t, l.s, "vonage:index:getNumbers", map[string]any{"hasApplication": false})
	require.NoError(t, err)
	require.Equal(t, 2, unlinked.Get("numbers").AsArray().Len())
	byApp, err := invoke(t, l.s, "vonage:index:getNumbers", map[string]any{"applicationId": "app-1"})
	require.NoError(t, err)
	require.Equal(t, 1, byApp.Get("numbers").AsArray().Len())

	// More than one page of 100.
	for i := 0; i < 120; i++ {
		l.m.setNumberRaw("DE", "4915000"+string(rune('0'+i/100))+string(rune('0'+(i/10)%10))+string(rune('0'+i%10)), nil)
	}
	many, err := invoke(t, l.s, "vonage:index:getNumbers", map[string]any{"country": "DE"})
	require.NoError(t, err)
	require.Equal(t, 120, many.Get("numbers").AsArray().Len())

	_, err = invoke(t, l.s, "vonage:index:getNumbers", map[string]any{"searchPattern": 7})
	require.ErrorContains(t, err, "searchPattern must be 0, 1 or 2")
}

func TestGetAvailableNumbers_SearchesAndPages(t *testing.T) {
	l := newVLC(t, "Number", "n")
	l.m.available = []map[string]any{
		{"country": "US", "msisdn": "14155550101", "type": "mobile-lvn", "cost": "0.90", "features": []any{"SMS", "VOICE"}},
		{"country": "US", "msisdn": "14155550102", "type": "landline", "cost": "1.25", "features": []any{"VOICE"}},
		{"country": "US", "msisdn": "14155550103", "type": "mobile-lvn", "cost": "0.90", "features": []any{"SMS"}},
		{"country": "GB", "msisdn": "447700900104", "type": "mobile-lvn", "cost": "1.00", "features": []any{"SMS"}},
	}
	got, err := invoke(t, l.s, "vonage:index:getAvailableNumbers", map[string]any{"country": "US"})
	require.NoError(t, err)
	require.Equal(t, 3.0, got.Get("count").AsNumber())
	require.Equal(t, 3, got.Get("numbers").AsArray().Len())
	n0 := got.Get("numbers").AsArray().Get(0).AsMap()
	require.Equal(t, "0.90", str(n0, "cost"))
	require.Equal(t, "mobile-lvn", str(n0, "type"))

	typed, err := invoke(t, l.s, "vonage:index:getAvailableNumbers", map[string]any{"country": "US", "type": "landline"})
	require.NoError(t, err)
	require.Equal(t, 1, typed.Get("numbers").AsArray().Len())

	page2, err := invoke(t, l.s, "vonage:index:getAvailableNumbers", map[string]any{"country": "US", "size": 2, "index": 2})
	require.NoError(t, err)
	require.Equal(t, 3.0, page2.Get("count").AsNumber(), "count is the total, not the page")
	require.Equal(t, 1, page2.Get("numbers").AsArray().Len())

	none, err := invoke(t, l.s, "vonage:index:getAvailableNumbers", map[string]any{"country": "FR"})
	require.NoError(t, err)
	require.Equal(t, 0, none.Get("numbers").AsArray().Len())

	for name, in := range map[string]map[string]any{
		"country": {"country": "USA"}, "size": {"country": "US", "size": 101}, "index": {"country": "US", "index": 0},
		"type": {"country": "US", "type": "fax"}, "feature": {"country": "US", "features": []any{"FAX"}}, "pattern": {"country": "US", "searchPattern": 3},
	} {
		_, err = invoke(t, l.s, "vonage:index:getAvailableNumbers", in)
		require.Error(t, err, name)
	}
	// Read-only: no purchase call is ever made.
	for _, c := range l.m.callLog() {
		require.NotContains(t, c, "/number/buy")
	}
}

func TestGetSubAccounts_ListsAndFilters(t *testing.T) {
	l := newVLC(t, "SubAccount", "sub")
	l.create(map[string]any{"name": "tenant-a"})
	l.m.mutate(func() {
		l.m.subs["sub00009"] = map[string]any{"api_key": "sub00009", "name": "tenant-b", "primary_account_api_key": "key12345",
			"use_primary_account_balance": false, "created_at": "2026-02-01T00:00:00Z", "suspended": true, "balance": 4.5, "credit_limit": 10.0}
	})
	all, err := invoke(t, l.s, "vonage:index:getSubAccounts", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, "key12345", str(all, "primaryAccountApiKey"))
	require.Equal(t, 2, all.Get("subAccounts").AsArray().Len())
	b := all.Get("subAccounts").AsArray().Get(1).AsMap()
	require.Equal(t, "tenant-b", str(b, "name"))
	require.True(t, b.Get("suspended").AsBool())
	require.Equal(t, 4.5, b.Get("balance").AsNumber())

	one, err := invoke(t, l.s, "vonage:index:getSubAccounts", map[string]any{"name": "tenant-a"})
	require.NoError(t, err)
	require.Equal(t, 1, one.Get("subAccounts").AsArray().Len())
	none, err := invoke(t, l.s, "vonage:index:getSubAccounts", map[string]any{"name": "zzz"})
	require.NoError(t, err)
	require.Equal(t, 0, none.Get("subAccounts").AsArray().Len())
}

// A /account/numbers response that omits "count" is the end of the list only when the page is short. A full
// page without a count must fail loudly, in the lookup and in the ownership check a Number resource relies on,
// instead of silently dropping the numbers on later pages.
func TestGetNumbers_MissingCountFailsLoudlyOnFullPage(t *testing.T) {
	l := newVLC(t, "Number", "n")
	for i := 0; i < 100; i++ {
		l.m.setNumberRaw("DE", fmt.Sprintf("4915000%03d", i), nil)
	}
	l.m.numbersOmitCount = true
	_, err := invoke(t, l.s, "vonage:index:getNumbers", map[string]any{"country": "DE"})
	require.ErrorContains(t, err, "without a total count")

	_, err = ownedClient(l).FindOwnedNumber(t.Context(), "DE", "4915000")
	require.ErrorContains(t, err, "without a total count")

	// A short page without a count is simply the last page.
	short := newVLC(t, "Number", "n")
	short.m.setNumberRaw("US", "14155550001", nil)
	short.m.numbersOmitCount = true
	got, err := invoke(t, short.s, "vonage:index:getNumbers", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 1, got.Get("numbers").AsArray().Len())
}

// The walk is capped: a total that never runs out is an error naming the limit, not a truncated list or
// a "not owned" answer.
func TestGetNumbers_PageCapIsAnError(t *testing.T) {
	l := newVLC(t, "Number", "n")
	l.m.setNumberRaw("US", "14155550001", nil)
	l.m.numbersCountOverride = 1_000_000
	l.m.numbersRepeatFirstPage = true
	_, err := invoke(t, l.s, "vonage:index:getNumbers", map[string]any{})
	require.ErrorContains(t, err, "more than 10000 numbers match")
	require.Equal(t, maxNumberPages, l.m.countCalls("GET /account/numbers"))

	_, err = ownedClient(l).FindOwnedNumber(t.Context(), "US", "1415555000")
	require.ErrorContains(t, err, "more than 10000 numbers match")
}

// ownedClient talks to the same mock the lifecycle uses.
func ownedClient(l *vlc) *Client { return newClient("key12345", "Secret123", l.srvURL, l.srvURL) }
