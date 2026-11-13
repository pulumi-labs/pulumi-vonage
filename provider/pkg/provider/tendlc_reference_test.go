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
	"strings"
	"testing"

	pgp "github.com/pulumi/pulumi-go-provider"
	"github.com/stretchr/testify/require"
)

func tenDlcResellerInputs() map[string]any {
	return map[string]any{"companyName": "Acme Messaging Ltd", "phone": "14155550100", "email": "ops@acme.example"}
}

func TestTenDlcReseller_Lifecycle(t *testing.T) {
	l := newVLC(t, "TenDlcReseller", "r")
	in := tenDlcResellerInputs()
	c := l.create(in)
	require.Equal(t, "res-1", c.ID)
	require.Equal(t, "res-1", str(c.Properties, "resellerId"))
	require.Equal(t, "PENDING", str(c.Properties, "status"))
	require.Equal(t, "ops@acme.example", l.m.tdResellers["res-1"]["email"])
	// A program that writes the same inputs again has no diff.
	noChanges(t, l.diff(c.ID, c.Properties, pm(in), pm(in)))

	// Review moves the status on its own; refresh follows and the inputs stay.
	l.m.mutate(func() {
		l.m.tdResellers["res-1"]["status"] = "APPROVED"
		l.m.tdResellers["res-1"]["tcr_reseller_id"] = "R123ABC"
	})
	rd := l.read(c.ID, c.Properties, pm(in))
	require.Equal(t, "APPROVED", str(rd.Properties, "status"))
	require.Equal(t, "R123ABC", str(rd.Properties, "tcrResellerId"))
	noChanges(t, l.diff(c.ID, rd.Properties, pm(in), rd.Inputs))

	// Vonage cannot update a reseller: every input replaces it.
	for k, v := range map[string]string{"companyName": "Acme Two", "phone": "14155550101", "email": "other@acme.example"} {
		in2 := clone(in)
		in2[k] = v
		require.Equal(t, pgp.UpdateReplace, diffKinds(l.diff(c.ID, rd.Properties, pm(in2), pm(in)))[k], k)
	}

	// Import adopts the stored values.
	imp := l.read("res-1", emptyMap(), emptyMap())
	require.Equal(t, "res-1", imp.ID)
	require.Equal(t, "Acme Messaging Ltd", str(imp.Inputs, "companyName"))
	require.Equal(t, "14155550100", str(imp.Inputs, "phone"))
	noChanges(t, l.diff(imp.ID, imp.Properties, imp.Inputs, imp.Inputs))
	l.check(map[string]any{"companyName": str(imp.Inputs, "companyName"), "phone": str(imp.Inputs, "phone"), "email": str(imp.Inputs, "email")})
	l.requireImportFails("res-9")

	// Destroy only forgets: the reseller stays on the account and no call is made.
	l.del(c.ID, rd.Properties)
	require.Contains(t, l.m.tdResellers, "res-1")
	// A reseller that is gone drops out of state on refresh.
	l.m.mutate(func() { delete(l.m.tdResellers, "res-1") })
	require.Empty(t, l.read(c.ID, rd.Properties, pm(in)).ID)
}

func TestTenDlcReseller_RejectedAndPreview(t *testing.T) {
	l := newVLC(t, "TenDlcReseller", "r")
	l.m.mutate(func() { l.m.tdResellerStatus = "REJECTED" })
	in := tenDlcResellerInputs()
	rej, err := l.s.Create(pgp.CreateRequest{Urn: l.urn, Properties: l.check(in)})
	require.Error(t, err)
	require.NotNil(t, rej.PartialState)
	require.Contains(t, rej.PartialState.Reasons[0], "Company name not allowed")
	require.Equal(t, "res-1", rej.ID, "a rejected reseller exists and stays in state")
	require.Len(t, l.m.tdResellers, 1, "one POST only")
	// Refresh keeps it, status REJECTED, with the reason.
	rd := l.read(rej.ID, rej.Properties, pm(in))
	require.Equal(t, "REJECTED", str(rd.Properties, "status"))
	require.Equal(t, "Company name not allowed.", str(rd.Properties, "rejectionReason"))
	// The next apply runs Update, which reports it again without registering another reseller.
	_, err = l.s.Update(pgp.UpdateRequest{ID: rej.ID, Urn: l.urn, State: rd.Properties, Inputs: pm(in)})
	require.Error(t, err)
	require.Len(t, l.m.tdResellers, 1)
	// Once Vonage approves it, Update succeeds.
	l.m.mutate(func() { l.m.tdResellers["res-1"]["status"] = "APPROVED" })
	up := l.update(rej.ID, rd.Properties, pm(in), pm(in))
	require.Equal(t, "APPROVED", str(up.Properties, "status"))

	// Preview does not call the API.
	l2 := newVLC(t, "TenDlcReseller", "r")
	d, err := l2.s.Create(pgp.CreateRequest{Urn: l2.urn, Properties: l2.check(in), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, d.ID)
	require.Empty(t, l2.m.tdResellers)

	for name, mut := range map[string]func(map[string]any){
		"empty name":   func(m map[string]any) { m["companyName"] = " " },
		"long name":    func(m map[string]any) { m["companyName"] = strings.Repeat("x", 101) },
		"plus phone":   func(m map[string]any) { m["phone"] = "+14155550100" },
		"short phone":  func(m map[string]any) { m["phone"] = "1415555" },
		"zero phone":   func(m map[string]any) { m["phone"] = "04155550100" },
		"no at":        func(m map[string]any) { m["email"] = "ops.acme.example" },
		"empty local":  func(m map[string]any) { m["email"] = "@acme.example" },
		"empty domain": func(m map[string]any) { m["email"] = "ops@" },
		"long email":   func(m map[string]any) { m["email"] = strings.Repeat("x", 100) + "@a.example" },
	} {
		bad := clone(in)
		mut(bad)
		chk, err := l.s.Check(pgp.CheckRequest{Urn: l.urn, Inputs: pm(bad)})
		require.NoError(t, err, name)
		require.NotEmpty(t, chk.Failures, name)
	}
}

func TestGetTenDlcResellers_Lookups(t *testing.T) {
	l := newVLC(t, "TenDlcReseller", "r")
	l.m.mutate(func() {
		l.m.tdPageSize = 2
		l.m.tdResellers = map[string]map[string]any{}
		for i := 1; i <= 5; i++ {
			l.m.tdResellers[fmt.Sprintf("res-%d", i)] = map[string]any{"id": fmt.Sprintf("res-%d", i), "company_name": "Co", "phone": "14155550100", "email": "a@b.example", "status": "APPROVED", "tcr_reseller_id": "R1"}
		}
		l.m.tdResellers["res-3"]["status"] = "REJECTED"
		l.m.tdResellers["res-3"]["rejection_reason"] = "no"
		delete(l.m.tdResellers["res-3"], "tcr_reseller_id")
	})
	one, err := invoke(t, l.s, "vonage:index:getTenDlcReseller", map[string]any{"resellerId": "res-3"})
	require.NoError(t, err)
	require.Equal(t, "REJECTED", str(one, "status"))
	require.Equal(t, "no", str(one, "rejectionReason"))
	require.Equal(t, "", str(one, "tcrResellerId"))
	_, err = invoke(t, l.s, "vonage:index:getTenDlcReseller", map[string]any{"resellerId": "nosuch"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getTenDlcReseller", map[string]any{"resellerId": ""})
	require.ErrorContains(t, err, "must not be empty")
	all, err := invoke(t, l.s, "vonage:index:getTenDlcResellers", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 5, all.Get("resellers").AsArray().Len(), "every page is followed")
}

func TestGetTenDlcReference_Lookups(t *testing.T) {
	l := newVLC(t, "TenDlcReseller", "r")
	terms, err := invoke(t, l.s, "vonage:index:getTenDlcCampaignTermsAndConditions", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 2, terms.Get("subTerms").AsArray().Len())

	enums, err := invoke(t, l.s, "vonage:index:getTenDlcEnums", map[string]any{})
	require.NoError(t, err)
	uc := enums.Get("usecases").AsArray()
	require.Equal(t, 2, uc.Len())
	first := uc.Get(0).AsMap()
	require.Equal(t, "CUSTOMER_CARE", str(first, "id"))
	require.Equal(t, "Customer Care", str(first, "displayName"))
	require.Equal(t, float64(2), first.Get("minMsgSamples").AsNumber())
	_, has := uc.Get(1).AsMap().GetOk("minMsgSamples")
	require.False(t, has, "a rule Vonage does not report stays unset")
	require.Equal(t, "I1", str(enums.Get("verticals").AsArray().Get(0).AsMap(), "industryId"))
	vals := enums.Get("values").AsMap()
	require.Equal(t, 2, vals.Get("brand_status").AsArray().Len())
	require.Equal(t, "10017", vals.Get("mno").AsArray().Get(0).AsString())
	require.Equal(t, 2, vals.Get("usecase").AsArray().Len())

	l.m.mutate(func() {
		l.m.tdPageSize = 2
		for i := 1; i <= 5; i++ {
			camp := "C1"
			if i > 3 {
				camp = "C2"
			}
			tp := map[string]any{"carriers": []any{"AT&T"}, "period": "minute", "scope": "campaign", "campaign_id": camp, "brand_id": "B1", "id": fmt.Sprintf("t%d", i),
				"value": map[string]any{"shared": 100 * i}}
			switch i {
			case 2:
				tp["value"] = map[string]any{"sms": 70, "mms": 30}
			case 3:
				tp["value"] = map[string]any{"unlimited": true}
			}
			l.m.tdThroughputs = append(l.m.tdThroughputs, tp)
		}
	})
	all, err := invoke(t, l.s, "vonage:index:getTenDlcThroughputs", map[string]any{})
	require.NoError(t, err)
	list := all.Get("throughputs").AsArray()
	require.Equal(t, 5, list.Len(), "page tokens are followed")
	require.Equal(t, float64(100), list.Get(0).AsMap().Get("value").AsMap().Get("shared").AsNumber())
	require.Equal(t, float64(30), list.Get(1).AsMap().Get("value").AsMap().Get("mms").AsNumber())
	require.True(t, list.Get(2).AsMap().Get("value").AsMap().Get("unlimited").AsBool())
	one, err := invoke(t, l.s, "vonage:index:getTenDlcThroughputs", map[string]any{"campaignId": "C2"})
	require.NoError(t, err)
	require.Equal(t, 2, one.Get("throughputs").AsArray().Len())
}
