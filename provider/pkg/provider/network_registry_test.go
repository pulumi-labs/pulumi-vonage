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

func nrInputs() map[string]any {
	return map[string]any{
		"countries":                  []any{"ES", "DE"},
		"contactRole":                "CTO",
		"name":                       "Acme GmbH",
		"organisationType":           "forProfitOrganisation",
		"taxNumber":                  "DE123456789",
		"privacyPolicyUrl":           "https://acme.example/privacy",
		"privacyPolicyManagerEmail":  "privacy@acme.example",
		"termsAndConditionsAccepted": true,
		"businessInformation": map[string]any{
			"addressStreet1": "Hauptstrasse 1", "postalCode": "10115", "country": "DE", "city": "Berlin",
		},
	}
}

func TestNetworkBusinessProfile_FullLifecycle(t *testing.T) {
	l := newVLC(t, "NetworkBusinessProfile", "bp")
	in := nrInputs()
	c := l.create(in)
	require.Equal(t, "bp-1", c.ID)
	require.Equal(t, "key12345", str(c.Properties, "accountId"))
	require.Equal(t, "pending", str(c.Properties, "status"))
	require.Equal(t, "DE123456789", l.m.nrProfiles["bp-1"]["tax_number"])
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Refresh keeps the program's values and finds nothing to do.
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// A change goes in as a change request, never as a replace.
	in2 := clone(in)
	in2["contactRole"] = "CEO"
	in2["tradingName"] = "Acme"
	d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, pgp.Update, diffKinds(d)["contactRole"])
	require.Equal(t, pgp.Update, diffKinds(d)["tradingName"])
	up := l.update(c.ID, rd.Properties, pm(in2), pm(in))
	cr, _ := l.m.nrProfiles["bp-1"]["change_request"].(map[string]any)
	require.Equal(t, "CEO", cr["contact_role"])
	require.Equal(t, "CTO", l.m.nrProfiles["bp-1"]["contact_role"], "the profile itself only changes on approval")
	require.Equal(t, "pending", str(up.Properties, "changeRequestStatus"))

	// While the request is pending the refresh reports the requested values: no perpetual diff.
	rd2 := l.read(c.ID, up.Properties, pm(in2))
	require.Equal(t, "CEO", str(rd2.Inputs, "contactRole"))
	noChanges(t, l.diff(c.ID, rd2.Properties, pm(in2), rd2.Inputs))

	// Once approved the profile carries the values and still nothing is pending.
	l.m.approveNRChange("bp-1")
	rd3 := l.read(c.ID, rd2.Properties, rd2.Inputs)
	require.Equal(t, "CEO", str(rd3.Inputs, "contactRole"))
	require.Equal(t, "accepted", str(rd3.Properties, "changeRequestStatus"))
	noChanges(t, l.diff(c.ID, rd3.Properties, pm(in2), rd3.Inputs))

	// A rejected change request is ignored: the refresh shows the live values and the diff resubmits.
	l.m.mutate(func() {
		l.m.nrProfiles["bp-1"]["change_request"] = map[string]any{"contact_role": "CFO", "status": map[string]any{"status": "rejected"}}
	})
	rd4 := l.read(c.ID, rd3.Properties, pm(in2))
	require.Equal(t, "CEO", str(rd4.Inputs, "contactRole"))

	l.del(c.ID, rd4.Properties)
	require.NotEmpty(t, l.m.nrProfiles["bp-1"]["delete_at"], "delete is soft")
	// Deleting again, and refreshing a soft-deleted profile, are quiet.
	l.del(c.ID, rd4.Properties)
	gone := l.read(c.ID, rd4.Properties, rd4.Inputs)
	require.Empty(t, gone.ID, "a deleted profile reads as gone")
}

func TestNetworkBusinessProfile_OptionalFieldsAreUnmanagedUnlessSet(t *testing.T) {
	l := newVLC(t, "NetworkBusinessProfile", "bp")
	in := nrInputs()
	c := l.create(in)
	// Vonage fills fields the program never set.
	l.m.mutate(func() {
		l.m.nrProfiles["bp-1"]["dpo_required"] = true
		l.m.nrProfiles["bp-1"]["email_notifications_enabled"] = true
		l.m.nrProfiles["bp-1"]["trading_name"] = "Server Name"
		l.m.nrProfiles["bp-1"]["countries"] = []any{"de", "es"}
	})
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	for _, k := range []string{"tradingName", "dpoRequired", "emailNotificationsEnabled"} {
		_, ok := rd.Inputs.GetOk(k)
		require.False(t, ok, "%s stays unmanaged on refresh", k)
	}
}

func TestNetworkBusinessProfile_ImportAdoptsServerValues(t *testing.T) {
	l := newVLC(t, "NetworkBusinessProfile", "bp")
	l.m.mutate(func() {
		l.m.nrProfiles = map[string]map[string]any{"bp-9": {
			"id": "bp-9", "account_id": "key12345", "countries": []any{"ES"}, "contact_role": "CTO", "name": "Imported",
			"trading_name": "Imp", "organisation_type": "nonProfitOrganisation", "tax_number": "T", "privacy_policy_url": "https://x",
			"privacy_policy_manager_email": "a@b.c", "terms_and_conditions_accepted": true, "dpo_required": false,
			"business_information": map[string]any{"address_street_1": "s", "postal_code": "p", "country": "ES", "city": "c"},
			"status":               map[string]any{"status": "accepted", "message": "ok"},
		}}
	})
	rd := l.read("bp-9", emptyMap(), emptyMap())
	require.Equal(t, "bp-9", rd.ID)
	require.Equal(t, "Imported", str(rd.Inputs, "name"))
	require.Equal(t, "Imp", str(rd.Inputs, "tradingName"))
	require.Equal(t, "accepted", str(rd.Properties, "status"))
	require.Equal(t, "ok", str(rd.Properties, "statusMessage"))
	v, ok := rd.Inputs.GetOk("dpoRequired")
	require.True(t, ok)
	require.False(t, v.AsBool())
	l.requireImportFails("nosuch")
}

func TestNetworkBusinessProfile_Check(t *testing.T) {
	l := newVLC(t, "NetworkBusinessProfile", "bp")
	for name, mut := range map[string]func(map[string]any){
		"no countries":  func(m map[string]any) { m["countries"] = []any{} },
		"bad country":   func(m map[string]any) { m["countries"] = []any{"Spain"} },
		"bad org type":  func(m map[string]any) { m["organisationType"] = "llc" },
		"empty name":    func(m map[string]any) { m["name"] = " " },
		"bad address":   func(m map[string]any) { m["businessInformation"].(map[string]any)["country"] = "Germany" },
		"empty address": func(m map[string]any) { m["businessInformation"].(map[string]any)["city"] = "" },
	} {
		in := clone(nrInputs())
		mut(in)
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(in)})
		require.NoError(t, err, name)
		require.NotEmpty(t, chk.Failures, name)
	}
	l.check(nrInputs())
}

func TestNetworkBusinessProfile_CreateRejected(t *testing.T) {
	l := newVLC(t, "NetworkBusinessProfile", "bp")
	l.m.mutate(func() { l.m.inject["POST /v1/network-registry/business-profiles"] = 422 })
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(nrInputs())})
	require.ErrorContains(t, err, "422")
	require.Empty(t, l.m.nrProfiles)
}

func TestGetNetworkBusinessProfile_FoundListAndNotFound(t *testing.T) {
	l := newVLC(t, "NetworkBusinessProfile", "bp")
	l.create(nrInputs())
	in2 := nrInputs()
	in2["name"] = "Second"
	l.create(in2)
	got, err := invoke(t, l.s, "vonage:index:getNetworkBusinessProfile", map[string]any{"profileId": "bp-2"})
	require.NoError(t, err)
	require.Equal(t, "bp-2", str(got, "profileId"))
	require.Equal(t, "Second", str(got, "name"))
	require.Equal(t, "pending", str(got, "status"))
	require.Equal(t, "Berlin", str(got.Get("businessInformation").AsMap(), "city"))
	_, err = invoke(t, l.s, "vonage:index:getNetworkBusinessProfile", map[string]any{"profileId": "nosuch"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getNetworkBusinessProfile", map[string]any{"profileId": ""})
	require.ErrorContains(t, err, "must not be empty")

	// Soft-deleted profiles are neither found nor listed.
	l.m.mutate(func() { l.m.nrProfiles["bp-1"]["delete_at"] = "2026-12-01T00:00:00Z" })
	_, err = invoke(t, l.s, "vonage:index:getNetworkBusinessProfile", map[string]any{"profileId": "bp-1"})
	require.ErrorContains(t, err, "was not found")
	list, err := invoke(t, l.s, "vonage:index:getNetworkBusinessProfiles", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 1, list.Get("profiles").AsArray().Len())
	shared, err := invoke(t, l.s, "vonage:index:getNetworkBusinessProfiles", map[string]any{"shared": true})
	require.NoError(t, err)
	require.Equal(t, 0, shared.Get("profiles").AsArray().Len())
}

func TestGetNetworkBusinessProfiles_FollowsEveryPage(t *testing.T) {
	l := newVLC(t, "NetworkBusinessProfile", "bp")
	for i := 0; i < 5; i++ {
		in := nrInputs()
		in["name"] = fmt.Sprintf("P%d", i)
		l.create(in)
	}
	l.m.mutate(func() { l.m.nrPageLimit = 2 })
	list, err := invoke(t, l.s, "vonage:index:getNetworkBusinessProfiles", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 5, list.Get("profiles").AsArray().Len())

	// A next link without a token is an error, not a guess.
	l.m.mutate(func() { l.m.nrNoNextToken = true })
	_, err = invoke(t, l.s, "vonage:index:getNetworkBusinessProfiles", map[string]any{})
	require.ErrorContains(t, err, "no next token")
}

func TestGetNetworkRegistryCatalogue_ReadsEveryList(t *testing.T) {
	l := newVLC(t, "NetworkBusinessProfile", "bp")
	l.m.mutate(func() {
		l.m.nrPageLimit = 1
		l.m.nrCatalogue = map[string][]any{
			"countries":            {map[string]any{"code": "ES", "name": "Spain"}, map[string]any{"code": "DE", "name": "Germany"}},
			"organisation-types":   {map[string]any{"id": "forProfitOrganisation", "name": "For profit"}},
			"categories":           {map[string]any{"id": "finance", "name": "Finance"}},
			"csps":                 {map[string]any{"id": "telefonica_es", "name": "Telefonica", "country": "ES", "supported_network_apis": []any{"sim-swap"}, "vetting_level": "basic"}},
			"network-apis":         {map[string]any{"id": "sim-swap", "name": "SIM Swap", "requires": []any{"verify_callback_url"}}},
			"purposes":             {map[string]any{"id": "fraud", "name": "Fraud", "network_api_id": "sim-swap"}},
			"vetting-levels":       {map[string]any{"id": "basic", "name": "Basic", "description": "d", "extra_fields": []any{"duns_number"}}},
			"terms-and-conditions": {map[string]any{"id": "tc1", "name": "Terms", "text": "Long text"}},
		}
	})
	got, err := invoke(t, l.s, "vonage:index:getNetworkRegistryCatalogue", map[string]any{"countries": []any{"es", "de"}, "all": true})
	require.NoError(t, err)
	require.Equal(t, 2, got.Get("countries").AsArray().Len(), "paged lists are followed")
	require.Equal(t, "Spain", str(got.Get("countries").AsArray().Get(0).AsMap(), "name"))
	require.Equal(t, "telefonica_es", str(got.Get("csps").AsArray().Get(0).AsMap(), "id"))
	require.Equal(t, "basic", str(got.Get("csps").AsArray().Get(0).AsMap(), "vettingLevel"))
	require.Equal(t, "sim-swap", str(got.Get("purposes").AsArray().Get(0).AsMap(), "networkApiId"))
	require.Equal(t, "verify_callback_url", got.Get("networkApis").AsArray().Get(0).AsMap().Get("requires").AsArray().Get(0).AsString())
	require.Equal(t, "duns_number", got.Get("vettingLevels").AsArray().Get(0).AsMap().Get("extraFields").AsArray().Get(0).AsString())
	require.Equal(t, "tc1", str(got.Get("termsAndConditions").AsArray().Get(0).AsMap(), "id"))
	require.Contains(t, l.m.nrCatalogueQueries, "csps?all=true&countries=ES%2CDE")

	tc, err := invoke(t, l.s, "vonage:index:getNetworkTermsAndConditions", map[string]any{"documentId": "tc1"})
	require.NoError(t, err)
	require.Equal(t, "Long text", str(tc, "text"))
	_, err = invoke(t, l.s, "vonage:index:getNetworkTermsAndConditions", map[string]any{"documentId": "nosuch"})
	require.ErrorContains(t, err, "was not found")
}
