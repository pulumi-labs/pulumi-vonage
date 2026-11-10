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

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/stretchr/testify/require"
)

func poolInputs() map[string]any {
	return map[string]any{"poolId": "sms-pool", "fallback": "Pool", "localizedSenderPreferred": true, "predictableSenderPreferred": false}
}

func TestNumberPool_FullLifecycle(t *testing.T) {
	l := newVLC(t, "NumberPool", "pool")
	in := poolInputs()
	c := l.create(in)
	require.Equal(t, "sms-pool", c.ID)
	require.Equal(t, "key12345", str(c.Properties, "accountId"))
	require.Equal(t, "Pool", l.m.pools["sms-pool"]["fallback"])
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	in2 := clone(in)
	in2["fallback"] = "Reject"
	in2["predictableSenderPreferred"] = true
	d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, pgp.Update, diffKinds(d)["fallback"])
	require.Equal(t, pgp.Update, diffKinds(d)["predictableSenderPreferred"])
	l.update(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, "Reject", l.m.pools["sms-pool"]["fallback"])
	require.Equal(t, true, l.m.pools["sms-pool"]["predictable_sender_preferred"])
	require.Equal(t, true, l.m.pools["sms-pool"]["localized_sender_preferred"], "unchanged flag stays true")

	// A poolId change replaces the pool.
	in3 := clone(in2)
	in3["poolId"] = "other"
	require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, rd.Properties, pm(in3), pm(in2)))["poolId"])

	l.del(c.ID, rd.Properties)
	require.NotContains(t, l.m.pools, "sms-pool")
	// Deleting again (gone outside Pulumi) is quiet.
	l.del(c.ID, rd.Properties)
}

func TestNumberPool_ImportAndDeletedOutsidePulumi(t *testing.T) {
	l := newVLC(t, "NumberPool", "pool")
	l.m.mutate(func() {
		l.m.pools = map[string]map[string]any{"adopted": {"pool_id": "adopted", "account_id": "key12345", "fallback": "CustomFrom",
			"localized_sender_preferred": false, "predictable_sender_preferred": true}}
		l.m.poolNumbers = map[string][]string{}
	})
	rd := l.read("adopted", emptyMap(), emptyMap())
	require.Equal(t, "adopted", rd.ID)
	require.Equal(t, "adopted", str(rd.Inputs, "poolId"))
	require.Equal(t, "CustomFrom", str(rd.Inputs, "fallback"))
	require.Equal(t, "CustomFrom", str(rd.Properties, "fallback"))

	l.m.mutate(func() { delete(l.m.pools, "adopted") })
	gone := l.read("adopted", rd.Properties, rd.Inputs)
	require.Empty(t, gone.ID, "a pool deleted outside Pulumi reads as gone")
}

func TestNumberPool_CheckAndConflict(t *testing.T) {
	l := newVLC(t, "NumberPool", "pool")
	bad := poolInputs()
	bad["fallback"] = "Whatever"
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(bad)})
	require.NoError(t, err)
	require.Len(t, chk.Failures, 1)
	bad = poolInputs()
	bad["poolId"] = "a/b"
	chk, err = l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(bad)})
	require.NoError(t, err)
	require.Len(t, chk.Failures, 1)

	l.create(poolInputs())
	_, err = l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(poolInputs())})
	require.ErrorContains(t, err, "409")
}

func TestNumberPoolNumber_FullLifecycle(t *testing.T) {
	m, srv := newMock(t)
	pool := &vlc{t: t, s: newServerFor(t, srv.URL), m: m, urn: vonageURN("NumberPool", "pool"), srvURL: srv.URL}
	pool.create(poolInputs())
	l := &vlc{t: t, s: pool.s, m: m, urn: vonageURN("NumberPoolNumber", "n"), srvURL: srv.URL}

	in := map[string]any{"poolId": "sms-pool", "number": "14155550100"}
	c := l.create(in)
	require.Equal(t, "sms-pool/14155550100", c.ID)
	require.Equal(t, []string{"14155550100"}, m.poolNumbers["sms-pool"])
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, c.ID, rd.ID)
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Import by ID alone.
	imp := l.read(c.ID, emptyMap(), emptyMap())
	require.Equal(t, "14155550100", str(imp.Inputs, "number"))
	require.Equal(t, "sms-pool", str(imp.Inputs, "poolId"))

	in2 := clone(in)
	in2["number"] = "14155550101"
	require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, rd.Properties, pm(in2), pm(in)))["number"])

	// Removed outside Pulumi.
	m.mutate(func() { m.poolNumbers["sms-pool"] = nil })
	require.Empty(t, l.read(c.ID, rd.Properties, rd.Inputs).ID)
	// Pool deleted outside Pulumi.
	m.mutate(func() { delete(m.pools, "sms-pool") })
	require.Empty(t, l.read(c.ID, rd.Properties, rd.Inputs).ID)

	l.del(c.ID, rd.Properties) // 404 tolerated
	_, err := l.s.Read(pgp.ReadRequest{ID: "no-slash", Urn: l.urn, Properties: emptyMap(), Inputs: emptyMap()})
	require.Error(t, err)
}

func TestNumberPoolNumber_CreateRemoveAndCheck(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	m.mutate(func() {
		m.pools = map[string]map[string]any{"p": {"pool_id": "p"}}
		m.poolNumbers = map[string][]string{"p": {"14155550100"}}
	})
	urn := vonageURN("NumberPoolNumber", "n")
	l := &vlc{t: t, s: s, m: m, urn: urn, srvURL: srv.URL}
	c := l.create(map[string]any{"poolId": "p", "number": "14155550101"})
	l.del(c.ID, c.Properties)
	require.Equal(t, []string{"14155550100"}, m.poolNumbers["p"], "only the managed number is removed")
	require.Equal(t, 1, m.countCalls("DELETE /v2/numberpools/accounts/key12345/pools/p/numbers/14155550101"))

	chk, err := s.Check(pgp.CheckRequest{Urn: urn, Inputs: pm(map[string]any{"poolId": "p", "number": "+14155550100"})})
	require.NoError(t, err)
	require.Len(t, chk.Failures, 1)

	// A missing pool fails the create.
	_, err = s.Create(pgp.CreateRequest{Urn: urn, Properties: l.check(map[string]any{"poolId": "nope", "number": "14155550100"})})
	require.Error(t, err)
}

func TestNumberPool_ListingsWalkEveryPageAndFailLoudly(t *testing.T) {
	m, srv := newMock(t)
	m.mutate(func() {
		m.pools = map[string]map[string]any{"p": {"pool_id": "p", "fallback": "Pool"}}
		m.poolNumbers = map[string][]string{}
		for i := 0; i < 7; i++ {
			m.poolNumbers["p"] = append(m.poolNumbers["p"], fmt.Sprintf("1415555%04d", i))
		}
		m.poolPageLimit = 3
	})
	c := newClient("key12345", "Secret123", srv.URL, srv.URL)
	nums, found, err := c.ListPoolNumbers(t.Context(), "p")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, nums, 7)
	require.Equal(t, 3, m.countCalls("GET /v2/numberpools/accounts/key12345/pools/p/numbers"))

	_, found, err = c.ListPoolNumbers(t.Context(), "missing")
	require.NoError(t, err)
	require.False(t, found)

	m.mutate(func() { m.poolOmitTotal = true })
	_, _, err = c.ListPoolNumbers(t.Context(), "p")
	require.ErrorContains(t, err, "total_pages")
}

func TestNumberPool_Lookups(t *testing.T) {
	l := newVLC(t, "NumberPool", "pool")
	l.create(poolInputs())
	l.m.mutate(func() { l.m.poolNumbers["sms-pool"] = []string{"14155550100", "14155550101"} })

	r, err := l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumberPool", Args: pm(map[string]any{"poolId": "sms-pool"})})
	require.NoError(t, err)
	require.Equal(t, "Pool", str(r.Return, "fallback"))
	require.Equal(t, "key12345", str(r.Return, "accountId"))

	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumberPool", Args: pm(map[string]any{"poolId": "nope"})})
	require.ErrorContains(t, err, "not found")

	r, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumberPools", Args: pm(map[string]any{})})
	require.NoError(t, err)
	pools, ok := r.Return.GetOk("pools")
	require.True(t, ok)
	require.Equal(t, 1, pools.AsArray().Len())

	r, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumberPoolNumbers", Args: pm(map[string]any{"poolId": "sms-pool"})})
	require.NoError(t, err)
	nums, _ := r.Return.GetOk("numbers")
	require.Equal(t, 2, nums.AsArray().Len())

	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumberPoolNumbers", Args: pm(map[string]any{"poolId": "nope"})})
	require.ErrorContains(t, err, "not found")

	// getNumberPoolNumber: found, absent number, missing pool.
	r, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumberPoolNumber", Args: pm(map[string]any{"poolId": "sms-pool", "number": "14155550101"})})
	require.NoError(t, err)
	require.Equal(t, "14155550101", str(r.Return, "number"))
	require.Equal(t, "sms-pool", str(r.Return, "poolId"))
	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumberPoolNumber", Args: pm(map[string]any{"poolId": "sms-pool", "number": "14155559999"})})
	require.ErrorContains(t, err, "is not in pool")
	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getNumberPoolNumber", Args: pm(map[string]any{"poolId": "nope", "number": "14155550101"})})
	require.ErrorContains(t, err, "pool \"nope\" was not found")
}

func TestNumberPool_OptionalFlagsDefaultToFalse(t *testing.T) {
	l := newVLC(t, "NumberPool", "pool")
	in := map[string]any{"poolId": "p", "fallback": "Reject"}
	c := l.create(in)
	require.Equal(t, false, l.m.pools["p"]["localized_sender_preferred"], "required flags are sent as false")
	require.Equal(t, false, l.m.pools["p"]["predictable_sender_preferred"])
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))
	rd := l.read(c.ID, c.Properties, pm(in))
	_, has := rd.Inputs.GetOk("localizedSenderPreferred")
	require.False(t, has, "an unset flag stays unset while the server agrees with the default")
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Drift to true is surfaced; removing a declared true flag from the program clears it.
	l.m.mutate(func() { l.m.pools["p"]["predictable_sender_preferred"] = true })
	rd = l.read(c.ID, rd.Properties, pm(in))
	require.True(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs).HasChanges)
	l.update(c.ID, rd.Properties, pm(in), rd.Inputs)
	require.Equal(t, false, l.m.pools["p"]["predictable_sender_preferred"])
}

func TestNumberPoolNumber_NotYetListedKeepsTheID(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	m.mutate(func() {
		m.pools = map[string]map[string]any{"p": {"pool_id": "p"}}
		m.poolNumbers = map[string][]string{}
	})
	// The add is accepted but the number never shows up in the listing (a partial 207 or lag).
	m.after["POST /v2/numberpools/accounts/key12345/pools/p/numbers"] = func() { m.poolNumbers["p"] = nil }
	l := &vlc{t: t, s: s, m: m, urn: vonageURN("NumberPoolNumber", "n"), srvURL: srv.URL}
	c, err := s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(map[string]any{"poolId": "p", "number": "14155550100"})})
	require.Error(t, err)
	require.Equal(t, "p/14155550100", c.ID)
	require.NotNil(t, c.PartialState)
	require.Contains(t, c.PartialState.Reasons[0], "not yet listed")
}
