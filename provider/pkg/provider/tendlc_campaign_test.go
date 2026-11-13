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
	"strings"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/stretchr/testify/require"
)

const (
	tdSample1 = "Acme: your appointment is confirmed for Monday at 10am. Reply STOP to opt out."
	tdSample2 = "Acme: your appointment moved to Tuesday at 11am. Reply STOP to opt out."
)

func tenDlcCampaignInputs(brandID string) map[string]any {
	return map[string]any{
		"brandId":       brandID,
		"usecase":       "CUSTOMER_CARE",
		"description":   "Appointment confirmations and reminders for existing customers of Acme.",
		"messageFlow":   "Customers give their number when booking and tick a box to receive reminders by SMS.",
		"samples":       []any{tdSample1, tdSample2},
		"helpMessage":   "Acme: for help email support@acme.example or call 555-0100.",
		"optInMessage":  "Acme: you are subscribed to appointment reminders. Reply STOP to opt out.",
		"optOutMessage": "Acme: you have been unsubscribed and will receive no more messages.",
		"tcAgreed":      true,
	}
}

// newCampaignLC returns a campaign lifecycle with one brand B000001 registered on the fake.
func newCampaignLC(t *testing.T, typ string) *vlc {
	l := newVLC(t, typ, "c")
	l.m.mutate(func() {
		l.m.tdBrands = map[string]map[string]any{"B000001": {"brand_id": "B000001", "status": "ACTIVE", "entity_type": "PRIVATE_PROFIT", "account_id": "key12345"}}
		l.m.tdNext = 1
	})
	return l
}

func TestTenDlcCampaign_FullLifecycle(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaign")
	in := tenDlcCampaignInputs("B000001")
	c := l.create(in)
	require.Equal(t, "B000001/C1", c.ID)
	require.Equal(t, "C1", str(c.Properties, "campaignId"))
	require.Equal(t, "PENDING_REVIEW", str(c.Properties, "status"))
	wire := l.m.tdCampaigns["B000001/C1"]
	require.Equal(t, "key12345", wire["account_id"], "accountId defaults to the API key")
	require.Equal(t, tdSample1, wire["sample_one"])
	require.Equal(t, tdSample2, wire["sample_two"])
	require.Nil(t, wire["sample_three"])
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Refresh keeps the program's values; tcAgreed is never returned and stays true.
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	require.True(t, rd.Inputs.Get("tcAgreed").AsBool())
	require.Equal(t, 2, rd.Inputs.Get("samples").AsArray().Len())

	// Mutable fields update in place through PATCH, which never carries the locked fields.
	in2 := clone(in)
	in2["description"] = "Appointment confirmations, reminders and rescheduling notices for Acme customers."
	in2["samples"] = []any{tdSample1}
	in2["label"] = "reminders"
	in2["autoRenewal"] = true
	d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, pgp.Update, diffKinds(d)["description"])
	require.Equal(t, pgp.Update, diffKinds(d)["samples"])
	require.Equal(t, pgp.Update, diffKinds(d)["label"])
	l.m.mutate(func() { l.m.tdCampaigns["B000001/C1"]["status"] = "ACTIVE" })
	up := l.update(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, "reminders", l.m.tdCampaigns["B000001/C1"]["label"])
	require.Equal(t, "ACTIVE", str(up.Properties, "status"))

	// Locked fields replace the campaign.
	in3 := clone(in2)
	in3["usecase"] = "MARKETING"
	in3["vertical"] = "RETAIL"
	in3["hipaa"] = true
	d3 := l.diff(c.ID, up.Properties, pm(in3), pm(in2))
	for _, k := range []string{"usecase", "vertical", "hipaa"} {
		require.Equal(t, pgp.UpdateReplace, diffKinds(d3)[k], k)
	}

	// Drift outside Pulumi shows on refresh.
	l.m.mutate(func() {
		l.m.tdCampaigns["B000001/C1"]["help_message"] = "Acme: new help text that was set outside Pulumi."
	})
	rd2 := l.read(c.ID, up.Properties, pm(in2))
	require.Contains(t, str(rd2.Inputs, "helpMessage"), "outside Pulumi")
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, rd2.Properties, pm(in2), rd2.Inputs))["helpMessage"])

	// Destroy turns auto-renewal off (the campaign cannot be deleted); a campaign that has ended reads as gone.
	l.del(c.ID, rd.Properties)
	require.Equal(t, false, l.m.tdCampaigns["B000001/C1"]["auto_renewal"])
	still := l.read(c.ID, up.Properties, pm(in2))
	require.Equal(t, "B000001/C1", still.ID, "a campaign pending cancellation still exists")
	l.m.mutate(func() { l.m.tdCampaigns["B000001/C1"]["status"] = "TERMINATED" })
	gone := l.read(c.ID, up.Properties, pm(in2))
	require.Empty(t, gone.ID)
	l.del(c.ID, rd.Properties)
}

func TestTenDlcCampaign_ImportOptionalAndCheck(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaign")
	in := tenDlcCampaignInputs("B000001")
	c := l.create(in)
	l.m.mutate(func() {
		l.m.tdCampaigns["B000001/C1"]["label"] = "server label"
		l.m.tdCampaigns["B000001/C1"]["opt_out_keywords"] = "STOP,END"
		l.m.tdCampaigns["B000001/C1"]["url_samples"] = []any{map[string]any{"url": "https://acme.example/a"}}
	})
	rd := l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	for _, k := range []string{"label", "optOutKeywords", "urlSamples"} {
		_, ok := rd.Inputs.GetOk(k)
		require.False(t, ok, "%s stays unmanaged on refresh", k)
	}
	imp := l.read("B000001/C1", emptyMap(), emptyMap())
	require.Equal(t, "B000001/C1", imp.ID)
	require.Equal(t, "CUSTOMER_CARE", str(imp.Inputs, "usecase"))
	require.Equal(t, "server label", str(imp.Inputs, "label"))
	require.Equal(t, "STOP,END", str(imp.Inputs, "optOutKeywords"))
	require.True(t, imp.Inputs.Get("tcAgreed").AsBool())
	require.Equal(t, 1, imp.Inputs.Get("urlSamples").AsArray().Len())
	// The imported inputs pass Check.
	l.check(map[string]any{
		"brandId": "B000001", "usecase": str(imp.Inputs, "usecase"), "description": str(imp.Inputs, "description"),
		"messageFlow": str(imp.Inputs, "messageFlow"), "samples": []any{tdSample1, tdSample2}, "helpMessage": str(imp.Inputs, "helpMessage"),
		"optInMessage": str(imp.Inputs, "optInMessage"), "optOutMessage": str(imp.Inputs, "optOutMessage"), "tcAgreed": true,
	})
	l.requireImportFails("B000001/nosuch")
	l.requireImportFails("B000009/C1")
	_, err := l.s.Read(pgp.ReadRequest{ID: "noslash", Urn: l.urn, Properties: emptyMap(), Inputs: emptyMap()})
	require.ErrorContains(t, err, "BRAND_ID/CAMPAIGN_ID")

	for name, mut := range map[string]func(map[string]any){
		"short description": func(m map[string]any) { m["description"] = "too short" },
		"no samples":        func(m map[string]any) { m["samples"] = []any{} },
		"six samples": func(m map[string]any) {
			m["samples"] = []any{tdSample1, tdSample1, tdSample1, tdSample1, tdSample1, tdSample1}
		},
		"short sample":  func(m map[string]any) { m["samples"] = []any{"short"} },
		"no agreement":  func(m map[string]any) { m["tcAgreed"] = false },
		"empty brand":   func(m map[string]any) { m["brandId"] = "" },
		"slash brand":   func(m map[string]any) { m["brandId"] = "a/b" },
		"long label":    func(m map[string]any) { m["label"] = strings.Repeat("x", 51) },
		"long reseller": func(m map[string]any) { m["resellerId"] = strings.Repeat("x", 9) },
	} {
		bad := clone(tenDlcCampaignInputs("B000001"))
		mut(bad)
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(bad)})
		require.NoError(t, err, name)
		require.NotEmpty(t, chk.Failures, name)
	}
}

func TestTenDlcCampaign_CreateRefusedAndPreview(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaign")
	d, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(tenDlcCampaignInputs("B000001")), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, d.ID)
	require.Empty(t, l.m.tdCampaigns, "preview does not call the API")
	// An unknown brand is refused by Vonage and nothing is tracked.
	_, err = l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(tenDlcCampaignInputs("B000009"))})
	require.ErrorContains(t, err, "404")
	require.Empty(t, l.m.tdCampaigns)
}

func TestTenDlcCampaignNumber_Lifecycle(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaignNumber")
	cl := newVLC(t, "TenDlcCampaign", "c")
	cl.m = l.m
	in := map[string]any{"brandId": "B000001", "campaignId": "C1", "country": "US", "number": "14155550100"}
	l.m.mutate(func() {
		l.m.tdCampaigns = map[string]map[string]any{"B000001/C1": {"campaign_id": "C1", "status": "ACTIVE"}}
	})
	c := l.create(in)
	require.Equal(t, "B000001/C1/14155550100", c.ID)
	require.Equal(t, "LINKING", str(c.Properties, "status"))
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// The link settles on its own; refresh follows.
	l.m.mutate(func() { l.m.tdNumbers["B000001/C1/14155550100"]["status"] = "LINKED" })
	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, "LINKED", str(rd.Properties, "status"))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Every input replaces the link.
	in2 := clone(in)
	in2["number"] = "14155550101"
	require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, rd.Properties, pm(in2), pm(in)))["number"])
	in3 := clone(in)
	in3["campaignId"] = "C2"
	require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, rd.Properties, pm(in3), pm(in)))["campaignId"])

	// Import adopts the link; an unlinked or missing number is not found.
	imp := l.read(c.ID, emptyMap(), emptyMap())
	require.Equal(t, "US", str(imp.Inputs, "country"))
	require.Equal(t, "14155550100", str(imp.Inputs, "number"))
	l.requireImportFails("B000001/C1/14155550999")
	_, err := l.s.Read(pgp.ReadRequest{ID: "B000001/C1", Urn: l.urn, Properties: emptyMap(), Inputs: emptyMap()})
	require.ErrorContains(t, err, "BRAND_ID/CAMPAIGN_ID/NUMBER")

	// Destroy unlinks; the link then reads as gone, and deleting again is quiet.
	l.del(c.ID, rd.Properties)
	require.Equal(t, "UNLINKED", l.m.tdNumbers["B000001/C1/14155550100"]["status"])
	gone := l.read(c.ID, rd.Properties, rd.Inputs)
	require.Empty(t, gone.ID)
	l.del(c.ID, rd.Properties)
}

func TestTenDlcCampaignNumber_RejectedCheckAndPreview(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaignNumber")
	l.m.mutate(func() {
		l.m.tdCampaigns = map[string]map[string]any{"B000001/C1": {"campaign_id": "C1", "status": "ACTIVE"}}
		l.m.tdRejectLinks = true
	})
	in := map[string]any{"brandId": "B000001", "campaignId": "C1", "country": "US", "number": "14155550100"}
	rej, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in)})
	require.Error(t, err)
	require.NotNil(t, rej.PartialState)
	require.Contains(t, rej.PartialState.Reasons[0], "rejected")
	require.Equal(t, "B000001/C1/14155550100", rej.ID, "a rejected link stays in state so destroy can unlink it")
	d, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, d.ID)
	for name, mut := range map[string]func(map[string]any){
		"plus sign":      func(m map[string]any) { m["number"] = "+14155550100" },
		"short number":   func(m map[string]any) { m["number"] = "12345" },
		"lowercase":      func(m map[string]any) { m["country"] = "us" },
		"long country":   func(m map[string]any) { m["country"] = "USA" },
		"empty campaign": func(m map[string]any) { m["campaignId"] = "" },
		"slash brand":    func(m map[string]any) { m["brandId"] = "a/b" },
	} {
		bad := clone(in)
		mut(bad)
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(bad)})
		require.NoError(t, err, name)
		require.NotEmpty(t, chk.Failures, name)
	}
	// An unknown campaign is refused and nothing is tracked.
	l.m.mutate(func() { l.m.tdRejectLinks = false })
	bad := clone(in)
	bad["campaignId"] = "C9"
	_, err = l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(bad)})
	require.ErrorContains(t, err, "404")
}

func TestGetTenDlcCampaign_Lookups(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaign")
	for i := 0; i < 4; i++ {
		in := tenDlcCampaignInputs("B000001")
		in["label"] = "c" + string(rune('a'+i))
		l.create(in)
	}
	got, err := invoke(t, l.s, "vonage:index:getTenDlcCampaign", map[string]any{"brandId": "B000001", "campaignId": "C2"})
	require.NoError(t, err)
	require.Equal(t, "C2", str(got, "campaignId"))
	require.Equal(t, "cb", str(got, "label"))
	require.Equal(t, 2, got.Get("samples").AsArray().Len())
	_, err = invoke(t, l.s, "vonage:index:getTenDlcCampaign", map[string]any{"brandId": "B000001", "campaignId": "nosuch"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getTenDlcCampaign", map[string]any{"brandId": "", "campaignId": "C1"})
	require.ErrorContains(t, err, "must not be empty")

	l.m.mutate(func() { l.m.tdPageSize = 3; l.m.tdCampaigns["B000001/C1"]["status"] = "TERMINATED" })
	_, err = invoke(t, l.s, "vonage:index:getTenDlcCampaign", map[string]any{"brandId": "B000001", "campaignId": "C1"})
	require.ErrorContains(t, err, "was not found")
	list, err := invoke(t, l.s, "vonage:index:getTenDlcCampaigns", map[string]any{"brandId": "B000001"})
	require.NoError(t, err)
	require.Equal(t, 3, list.Get("campaigns").AsArray().Len(), "pages are followed and ended campaigns skipped")
	act, err := invoke(t, l.s, "vonage:index:getTenDlcCampaigns", map[string]any{"brandId": "B000001", "status": "PENDING_REVIEW"})
	require.NoError(t, err)
	require.Equal(t, 3, act.Get("campaigns").AsArray().Len())
	_, err = invoke(t, l.s, "vonage:index:getTenDlcCampaigns", map[string]any{"brandId": "B000009"})
	require.ErrorContains(t, err, "was not found")

	nl := newVLC(t, "TenDlcCampaignNumber", "n")
	nl.m = l.m
	l.m.mutate(func() {
		l.m.tdNumbers = map[string]map[string]any{
			"B000001/C2/14155550100": {"number": "14155550100", "country": "US", "status": "LINKED", "compliance": []any{"ok"}},
			"B000001/C2/14155550101": {"number": "14155550101", "country": "US", "status": "LINKING"},
			"B000001/C2/14155550102": {"number": "14155550102", "country": "US", "status": "UNLINKED"},
			"B000001/C2/14155550103": {"number": "14155550103", "country": "US", "status": "LINKED"},
		}
	})
	n, err := invoke(t, l.s, "vonage:index:getTenDlcCampaignNumber", map[string]any{"brandId": "B000001", "campaignId": "C2", "number": "14155550100"})
	require.NoError(t, err)
	require.Equal(t, "LINKED", str(n, "status"))
	require.Equal(t, 1, n.Get("compliance").AsArray().Len())
	_, err = invoke(t, l.s, "vonage:index:getTenDlcCampaignNumber", map[string]any{"brandId": "B000001", "campaignId": "C2", "number": "14155550102"})
	require.ErrorContains(t, err, "is not linked")
	_, err = invoke(t, l.s, "vonage:index:getTenDlcCampaignNumber", map[string]any{"brandId": "B000001", "campaignId": "C2", "number": ""})
	require.ErrorContains(t, err, "must not be empty")
	ns, err := invoke(t, l.s, "vonage:index:getTenDlcCampaignNumbers", map[string]any{"brandId": "B000001", "campaignId": "C2"})
	require.NoError(t, err)
	require.Equal(t, 3, ns.Get("numbers").AsArray().Len(), "unlinked numbers are skipped and pages followed")
}

func tenDlcDetailsInputs() map[string]any {
	return map[string]any{
		"brandName": "Acme",
		"consentMechanisms": []any{
			map[string]any{"method": "ONLINE_FORM", "details": "Customers tick a box on the booking form.", "attachment": "https://acme.example/booking.png"},
			map[string]any{"method": "TEXT_TO_JOIN", "details": "Customers text JOIN to 55555."},
		},
		"pricingDisclosure":  true,
		"frequency":          "RECURRING",
		"privacyPolicy":      "https://acme.example/privacy",
		"termsAndConditions": "https://acme.example/terms",
	}
}

func tenDlcDetailsCampaignInputs() map[string]any {
	in := clone(tenDlcCampaignInputs("B000001"))
	delete(in, "messageFlow")
	in["messageFlowDetails"] = tenDlcDetailsInputs()
	return in
}

func TestTenDlcCampaign_MessageFlowDetailsLifecycle(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaign")
	in := tenDlcDetailsCampaignInputs()
	c := l.create(in)
	require.Equal(t, "B000001/C1", c.ID)
	// Only the details go on the wire, never both fields.
	require.Len(t, l.m.tdBodies, 1)
	require.Nil(t, l.m.tdBodies[0]["message_flow"])
	det, ok := l.m.tdBodies[0]["message_flow_details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "RECURRING", det["frequency"])
	require.Equal(t, true, det["pricing_disclosure"])
	require.Nil(t, det["carrier_disclaimer"])
	require.Len(t, det["consent_mechanisms"], 2)
	// The state carries the text Vonage rendered.
	require.Contains(t, str(c.Properties, "messageFlow"), "Rendered from details")
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Refresh: Vonage returns the rendered text; the program's details stay, no diff, and messageFlow remains unset in the inputs.
	rd := l.read(c.ID, c.Properties, pm(in))
	_, has := rd.Inputs.GetOk("messageFlow")
	require.False(t, has)
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))
	// The same holds when Vonage returns no details at all.
	l.m.mutate(func() { delete(l.m.tdCampaigns["B000001/C1"], "message_flow_details") })
	rd = l.read(c.ID, c.Properties, pm(in))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Changing the details updates in place.
	in2 := clone(in)
	d2 := clone(tenDlcDetailsInputs())
	d2["frequency"] = "ONE_TIME"
	in2["messageFlowDetails"] = d2
	d := l.diff(c.ID, rd.Properties, pm(in2), pm(in))
	require.Equal(t, pgp.Update, diffKinds(d)["messageFlowDetails"])
	up := l.update(c.ID, rd.Properties, pm(in2), pm(in))
	require.Contains(t, str(up.Properties, "messageFlow"), "ONE_TIME")
	require.Len(t, l.m.tdBodies, 2)
	require.Nil(t, l.m.tdBodies[1]["message_flow"])
	noChanges(t, l.diff(c.ID, up.Properties, pm(in2), pm(in2)))

	// Switching to the deprecated text updates in place and drops the details.
	in3 := clone(tenDlcCampaignInputs("B000001"))
	d3 := l.diff(c.ID, up.Properties, pm(in3), pm(in2))
	require.Equal(t, pgp.Update, diffKinds(d3)["messageFlow"])
	up3 := l.update(c.ID, up.Properties, pm(in3), pm(in2))
	require.Nil(t, l.m.tdBodies[2]["message_flow_details"])
	require.Equal(t, str(pm(in3), "messageFlow"), str(up3.Properties, "messageFlow"))
	noChanges(t, l.diff(c.ID, up3.Properties, pm(in3), pm(in3)))

	// And back to details.
	d4 := l.diff(c.ID, up3.Properties, pm(in2), pm(in3))
	require.Equal(t, pgp.Update, diffKinds(d4)["messageFlowDetails"])
	up4 := l.update(c.ID, up3.Properties, pm(in2), pm(in3))
	noChanges(t, l.diff(c.ID, up4.Properties, pm(in2), pm(in2)))
}

func TestTenDlcCampaign_MessageFlowDetailsImportAndDrift(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaign")
	in := tenDlcDetailsCampaignInputs()
	c := l.create(in)
	// Import while Vonage returns the details: they are adopted and the text is not.
	imp := l.read(c.ID, emptyMap(), emptyMap())
	require.Equal(t, "RECURRING", str(imp.Inputs.Get("messageFlowDetails").AsMap(), "frequency"))
	_, has := imp.Inputs.GetOk("messageFlow")
	require.False(t, has)
	require.Equal(t, 2, imp.Inputs.Get("messageFlowDetails").AsMap().Get("consentMechanisms").AsArray().Len())
	noChanges(t, l.diff(c.ID, imp.Properties, imp.Inputs, imp.Inputs))
	// The imported inputs pass Check.
	l.check(map[string]any{
		"brandId": "B000001", "usecase": "CUSTOMER_CARE", "description": str(imp.Inputs, "description"), "messageFlowDetails": tenDlcDetailsInputs(),
		"samples": []any{tdSample1, tdSample2}, "helpMessage": str(imp.Inputs, "helpMessage"), "optInMessage": str(imp.Inputs, "optInMessage"),
		"optOutMessage": str(imp.Inputs, "optOutMessage"), "tcAgreed": true,
	})
	// Import when Vonage returns only the text: the text becomes the input.
	l.m.mutate(func() { delete(l.m.tdCampaigns["B000001/C1"], "message_flow_details") })
	imp2 := l.read(c.ID, emptyMap(), emptyMap())
	require.Contains(t, str(imp2.Inputs, "messageFlow"), "Rendered from details")
	_, has = imp2.Inputs.GetOk("messageFlowDetails")
	require.False(t, has)
	// Details changed outside Pulumi show as drift on refresh.
	l.m.mutate(func() {
		l.m.tdCampaigns["B000001/C1"]["message_flow_details"] = map[string]any{
			"consent_mechanisms": []any{map[string]any{"method": "OTHER", "details": "Changed elsewhere.", "attachment": "https://acme.example/x.png"}},
			"pricing_disclosure": true, "frequency": "ONE_TIME", "privacy_policy": "https://acme.example/privacy", "terms_and_conditions": "https://acme.example/terms",
		}
	})
	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, pgp.Update, diffKinds(l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))["messageFlowDetails"])
}

func TestTenDlcCampaign_MessageFlowCheck(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaign")
	both := tenDlcDetailsCampaignInputs()
	both["messageFlow"] = "Customers give their number when booking and tick a box to receive reminders by SMS."
	neither := tenDlcDetailsCampaignInputs()
	delete(neither, "messageFlowDetails")
	short := tenDlcCampaignInputs("B000001")
	short["messageFlow"] = "too short"
	cases := map[string]map[string]any{"both": both, "neither": neither, "short text": short}
	with := func(name string, mut func(d map[string]any)) {
		in := tenDlcDetailsCampaignInputs()
		d := clone(tenDlcDetailsInputs())
		mut(d)
		in["messageFlowDetails"] = d
		cases[name] = in
	}
	with("no consent", func(d map[string]any) { d["consentMechanisms"] = []any{} })
	with("bad method", func(d map[string]any) {
		d["consentMechanisms"] = []any{map[string]any{"method": "CARRIER_PIGEON", "details": "x", "attachment": "https://a.example/x"}}
	})
	with("no attachment", func(d map[string]any) {
		d["consentMechanisms"] = []any{map[string]any{"method": "ONLINE_FORM", "details": "Customers tick a box."}}
	})
	with("no mechanism details", func(d map[string]any) {
		d["consentMechanisms"] = []any{map[string]any{"method": "TEXT_TO_JOIN", "details": " "}}
	})
	with("pricing false", func(d map[string]any) { d["pricingDisclosure"] = false })
	with("disclaimer false", func(d map[string]any) { d["carrierDisclaimer"] = false })
	with("bad frequency", func(d map[string]any) { d["frequency"] = "DAILY" })
	with("no privacy policy", func(d map[string]any) { d["privacyPolicy"] = "" })
	with("no terms", func(d map[string]any) { d["termsAndConditions"] = "" })
	for name, in := range cases {
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(in)})
		require.NoError(t, err, name)
		require.NotEmpty(t, chk.Failures, name)
		want := "messageFlowDetails"
		if name == "both" || name == "neither" || name == "short text" {
			want = "messageFlow"
		}
		for _, f := range chk.Failures {
			require.Equal(t, want, f.Property, "%s: %s", name, f.Reason)
		}
	}
	// A TEXT_TO_JOIN mechanism needs no attachment and carrierDisclaimer may be true.
	ok := tenDlcDetailsCampaignInputs()
	d := clone(tenDlcDetailsInputs())
	d["carrierDisclaimer"] = true
	delete(d, "brandName")
	d["consentMechanisms"] = []any{map[string]any{"method": "TEXT_TO_JOIN", "details": "Customers text JOIN."}}
	ok["messageFlowDetails"] = d
	l.check(ok)
}

func TestTenDlcCampaignNumber_ConfirmReadFailureKeepsID(t *testing.T) {
	l := newCampaignLC(t, "TenDlcCampaignNumber")
	l.m.mutate(func() {
		l.m.tdCampaigns = map[string]map[string]any{"B000001/C1": {"campaign_id": "C1", "status": "ACTIVE"}}
		l.m.tdFailNumberGet = true
	})
	in := map[string]any{"brandId": "B000001", "campaignId": "C1", "country": "US", "number": "14155550100"}
	res, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in)})
	require.Error(t, err)
	require.NotNil(t, res.PartialState)
	require.Equal(t, "B000001/C1/14155550100", res.ID, "the link was accepted, so the ID is kept for the next apply or destroy")
	require.Contains(t, res.PartialState.Reasons[0], "confirming the link failed")
	require.Contains(t, l.m.tdNumbers, "B000001/C1/14155550100")
}
