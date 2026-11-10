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

func nrAppProfileInputs(businessProfileID string) map[string]any {
	return map[string]any{
		"businessProfileId":       businessProfileID,
		"name":                    "Fraud shield",
		"categoryId":              "finance",
		"networkApis":             []any{map[string]any{"networkApiId": "sim-swap", "purposeIds": []any{"fraud", "kyc"}}},
		"infrastructureLocations": []any{"DE", "ES"},
	}
}

// newAppLC returns a lifecycle harness for NetworkApplicationProfile with one business profile (bp-1).
func newAppLC(t *testing.T) *vlc {
	t.Helper()
	l := newVLC(t, "NetworkApplicationProfile", "ap")
	l.m.mutate(func() { l.m.nrProfiles = map[string]map[string]any{"bp-1": {"id": "bp-1", "account_id": "key12345"}} })
	return l
}

func TestNetworkApplicationProfile_FullLifecycle(t *testing.T) {
	l := newAppLC(t)
	in := nrAppProfileInputs("bp-1")
	c := l.create(in)
	require.Equal(t, "ap-1", c.ID)
	require.Equal(t, "key12345", str(c.Properties, "accountId"))
	require.Equal(t, "pending", str(c.Properties, "status"))
	body := l.m.nrApps["ap-1"]
	require.Equal(t, "Fraud shield", body["name"])
	require.Equal(t, "finance", body["category_id"])
	_, sentTC := body["terms_and_conditions_accepted"]
	require.False(t, sentTC, "unset optional inputs are not sent")
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// The status moves on its own and never shows as a diff.
	l.m.mutate(func() { l.m.nrApps["ap-1"]["status"] = map[string]any{"status": "rejected", "message": "bad purpose"} })
	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, "rejected", str(rd.Properties, "status"))
	require.Equal(t, "bad purpose", str(rd.Properties, "statusMessage"))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Server spelling and ordering differences are not drift.
	l.m.mutate(func() {
		l.m.nrApps["ap-1"]["infrastructure_locations"] = []any{"es", "de"}
		l.m.nrApps["ap-1"]["network_apis"] = []any{map[string]any{"network_api_id": "sim-swap", "purpose_ids": []any{"kyc", "fraud"}}}
	})
	rd = l.read(c.ID, rd.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// There is no update call: every input change replaces, creating the new profile first.
	for name, mut := range map[string]func(map[string]any){
		"name":                    func(m map[string]any) { m["name"] = "Other" },
		"categoryId":              func(m map[string]any) { m["categoryId"] = "games" },
		"description":             func(m map[string]any) { m["description"] = "d" },
		"privacyPolicyUrl":        func(m map[string]any) { m["privacyPolicyUrl"] = "https://x" },
		"infrastructureLocations": func(m map[string]any) { m["infrastructureLocations"] = []any{"DE"} },
		"networkApis": func(m map[string]any) {
			m["networkApis"] = []any{map[string]any{"networkApiId": "number-verification", "purposeIds": []any{"fraud"}}}
		},
		"businessProfileId":          func(m map[string]any) { m["businessProfileId"] = "bp-2" },
		"termsAndConditionsAccepted": func(m map[string]any) { m["termsAndConditionsAccepted"] = true },
	} {
		in2 := clone(in)
		mut(in2)
		d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
		require.Equal(t, pgp.UpdateReplace, diffKinds(d)[name], name)
		require.False(t, d.DeleteBeforeReplace, name+": create first")
	}

	l.del(c.ID, rd.Properties)
	require.NotEmpty(t, l.m.nrApps["ap-1"]["delete_at"], "delete is soft")
	l.del(c.ID, rd.Properties) // deleting again is quiet
	gone := l.read(c.ID, rd.Properties, rd.Inputs)
	require.Empty(t, gone.ID, "a deleted profile reads as gone")
	l.m.mutate(func() { delete(l.m.nrApps, "ap-1") })
	l.del(c.ID, rd.Properties) // a profile that is gone for good is quiet too
}

func TestNetworkApplicationProfile_OptionalFieldsAndImport(t *testing.T) {
	l := newAppLC(t)
	in := nrAppProfileInputs("bp-1")
	c := l.create(in)
	// Fields the program never set stay unmanaged even when Vonage reports them.
	l.m.mutate(func() {
		l.m.nrApps["ap-1"]["description"] = "Server text"
		l.m.nrApps["ap-1"]["privacy_policy_url"] = "https://server"
		l.m.nrApps["ap-1"]["terms_and_conditions_accepted"] = true
	})
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	for _, k := range []string{"description", "privacyPolicyUrl", "termsAndConditionsAccepted"} {
		_, ok := rd.Inputs.GetOk(k)
		require.False(t, ok, "%s stays unmanaged on refresh", k)
	}

	// A set field that drifts shows up.
	in2 := clone(in)
	in2["description"] = "mine"
	c2 := l.create(in2)
	l.m.mutate(func() { l.m.nrApps[c2.ID]["description"] = "changed outside" })
	rd2 := l.read(c2.ID, c2.Properties, pm(in2))
	require.Equal(t, "changed outside", str(rd2.Inputs, "description"))
	require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c2.ID, rd2.Properties, pm(in2), rd2.Inputs))["description"])

	// Import adopts everything the server reports.
	imp := l.read("ap-1", emptyMap(), emptyMap())
	require.Equal(t, "ap-1", imp.ID)
	require.Equal(t, "Fraud shield", str(imp.Inputs, "name"))
	require.Equal(t, "Server text", str(imp.Inputs, "description"))
	require.Equal(t, "https://server", str(imp.Inputs, "privacyPolicyUrl"))
	v, ok := imp.Inputs.GetOk("termsAndConditionsAccepted")
	require.True(t, ok)
	require.True(t, v.AsBool())
	require.Equal(t, 2, imp.Inputs.Get("infrastructureLocations").AsArray().Len())
	l.requireImportFails("nosuch")
}

func TestNetworkApplicationProfile_CheckAndCreateRefused(t *testing.T) {
	l := newAppLC(t)
	for name, mut := range map[string]func(map[string]any){
		"empty name":      func(m map[string]any) { m["name"] = " " },
		"long name":       func(m map[string]any) { m["name"] = string(make([]byte, 101)) },
		"empty category":  func(m map[string]any) { m["categoryId"] = "" },
		"no network apis": func(m map[string]any) { m["networkApis"] = []any{} },
		"empty api id": func(m map[string]any) {
			m["networkApis"] = []any{map[string]any{"networkApiId": "", "purposeIds": []any{}}}
		},
		"bad location": func(m map[string]any) { m["infrastructureLocations"] = []any{"Germany"} },
	} {
		in := clone(nrAppProfileInputs("bp-1"))
		mut(in)
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(in)})
		require.NoError(t, err, name)
		require.NotEmpty(t, chk.Failures, name)
	}
	l.check(nrAppProfileInputs("bp-1"))

	// A profile of an unknown business profile is refused by Vonage and nothing is tracked.
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(nrAppProfileInputs("bp-9"))})
	require.ErrorContains(t, err, "422")
	require.Empty(t, l.m.nrApps)
	d, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(nrAppProfileInputs("bp-1")), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, d.ID)
	require.Empty(t, l.m.nrApps, "preview does not call the API")
}

func TestNetworkApplicationProfileShare_Lifecycle(t *testing.T) {
	l := newAppLC(t)
	l.create(nrAppProfileInputs("bp-1"))
	sl := &vlc{t: t, s: l.s, m: l.m, urn: vonageURN("NetworkApplicationProfileShare", "sh"), srvURL: l.srvURL}
	in := map[string]any{"profileId": "ap-1", "accountId": "acc2"}
	c := sl.create(in)
	require.Equal(t, "ap-1/acc2", c.ID)
	require.Equal(t, []string{"acc2"}, l.m.nrAppShare["ap-1"])
	noChanges(t, sl.diff(c.ID, c.Properties, pm(in), pm(in)))
	rd := sl.read(c.ID, c.Properties, pm(in))
	require.Equal(t, c.ID, rd.ID)

	// Either input replaces.
	for name, mut := range map[string]func(map[string]any){
		"accountId": func(m map[string]any) { m["accountId"] = "acc3" },
		"profileId": func(m map[string]any) { m["profileId"] = "ap-2" },
	} {
		in2 := clone(in)
		mut(in2)
		require.Equal(t, pgp.UpdateReplace, diffKinds(sl.diff(c.ID, rd.Properties, pm(in2), pm(in)))[name], name)
	}

	// Sharing twice is refused by Vonage and reported.
	_, err := sl.s.Create(pgp.CreateRequest{Urn: sl.urn, Properties: sl.check(in)})
	require.ErrorContains(t, err, "409")

	// A share removed outside Pulumi reads as gone; so does one of a deleted-for-good profile.
	l.m.mutate(func() { l.m.nrAppShare["ap-1"] = nil })
	require.Empty(t, sl.read(c.ID, rd.Properties, pm(in)).ID)
	sl.del(c.ID, rd.Properties) // unsharing an absent share is quiet
	l.m.mutate(func() { delete(l.m.nrApps, "ap-1") })
	require.Empty(t, sl.read(c.ID, rd.Properties, pm(in)).ID)
	sl.del(c.ID, rd.Properties)
}

func TestNetworkApplicationProfileShare_ImportDeleteAndCheck(t *testing.T) {
	l := newAppLC(t)
	l.create(nrAppProfileInputs("bp-1"))
	l.m.mutate(func() { l.m.nrAppShare["ap-1"] = []string{"acc2", "acc3"} })
	sl := &vlc{t: t, s: l.s, m: l.m, urn: vonageURN("NetworkApplicationProfileShare", "sh"), srvURL: l.srvURL}
	imp := sl.read("ap-1/acc3", emptyMap(), emptyMap())
	require.Equal(t, "ap-1/acc3", imp.ID)
	require.Equal(t, "ap-1", str(imp.Inputs, "profileId"))
	require.Equal(t, "acc3", str(imp.Inputs, "accountId"))
	sl.requireImportFails("ap-1/nosuch")
	sl.requireImportFails("nosuch/acc2")
	_, err := sl.s.Read(pgp.ReadRequest{ID: "badid", Urn: sl.urn, Properties: emptyMap(), Inputs: emptyMap()})
	require.ErrorContains(t, err, "PROFILE_ID/ACCOUNT_ID")

	sl.del(imp.ID, imp.Properties)
	require.Equal(t, []string{"acc2"}, l.m.nrAppShare["ap-1"], "only the named account is unshared")

	for _, in := range []map[string]any{{"profileId": "", "accountId": "a"}, {"profileId": "p", "accountId": "a/b"}} {
		chk, err := sl.s.Check(pgp.CheckRequest{Urn: sl.urn, Inputs: pm(in)})
		require.NoError(t, err)
		require.NotEmpty(t, chk.Failures)
	}
}

func TestGetNetworkApplicationProfile_ListFilterAndShares(t *testing.T) {
	l := newAppLC(t)
	l.m.mutate(func() { l.m.nrProfiles["bp-2"] = map[string]any{"id": "bp-2"} })
	l.create(nrAppProfileInputs("bp-1"))
	in2 := nrAppProfileInputs("bp-2")
	in2["name"] = "Second"
	l.create(in2)
	for i := 0; i < 3; i++ {
		in := nrAppProfileInputs("bp-1")
		in["name"] = fmt.Sprintf("Extra%d", i)
		l.create(in)
	}
	got, err := invoke(t, l.s, "vonage:index:getNetworkApplicationProfile", map[string]any{"profileId": "ap-2"})
	require.NoError(t, err)
	require.Equal(t, "ap-2", str(got, "profileId"))
	require.Equal(t, "Second", str(got, "name"))
	require.Equal(t, "bp-2", str(got, "businessProfileId"))
	require.Equal(t, "sim-swap", str(got.Get("networkApis").AsArray().Get(0).AsMap(), "networkApiId"))
	require.Equal(t, 2, got.Get("infrastructureLocations").AsArray().Len())
	_, err = invoke(t, l.s, "vonage:index:getNetworkApplicationProfile", map[string]any{"profileId": "nosuch"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getNetworkApplicationProfile", map[string]any{"profileId": ""})
	require.ErrorContains(t, err, "must not be empty")

	// Every page is followed, filters are sent, and deleted profiles are neither found nor listed.
	l.m.mutate(func() { l.m.nrPageLimit = 2 })
	list, err := invoke(t, l.s, "vonage:index:getNetworkApplicationProfiles", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 5, list.Get("profiles").AsArray().Len())
	byBP, err := invoke(t, l.s, "vonage:index:getNetworkApplicationProfiles", map[string]any{"businessProfileId": "bp-2"})
	require.NoError(t, err)
	require.Equal(t, 1, byBP.Get("profiles").AsArray().Len())
	pending, err := invoke(t, l.s, "vonage:index:getNetworkApplicationProfiles", map[string]any{"status": "accepted"})
	require.NoError(t, err)
	require.Equal(t, 0, pending.Get("profiles").AsArray().Len())
	shared, err := invoke(t, l.s, "vonage:index:getNetworkApplicationProfiles", map[string]any{"shared": true})
	require.NoError(t, err)
	require.Equal(t, 0, shared.Get("profiles").AsArray().Len())
	l.m.mutate(func() { l.m.nrApps["ap-1"]["delete_at"] = "2026-12-01T00:00:00Z" })
	_, err = invoke(t, l.s, "vonage:index:getNetworkApplicationProfile", map[string]any{"profileId": "ap-1"})
	require.ErrorContains(t, err, "was not found")
	list, err = invoke(t, l.s, "vonage:index:getNetworkApplicationProfiles", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 4, list.Get("profiles").AsArray().Len())

	l.m.mutate(func() { l.m.nrAppShare["ap-2"] = []string{"a1", "a2", "a3"} })
	sh, err := invoke(t, l.s, "vonage:index:getNetworkApplicationProfileShares", map[string]any{"profileId": "ap-2"})
	require.NoError(t, err)
	require.Equal(t, 3, sh.Get("accountIds").AsArray().Len(), "share pages are followed")
	one, err := invoke(t, l.s, "vonage:index:getNetworkApplicationProfileShare", map[string]any{"profileId": "ap-2", "accountId": "a2"})
	require.NoError(t, err)
	require.Equal(t, "a2", str(one, "accountId"))
	_, err = invoke(t, l.s, "vonage:index:getNetworkApplicationProfileShare", map[string]any{"profileId": "ap-2", "accountId": "zzz"})
	require.ErrorContains(t, err, "is not shared with")
	_, err = invoke(t, l.s, "vonage:index:getNetworkApplicationProfileShare", map[string]any{"profileId": "nosuch", "accountId": "a2"})
	require.ErrorContains(t, err, "is not shared with")
	_, err = invoke(t, l.s, "vonage:index:getNetworkApplicationProfileShare", map[string]any{"profileId": "", "accountId": "a2"})
	require.ErrorContains(t, err, "must not be empty")
	_, err = invoke(t, l.s, "vonage:index:getNetworkApplicationProfileShares", map[string]any{"profileId": "nosuch"})
	require.ErrorContains(t, err, "was not found")
}
