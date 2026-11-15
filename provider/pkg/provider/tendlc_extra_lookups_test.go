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

func TestGetTenDlcBrandFeedback_Lookup(t *testing.T) {
	l := newVLC(t, "TenDlcBrand", "b")
	l.m.mutate(func() {
		l.m.tdBrands = map[string]map[string]any{
			"BPEND": {"brand_id": "BPEND", "identity_status": "PENDING"},
			"BFAIL": {"brand_id": "BFAIL", "identity_status": "UNVERIFIED"},
			"BGOOD": {"brand_id": "BGOOD", "identity_status": "VERIFIED"},
		}
	})
	r, err := invoke(t, l.s, "vonage:index:getTenDlcBrandFeedback", map[string]any{"brandId": "BFAIL"})
	require.NoError(t, err)
	cats := r.Get("categories").AsArray()
	require.Equal(t, 1, cats.Len())
	require.Equal(t, "ADDRESS", str(cats.Get(0).AsMap(), "name"))
	require.Equal(t, 2, cats.Get(0).AsMap().Get("fields").AsArray().Len())

	r, err = invoke(t, l.s, "vonage:index:getTenDlcBrandFeedback", map[string]any{"brandId": "BGOOD"})
	require.NoError(t, err)
	require.Equal(t, 0, r.Get("categories").AsArray().Len(), "a brand that is not UNVERIFIED has no feedback")

	_, err = invoke(t, l.s, "vonage:index:getTenDlcBrandFeedback", map[string]any{"brandId": "BPEND"})
	require.ErrorContains(t, err, "422")
	_, err = invoke(t, l.s, "vonage:index:getTenDlcBrandFeedback", map[string]any{"brandId": "BNONE"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getTenDlcBrandFeedback", map[string]any{"brandId": ""})
	require.ErrorContains(t, err, "brandId must be set")
}

func TestGetTenDlcBrandUseCaseQualification_Lookup(t *testing.T) {
	l := newVLC(t, "TenDlcBrand", "b")
	l.m.mutate(func() { l.m.tdBrands = map[string]map[string]any{"B1": {"brand_id": "B1"}} })
	r, err := invoke(t, l.s, "vonage:index:getTenDlcBrandUseCaseQualification", map[string]any{"brandId": "B1", "usecase": "MARKETING"})
	require.NoError(t, err)
	require.Equal(t, "MARKETING", str(r, "usecase"))
	require.Equal(t, float64(30), r.Get("quarterlyFee").AsNumber())
	require.Equal(t, float64(60), r.Get("annualFee").AsNumber())
	mnos := r.Get("mnos").AsArray()
	require.Equal(t, 2, mnos.Len())
	att := mnos.Get(0).AsMap()
	require.Equal(t, "AT&T", str(att, "mno"))
	require.True(t, att.Get("qualify").AsBool())
	require.True(t, att.Get("reqSubscriberOptIn").AsBool())
	require.False(t, att.Get("reqSubscriberHelp").AsBool(), "an explicit false is kept")
	require.Equal(t, "Q", str(att, "attMsgClass"))
	require.Equal(t, float64(2000), att.Get("attTpm").AsNumber())
	require.Equal(t, 1, att.Get("complaints").AsArray().Len())
	require.Equal(t, "SINCH", str(att.Get("complaints").AsArray().Get(0).AsMap(), "dca"))
	tmo := mnos.Get(1).AsMap()
	require.False(t, tmo.Get("qualify").AsBool())
	require.Equal(t, "LOW", str(tmo, "tmoBrandTier"))
	_, has := tmo.GetOk("mnoSupport")
	require.False(t, has, "a flag Vonage does not report stays unset")
	require.Equal(t, 0, tmo.Get("complaints").AsArray().Len())

	_, err = invoke(t, l.s, "vonage:index:getTenDlcBrandUseCaseQualification", map[string]any{"brandId": "B1", "usecase": "UNKNOWN"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getTenDlcBrandUseCaseQualification", map[string]any{"brandId": "BNONE", "usecase": "MARKETING"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getTenDlcBrandUseCaseQualification", map[string]any{"brandId": "B1", "usecase": ""})
	require.ErrorContains(t, err, "must be set")
}

func TestGetTenDlcPartnerCampaign_Lookup(t *testing.T) {
	l := newVLC(t, "TenDlcBrand", "b")
	l.m.mutate(func() {
		l.m.tdPartnerCampaigns = map[string]map[string]any{"VC1": {"campaign_id": "VC1", "sharing_status": "PENDING", "downstream_cnp_id": "D1"}}
	})
	r, err := invoke(t, l.s, "vonage:index:getTenDlcPartnerCampaign", map[string]any{"campaignId": "VC1"})
	require.NoError(t, err)
	require.Equal(t, "PENDING", str(r, "sharingStatus"))
	require.Equal(t, "D1", str(r, "downstreamCnpId"))
	require.Equal(t, "", str(r, "upstreamCnpId"))
	_, err = invoke(t, l.s, "vonage:index:getTenDlcPartnerCampaign", map[string]any{"campaignId": "NOPE"})
	require.ErrorContains(t, err, "was not found")
	_, err = invoke(t, l.s, "vonage:index:getTenDlcPartnerCampaign", map[string]any{"campaignId": ""})
	require.ErrorContains(t, err, "campaignId must be set")
}

func TestGetTenDlcMessageFlowPreview_Lookup(t *testing.T) {
	l := newVLC(t, "TenDlcBrand", "b")
	args := map[string]any{
		"brandName": "Acme", "frequency": "RECURRING", "pricingDisclosure": true,
		"privacyPolicy": "https://acme.example/privacy", "termsAndConditions": "https://acme.example/terms",
		"consentMechanisms": []any{map[string]any{"method": "TEXT_TO_JOIN", "details": "Text JOIN to 12345"}},
	}
	r, err := invoke(t, l.s, "vonage:index:getTenDlcMessageFlowPreview", args)
	require.NoError(t, err)
	require.Contains(t, str(r, "messageFlow"), "Brand: Acme")
	require.NotContains(t, str(r, "messageFlow"), "Carriers are not liable")

	args["carrierDisclaimer"] = true
	args["optInKeywords"] = "START,JOIN"
	r, err = invoke(t, l.s, "vonage:index:getTenDlcMessageFlowPreview", args)
	require.NoError(t, err)
	require.Contains(t, str(r, "messageFlow"), "Carriers are not liable")
	require.Contains(t, str(r, "messageFlow"), "Opt-in keywords: START,JOIN.")

	// Vonage's refusal (for example a text outside 40 to 4096 characters) surfaces as the error.
	args["brandName"] = "REFUSED"
	_, err = invoke(t, l.s, "vonage:index:getTenDlcMessageFlowPreview", args)
	require.ErrorContains(t, err, "422")
}
