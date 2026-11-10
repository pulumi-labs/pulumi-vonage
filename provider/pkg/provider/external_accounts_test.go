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

func messengerInputs() map[string]any {
	return map[string]any{"externalId": "1234567890", "accessToken": "tok-1", "name": "Shop page"}
}

func TestMessengerAccount_FullLifecycle(t *testing.T) {
	l := newVLC(t, "MessengerAccount", "page")
	in := messengerInputs()
	c := l.create(in)
	require.Equal(t, "1234567890", c.ID)
	require.Equal(t, "tok-1", l.m.extAccounts["messenger"]["1234567890"]["access_token"])
	require.Equal(t, "key12345", str(c.Properties, "apiKey"))
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Token rotation and a rename are in-place updates.
	in2 := clone(in)
	in2["accessToken"] = "tok-2"
	in2["name"] = "Renamed"
	d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, pgp.Update, diffKinds(d)["accessToken"])
	require.Equal(t, pgp.Update, diffKinds(d)["name"])
	l.update(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, "tok-2", l.m.extAccounts["messenger"]["1234567890"]["access_token"])
	require.Equal(t, "Renamed", l.m.extAccounts["messenger"]["1234567890"]["name"])

	// A name Vonage assigned itself is not a diff when the program leaves the name unset,
	// on create, on refresh and on the next preview.
	noName := map[string]any{"externalId": "555", "accessToken": "tok-9"}
	c2 := l.create(noName)
	l.m.mutate(func() { l.m.extAccounts["messenger"]["555"]["name"] = "Server Assigned" })
	rd2 := l.read(c2.ID, c2.Properties, pm(noName))
	noChanges(t, l.diff(c2.ID, rd2.Properties, pm(noName), pm(noName)))
	// Updating the token leaves the server name alone.
	noName2 := map[string]any{"externalId": "555", "accessToken": "tok-10"}
	l.update(c2.ID, rd2.Properties, pm(noName2), pm(noName))
	require.Equal(t, "Server Assigned", l.m.extAccounts["messenger"]["555"]["name"])

	// A Page ID change replaces.
	in4 := clone(in2)
	in4["externalId"] = "999"
	require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, rd.Properties, pm(in4), pm(in2)))["externalId"])

	l.del(c.ID, rd.Properties)
	require.NotContains(t, l.m.extAccounts["messenger"], "1234567890")
	l.del(c.ID, rd.Properties) // gone outside Pulumi: quiet
}

func TestMessengerAccount_ImportDeletedAndCheck(t *testing.T) {
	l := newVLC(t, "MessengerAccount", "page")
	l.m.mutate(func() {
		l.m.extAccounts = map[string]map[string]map[string]any{"messenger": {"42": {"external_id": "42", "api_key": "key12345",
			"provider": "messenger", "access_token": "adopted-token", "name": "Adopted", "applications": []any{"app-1"}}}}
	})
	rd := l.read("42", emptyMap(), emptyMap())
	require.Equal(t, "42", rd.ID)
	require.Equal(t, "42", str(rd.Inputs, "externalId"))
	require.Equal(t, "adopted-token", str(rd.Inputs, "accessToken"))
	require.Equal(t, "Adopted", str(rd.Inputs, "name"))
	apps, ok := rd.Properties.GetOk("applications")
	require.True(t, ok)
	require.Equal(t, 1, apps.AsArray().Len())

	l.m.mutate(func() { delete(l.m.extAccounts["messenger"], "42") })
	require.Empty(t, l.read("42", rd.Properties, rd.Inputs).ID, "deleted outside Pulumi reads as gone")
	_, err := l.s.Read(pgp.ReadRequest{ID: "42", Urn: l.urn, Properties: emptyMap(), Inputs: emptyMap()})
	require.ErrorContains(t, err, "cannot import")

	bad := messengerInputs()
	bad["externalId"] = "a/b"
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(bad)})
	require.NoError(t, err)
	require.Len(t, chk.Failures, 1)
	bad = messengerInputs()
	bad["accessToken"] = " "
	chk, err = l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(bad)})
	require.NoError(t, err)
	require.Len(t, chk.Failures, 1)

	l.create(messengerInputs())
	_, err = l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(messengerInputs())})
	require.ErrorContains(t, err, "400")
}

func TestExternalAccountApplicationLink_FullLifecycle(t *testing.T) {
	m, srv := newMock(t)
	s := newServerFor(t, srv.URL)
	m.mutate(func() {
		m.extAccounts = map[string]map[string]map[string]any{
			"whatsapp": {"14155550100": {"external_id": "14155550100", "api_key": "key12345", "provider": "whatsapp", "applications": []any{}}},
		}
	})
	l := &vlc{t: t, s: s, m: m, urn: vonageURN("ExternalAccountApplicationLink", "link"), srvURL: srv.URL}

	in := map[string]any{"channel": "whatsapp", "externalId": "14155550100", "applicationId": "app-1"}
	c := l.create(in)
	require.Equal(t, "whatsapp/14155550100/app-1", c.ID)
	require.Equal(t, []any{"app-1"}, m.extAccounts["whatsapp"]["14155550100"]["applications"])
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, c.ID, rd.ID)
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Import from the ID alone.
	imp := l.read(c.ID, emptyMap(), emptyMap())
	require.Equal(t, "app-1", str(imp.Inputs, "applicationId"))
	require.Equal(t, "whatsapp", str(imp.Inputs, "channel"))
	require.Equal(t, "14155550100", str(imp.Inputs, "externalId"))

	// Every field replaces.
	for k, v := range map[string]any{"applicationId": "app-2", "externalId": "1", "channel": "messenger"} {
		in2 := clone(in)
		in2[k] = v
		require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, rd.Properties, pm(in2), pm(in)))[k], k)
	}

	// Creating again with the application already linked is idempotent; a second application hits the limit.
	l.create(in)
	in2 := clone(in)
	in2["applicationId"] = "app-2"
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in2)})
	require.ErrorContains(t, err, "409")

	// Unlinked outside Pulumi.
	m.mutate(func() { m.extAccounts["whatsapp"]["14155550100"]["applications"] = []any{} })
	require.Empty(t, l.read(c.ID, rd.Properties, rd.Inputs).ID)
	l.del(c.ID, rd.Properties) // 409 (not linked) is tolerated
	// Account deleted outside Pulumi.
	m.mutate(func() { delete(m.extAccounts["whatsapp"], "14155550100") })
	require.Empty(t, l.read(c.ID, rd.Properties, rd.Inputs).ID)
	l.del(c.ID, rd.Properties)
	_, err = l.s.Read(pgp.ReadRequest{ID: "no-slash", Urn: l.urn, Properties: emptyMap(), Inputs: emptyMap()})
	require.Error(t, err)
	_, err = l.s.Read(pgp.ReadRequest{ID: "whatsapp/14155550100/app-1", Urn: l.urn, Properties: emptyMap(), Inputs: emptyMap()})
	require.ErrorContains(t, err, "cannot import")

	// Linking to an account that does not exist fails clearly; channel is validated.
	_, err = l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in)})
	require.ErrorContains(t, err, "does not exist")
	bad := clone(in)
	bad["channel"] = "sms"
	chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(bad)})
	require.NoError(t, err)
	require.Len(t, chk.Failures, 1)
}

func TestExternalAccounts_Lookups(t *testing.T) {
	l := newVLC(t, "MessengerAccount", "page")
	l.m.mutate(func() {
		l.m.extAccounts = map[string]map[string]map[string]any{
			"messenger": {"1": {"external_id": "1", "api_key": "key12345", "provider": "messenger", "access_token": "secret-tok", "name": "Page", "applications": []any{"app-1"}}},
			"whatsapp":  {"2": {"external_id": "2", "api_key": "key12345", "provider": "whatsapp"}},
		}
	})
	r, err := l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccount", Args: pm(map[string]any{"channel": "whatsapp", "externalId": "2"})})
	require.NoError(t, err)
	require.Equal(t, "whatsapp", str(r.Return, "channel"))
	apps, _ := r.Return.GetOk("applications")
	require.Equal(t, 0, apps.AsArray().Len())

	r, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccountApplicationLink", Args: pm(map[string]any{"channel": "messenger", "externalId": "1", "applicationId": "app-1"})})
	require.NoError(t, err)
	lk, _ := r.Return.GetOk("linked")
	require.True(t, lk.AsBool())
	r, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccountApplicationLink", Args: pm(map[string]any{"channel": "messenger", "externalId": "1", "applicationId": "other"})})
	require.NoError(t, err)
	lk, _ = r.Return.GetOk("linked")
	require.False(t, lk.AsBool())
	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccountApplicationLink", Args: pm(map[string]any{"channel": "messenger", "externalId": "nope", "applicationId": "x"})})
	require.ErrorContains(t, err, "not found")

	r, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getMessengerAccount", Args: pm(map[string]any{"externalId": "1"})})
	require.NoError(t, err)
	require.Equal(t, "secret-tok", str(r.Return, "accessToken"))
	require.Equal(t, "Page", str(r.Return, "name"))

	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccount", Args: pm(map[string]any{"channel": "messenger", "externalId": "nope"})})
	require.ErrorContains(t, err, "not found")
	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getMessengerAccount", Args: pm(map[string]any{"externalId": "nope"})})
	require.ErrorContains(t, err, "not found")
	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccount", Args: pm(map[string]any{"channel": "sms", "externalId": "1"})})
	require.Error(t, err)

	r, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccounts", Args: pm(map[string]any{})})
	require.NoError(t, err)
	all, _ := r.Return.GetOk("accounts")
	require.Equal(t, 2, all.AsArray().Len())
	r, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccounts", Args: pm(map[string]any{"channel": "messenger"})})
	require.NoError(t, err)
	one, _ := r.Return.GetOk("accounts")
	require.Equal(t, 1, one.AsArray().Len())
}

func TestExternalAccounts_ListPaging(t *testing.T) {
	l := newVLC(t, "MessengerAccount", "page")
	l.m.mutate(func() {
		accts := map[string]map[string]any{}
		for i := 0; i < 7; i++ {
			id := fmt.Sprintf("p%02d", i)
			accts[id] = map[string]any{"external_id": id, "api_key": "key12345", "provider": "messenger"}
		}
		l.m.extAccounts = map[string]map[string]map[string]any{"messenger": accts}
		l.m.extPageLimit = 3
	})
	r, err := l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccounts", Args: pm(map[string]any{})})
	require.NoError(t, err)
	all, _ := r.Return.GetOk("accounts")
	require.Equal(t, 7, all.AsArray().Len(), "every page is followed")

	// A full page with no paging links is refused rather than guessed.
	l.m.mutate(func() { l.m.extNoLinks = true; l.m.extPageLimit = 100 })
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("q%03d", i)
		l.m.mutate(func() { l.m.extAccounts["messenger"][id] = map[string]any{"external_id": id, "provider": "messenger"} })
	}
	_, err = l.s.Invoke(pgp.InvokeRequest{Token: "vonage:index:getExternalAccounts", Args: pm(map[string]any{})})
	require.ErrorContains(t, err, "refusing to guess")
}
