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

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/stretchr/testify/require"
)

func tenDlcBrandInputs() map[string]any {
	return map[string]any{
		"entityType":  "PRIVATE_PROFIT",
		"companyName": "Acme Inc",
		"ein":         "12-3456789",
		"email":       "ops@acme.example",
		"website":     "https://acme.example",
		"vertical":    "RETAIL",
		"country":     "US",
	}
}

func TestTenDlcAccountSettings_Lifecycle(t *testing.T) {
	l := newVLC(t, "TenDlcAccountSettings", "s")
	in := map[string]any{"optOutAssist": true}
	c := l.create(in)
	require.Equal(t, "key12345", c.ID, "the ID is the account API key")
	require.True(t, l.m.tdOptOut)
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	in2 := map[string]any{"optOutAssist": false}
	d := l.diff(c.ID, c.Properties, pm(in2), pm(in))
	require.Equal(t, pgp.Update, diffKinds(d)["optOutAssist"])
	up := l.update(c.ID, c.Properties, pm(in2), pm(in))
	require.False(t, l.m.tdOptOut)
	require.False(t, up.Properties.Get("optOutAssist").AsBool())

	// Drift outside Pulumi shows on refresh; import adopts the live value.
	l.m.mutate(func() { l.m.tdOptOut = true })
	rd := l.read(c.ID, up.Properties, pm(in2))
	require.True(t, rd.Inputs.Get("optOutAssist").AsBool())
	imp := l.read("key12345", emptyMap(), emptyMap())
	require.True(t, imp.Inputs.Get("optOutAssist").AsBool())
	noChanges(t, l.diff(c.ID, imp.Properties, pm(map[string]any{"optOutAssist": true}), imp.Inputs))

	// Destroy forgets: the setting keeps its value.
	l.del(c.ID, rd.Properties)
	require.True(t, l.m.tdOptOut)
}

func TestTenDlcBrand_FullLifecycle(t *testing.T) {
	l := newVLC(t, "TenDlcBrand", "b")
	in := tenDlcBrandInputs()
	c := l.create(in)
	require.Equal(t, "B000001", c.ID)
	require.Equal(t, "B000001", str(c.Properties, "brandId"))
	require.Equal(t, "ACTIVE", str(c.Properties, "status"))
	require.Equal(t, "key12345", l.m.tdBrands["B000001"]["account_id"], "accountId defaults to the API key")
	require.Equal(t, "12-3456789", l.m.tdBrands["B000001"]["ein"])
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Refresh keeps the program's values and finds nothing to do.
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	_, ok := rd.Inputs.GetOk("accountId")
	require.False(t, ok, "an unset accountId stays unmanaged")

	// A mutable field updates in place through PATCH.
	in2 := clone(in)
	in2["companyName"] = "Acme Corp"
	in2["reseller"] = true
	d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, pgp.Update, diffKinds(d)["companyName"])
	require.Equal(t, pgp.Update, diffKinds(d)["reseller"])
	up := l.update(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, "Acme Corp", l.m.tdBrands["B000001"]["company_name"])
	require.Equal(t, "Acme Corp", str(up.Properties, "companyName"))
	require.Equal(t, "B000001", str(up.Properties, "brandId"))

	// Drift outside Pulumi shows on refresh.
	l.m.mutate(func() { l.m.tdBrands["B000001"]["email"] = "other@acme.example" })
	rd2 := l.read(c.ID, up.Properties, pm(in2))
	require.Equal(t, "other@acme.example", str(rd2.Inputs, "email"))
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, rd2.Properties, pm(in2), rd2.Inputs))["email"])

	// Fields the API cannot patch replace the brand.
	in3 := clone(in2)
	in3["universalEin"] = "99-9999999"
	in3["accountId"] = "other1"
	d3 := l.diff(c.ID, rd.Properties, pm(in3), pm(in2))
	require.Equal(t, pgp.UpdateReplace, diffKinds(d3)["universalEin"])
	require.Equal(t, pgp.UpdateReplace, diffKinds(d3)["accountId"])

	// Delete moves the brand to DELETED; it then reads as gone (Vonage answers 502 "not found").
	l.del(c.ID, rd2.Properties)
	require.Equal(t, "DELETED", l.m.tdBrands["B000001"]["status"])
	l.del(c.ID, rd2.Properties)
	gone := l.read(c.ID, rd2.Properties, rd2.Inputs)
	require.Empty(t, gone.ID, "a deleted brand reads as gone")
}

func TestTenDlcBrand_ImportAndOptionalFields(t *testing.T) {
	l := newVLC(t, "TenDlcBrand", "b")
	in := tenDlcBrandInputs()
	c := l.create(in)
	l.m.mutate(func() {
		l.m.tdBrands["B000001"]["phone"] = "+15550100"
		l.m.tdBrands["B000001"]["stock_symbol"] = "ACME"
	})
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	for _, k := range []string{"phone", "stockSymbol"} {
		_, ok := rd.Inputs.GetOk(k)
		require.False(t, ok, "%s stays unmanaged on refresh", k)
	}
	// Import adopts everything the server reports, and the imported inputs pass Check.
	imp := l.read("B000001", emptyMap(), emptyMap())
	require.Equal(t, "B000001", imp.ID)
	require.Equal(t, "PRIVATE_PROFIT", str(imp.Inputs, "entityType"))
	require.Equal(t, "+15550100", str(imp.Inputs, "phone"))
	require.Equal(t, "key12345", str(imp.Inputs, "accountId"))
	require.Equal(t, "BASIC_ACCOUNT", str(imp.Properties, "brandRelationship"))
	l.check(map[string]any{"entityType": "PRIVATE_PROFIT", "companyName": "Acme Inc", "ein": "12-3456789"})
	l.requireImportFails("B0000FF")
	l.m.mutate(func() { l.m.tdBrands["B000001"]["status"] = "DELETED" })
	l.requireImportFails("B000001")
}

func TestTenDlcBrand_CheckAndCreateRefused(t *testing.T) {
	l := newVLC(t, "TenDlcBrand", "b")
	for name, mut := range map[string]func(map[string]any){
		"bad entity type": func(m map[string]any) { m["entityType"] = "LLC" },
		"long state":      func(m map[string]any) { m["state"] = string(make([]byte, 21)) },
		"bad ein":         func(m map[string]any) { m["ein"] = "12" },
		"bad mobile":      func(m map[string]any) { m["mobilePhone"] = "+15550100" },
		"lowercase issuing country": func(m map[string]any) {
			m["einIssuingCountry"] = "us"
		},
		"bad alt id type": func(m map[string]any) { m["altBusinessIdType"] = "TAX" },
		"empty account":   func(m map[string]any) { m["accountId"] = " " },
	} {
		in := clone(tenDlcBrandInputs())
		mut(in)
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(in)})
		require.NoError(t, err, name)
		require.NotEmpty(t, chk.Failures, name)
	}
	l.check(tenDlcBrandInputs())

	// A refusal creates nothing; a preview never calls the API.
	bad := tenDlcBrandInputs()
	delete(bad, "entityType")
	_, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: pm(bad)})
	require.Error(t, err)
	require.Empty(t, l.m.tdBrands)
	d, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(tenDlcBrandInputs()), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, d.ID)
	require.Empty(t, l.m.tdBrands, "preview does not call the API")

	// Vonage refuses to delete a brand with campaigns, and the brand stays.
	c := l.create(tenDlcBrandInputs())
	l.m.mutate(func() { l.m.tdBrands["B000001"]["campaign_count"] = 2 })
	err = l.s.Delete(pgp.DeleteRequest{ID: c.ID, Urn: l.urn, Properties: c.Properties})
	require.ErrorContains(t, err, "409")
	require.Equal(t, "ACTIVE", l.m.tdBrands["B000001"]["status"])
}

func TestGetTenDlc_Lookups(t *testing.T) {
	l := newVLC(t, "TenDlcBrand", "b")
	for i := 0; i < 5; i++ {
		in := tenDlcBrandInputs()
		in["displayName"] = "Brand " + string(rune('A'+i))
		l.create(in)
	}
	got, err := invoke(t, l.s, "vonage:index:getTenDlcBrand", map[string]any{"brandId": "B000002"})
	require.NoError(t, err)
	require.Equal(t, "B000002", str(got, "brandId"))
	require.Equal(t, "Brand B", str(got, "displayName"))
	require.Equal(t, "RETAIL", str(got, "vertical"))
	_, err = invoke(t, l.s, "vonage:index:getTenDlcBrand", map[string]any{"brandId": "B0000FF"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getTenDlcBrand", map[string]any{"brandId": ""})
	require.ErrorContains(t, err, "must not be empty")

	// Every page is followed and deleted brands are neither found nor listed.
	l.m.mutate(func() { l.m.tdPageSize = 2 })
	list, err := invoke(t, l.s, "vonage:index:getTenDlcBrands", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 5, list.Get("brands").AsArray().Len())
	l.m.mutate(func() { l.m.tdBrands["B000001"]["status"] = "DELETED"; l.m.tdBrands["B000003"]["status"] = "SUSPENDED" })
	_, err = invoke(t, l.s, "vonage:index:getTenDlcBrand", map[string]any{"brandId": "B000001"})
	require.ErrorContains(t, err, "was not found")
	list, err = invoke(t, l.s, "vonage:index:getTenDlcBrands", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 4, list.Get("brands").AsArray().Len())
	sus, err := invoke(t, l.s, "vonage:index:getTenDlcBrands", map[string]any{"status": "SUSPENDED"})
	require.NoError(t, err)
	require.Equal(t, 1, sus.Get("brands").AsArray().Len())

	l.m.mutate(func() { l.m.tdOptOut = true })
	acc, err := invoke(t, l.s, "vonage:index:getTenDlcAccountSettings", map[string]any{})
	require.NoError(t, err)
	require.True(t, acc.Get("optOutAssist").AsBool())
}
