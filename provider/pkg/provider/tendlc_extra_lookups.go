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
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// Read-only 10DLC lookups (Vonage 10DLC OpenAPI v1.0.34, read from the developer portal page on
// 2026-11-14): brand feedback, brand use case qualification, partner campaign sharing and the message
// flow preview. Shapes follow the published document and have not been exercised against a live Vonage
// account.

// ---------------------------------------------------------------------------
// getTenDlcBrandFeedback
// ---------------------------------------------------------------------------

type wireTenDlcFeedbackCategory struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Fields      []string `json:"fields"`
}

// GetTenDlcBrandFeedback reads the identity feedback of a brand.
type GetTenDlcBrandFeedback struct{}

// GetTenDlcBrandFeedbackArgs are the lookup arguments.
type GetTenDlcBrandFeedbackArgs struct {
	BrandID string `pulumi:"brandId"`
}

func (a *GetTenDlcBrandFeedbackArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "The TCR brand ID of the brand (TenDlcBrand.brandId).")
}

// TenDlcFeedbackCategoryInfo is one category of brand feedback.
type TenDlcFeedbackCategoryInfo struct {
	Name        string   `pulumi:"name"`
	Description string   `pulumi:"description"`
	Fields      []string `pulumi:"fields"`
}

func (a *TenDlcFeedbackCategoryInfo) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Name of the feedback category.")
	an.Describe(&a.Description, "What the category says about the brand.")
	an.Describe(&a.Fields, "The brand fields the feedback concerns.")
}

// GetTenDlcBrandFeedbackResult carries the feedback.
type GetTenDlcBrandFeedbackResult struct {
	Categories []TenDlcFeedbackCategoryInfo `pulumi:"categories"`
}

func (a *GetTenDlcBrandFeedbackResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Categories, "The feedback categories. Empty when the brand's identity status is not UNVERIFIED, which is the only status that has feedback.")
}

func (f *GetTenDlcBrandFeedback) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read the feedback Vonage and The Campaign Registry give on a brand that failed identity verification (GET /v1/10dlc/brands/{brand_id}/feedback). "+
		"Vonage answers with a 422 error while the identity check is still PENDING, and with an empty list unless the identity status is UNVERIFIED. Not verified against a live account.")
}

func (f *GetTenDlcBrandFeedback) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcBrandFeedbackArgs]) (infer.FunctionResponse[GetTenDlcBrandFeedbackResult], error) {
	const fn = "getTenDlcBrandFeedback"
	type resp = infer.FunctionResponse[GetTenDlcBrandFeedbackResult]
	if req.Input.BrandID == "" {
		return resp{}, fmt.Errorf("%s: brandId must be set", fn)
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	var wire struct {
		Categories []wireTenDlcFeedbackCategory `json:"categories"`
	}
	if err := c.apiJSON(ctx, http.MethodGet, tenDlcBase+"/brands/"+url.PathEscape(req.Input.BrandID)+"/feedback", nil, nil, &wire); err != nil {
		if isNotFound(err) {
			return resp{}, fmt.Errorf("%s: brand %q was not found", fn, req.Input.BrandID)
		}
		return resp{}, fmt.Errorf("%s: %w", fn, err)
	}
	out := GetTenDlcBrandFeedbackResult{Categories: []TenDlcFeedbackCategoryInfo{}}
	for _, w := range wire.Categories {
		fields := w.Fields
		if fields == nil {
			fields = []string{}
		}
		out.Categories = append(out.Categories, TenDlcFeedbackCategoryInfo{Name: w.Name, Description: w.Description, Fields: fields})
	}
	return resp{Output: out}, nil
}

// ---------------------------------------------------------------------------
// getTenDlcBrandUseCaseQualification
// ---------------------------------------------------------------------------

type wireTenDlcComplaint struct {
	Description string `json:"description"`
	DCA         string `json:"dca"`
	CreatedAt   string `json:"created_at"`
}

type wireTenDlcMnoMetadata struct {
	NetworkID           string                `json:"network_id"`
	MNO                 string                `json:"mno"`
	Status              string                `json:"status"`
	MnoSupport          *bool                 `json:"mno_support"`
	MnoReview           *bool                 `json:"mno_review"`
	Qualify             *bool                 `json:"qualify"`
	MinMsgSamples       *int                  `json:"min_msg_samples"`
	ReqSubscriberOptIn  *bool                 `json:"req_subscriber_opt_in"`
	ReqSubscriberOptOut *bool                 `json:"req_subscriber_opt_out"`
	ReqSubscriberHelp   *bool                 `json:"req_subscriber_help"`
	NoEmbeddedLink      *bool                 `json:"no_embedded_link"`
	NoEmbeddedPhone     *bool                 `json:"no_embedded_phone"`
	AttMsgClass         string                `json:"att_msg_class"`
	AttTpm              *int                  `json:"att_tpm"`
	AttMmsTpm           *int                  `json:"att_mms_tpm"`
	AttNumberBased      *bool                 `json:"att_number_based"`
	TmoBrandTier        string                `json:"tmo_brand_tier"`
	TmoTpd              *int                  `json:"tmo_tpd"`
	Complaints          []wireTenDlcComplaint `json:"complaints"`
}

// TenDlcComplaintInfo is a complaint an MNO recorded against a brand.
type TenDlcComplaintInfo struct {
	Description string `pulumi:"description"`
	Dca         string `pulumi:"dca"`
	CreatedAt   string `pulumi:"createdAt"`
}

func (a *TenDlcComplaintInfo) Annotate(an infer.Annotator) {
	an.Describe(&a.Description, "Description of the complaint.")
	an.Describe(&a.Dca, "Name of the direct connect aggregator the complaint came through.")
	an.Describe(&a.CreatedAt, "When the complaint was recorded (date-time).")
}

// TenDlcMnoQualification is what one mobile network operator requires for a use case.
type TenDlcMnoQualification struct {
	NetworkID           string                `pulumi:"networkId"`
	Mno                 string                `pulumi:"mno"`
	Status              string                `pulumi:"status"`
	MnoSupport          *bool                 `pulumi:"mnoSupport,optional"`
	MnoReview           *bool                 `pulumi:"mnoReview,optional"`
	Qualify             *bool                 `pulumi:"qualify,optional"`
	MinMsgSamples       *int                  `pulumi:"minMsgSamples,optional"`
	ReqSubscriberOptIn  *bool                 `pulumi:"reqSubscriberOptIn,optional"`
	ReqSubscriberOptOut *bool                 `pulumi:"reqSubscriberOptOut,optional"`
	ReqSubscriberHelp   *bool                 `pulumi:"reqSubscriberHelp,optional"`
	NoEmbeddedLink      *bool                 `pulumi:"noEmbeddedLink,optional"`
	NoEmbeddedPhone     *bool                 `pulumi:"noEmbeddedPhone,optional"`
	AttMsgClass         string                `pulumi:"attMsgClass"`
	AttTpm              *int                  `pulumi:"attTpm,optional"`
	AttMmsTpm           *int                  `pulumi:"attMmsTpm,optional"`
	AttNumberBased      *bool                 `pulumi:"attNumberBased,optional"`
	TmoBrandTier        string                `pulumi:"tmoBrandTier"`
	TmoTpd              *int                  `pulumi:"tmoTpd,optional"`
	Complaints          []TenDlcComplaintInfo `pulumi:"complaints"`
}

func (a *TenDlcMnoQualification) Annotate(an infer.Annotator) {
	an.Describe(&a.NetworkID, "Network ID of the operator.")
	an.Describe(&a.Mno, "Name of the operator.")
	an.Describe(&a.Status, "The operator's campaign operation status (see getTenDlcEnums, campaign_op_status). Empty when not reported.")
	an.Describe(&a.MnoSupport, "False when the operator cannot support the use case.")
	an.Describe(&a.MnoReview, "True when a campaign for the use case goes through the operator's manual review.")
	an.Describe(&a.Qualify, "False when the brand does not qualify for the use case on this operator.")
	an.Describe(&a.MinMsgSamples, "Minimum number of message samples the operator requires.")
	an.Describe(&a.ReqSubscriberOptIn, "True when the operator requires the subscriber to opt in before messages are sent.")
	an.Describe(&a.ReqSubscriberOptOut, "True when the operator requires an opt-out mechanism (STOP keywords).")
	an.Describe(&a.ReqSubscriberHelp, "True when the operator requires a help mechanism (HELP keywords).")
	an.Describe(&a.NoEmbeddedLink, "True when the operator forbids links in the messages.")
	an.Describe(&a.NoEmbeddedPhone, "True when the operator forbids phone numbers in the messages.")
	an.Describe(&a.AttMsgClass, "AT&T only: the message class the operator assigned. Empty for other operators.")
	an.Describe(&a.AttTpm, "AT&T only: SMS messages per minute qualified for the use case.")
	an.Describe(&a.AttMmsTpm, "AT&T only: MMS messages per minute qualified for the use case.")
	an.Describe(&a.AttNumberBased, "AT&T only: whether the throughput applies per number rather than per campaign.")
	an.Describe(&a.TmoBrandTier, "T-Mobile only: the brand tier that restricts the daily volume. Empty for other operators.")
	an.Describe(&a.TmoTpd, "T-Mobile only: messages per day qualified for the use case.")
	an.Describe(&a.Complaints, "Complaints the operator recorded against the brand.")
}

// GetTenDlcBrandUseCaseQualification checks whether a brand qualifies for a campaign use case.
type GetTenDlcBrandUseCaseQualification struct{}

// GetTenDlcBrandUseCaseQualificationArgs are the lookup arguments.
type GetTenDlcBrandUseCaseQualificationArgs struct {
	BrandID string `pulumi:"brandId"`
	Usecase string `pulumi:"usecase"`
}

func (a *GetTenDlcBrandUseCaseQualificationArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "The TCR brand ID of the brand (TenDlcBrand.brandId).")
	an.Describe(&a.Usecase, "The use case to check, for example MARKETING, 2FA or LOW_VOLUME (see getTenDlcEnums for the current list; it is what TenDlcCampaign.usecase takes).")
}

// GetTenDlcBrandUseCaseQualificationResult is the qualification of the brand.
type GetTenDlcBrandUseCaseQualificationResult struct {
	Usecase      string                   `pulumi:"usecase"`
	QuarterlyFee *int                     `pulumi:"quarterlyFee,optional"`
	AnnualFee    *int                     `pulumi:"annualFee,optional"`
	Mnos         []TenDlcMnoQualification `pulumi:"mnos"`
}

func (a *GetTenDlcBrandUseCaseQualificationResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Usecase, "The use case checked.")
	an.Describe(&a.QuarterlyFee, "The quarterly fee of the use case, if Vonage reports one.")
	an.Describe(&a.AnnualFee, "The annual fee of the use case, if Vonage reports one.")
	an.Describe(&a.Mnos, "What each mobile network operator requires of a campaign for the use case, and whether the brand qualifies.")
}

func (f *GetTenDlcBrandUseCaseQualification) Annotate(a infer.Annotator) {
	a.Describe(&f, "Check whether a brand qualifies to run a campaign for a use case (GET /v1/10dlc/brands/{brand_id}/usecases/{usecase}). Vonage asks a brand to check this before creating a campaign; "+
		"a use case that does not qualify may be added through third-party vetting, which is not managed here. Not verified against a live account.")
}

func (f *GetTenDlcBrandUseCaseQualification) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcBrandUseCaseQualificationArgs]) (infer.FunctionResponse[GetTenDlcBrandUseCaseQualificationResult], error) {
	const fn = "getTenDlcBrandUseCaseQualification"
	type resp = infer.FunctionResponse[GetTenDlcBrandUseCaseQualificationResult]
	if req.Input.BrandID == "" || req.Input.Usecase == "" {
		return resp{}, fmt.Errorf("%s: brandId and usecase must be set", fn)
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	var wire struct {
		Usecase      string                  `json:"usecase"`
		QuarterlyFee *int                    `json:"quarterly_fee"`
		AnnualFee    *int                    `json:"annual_fee"`
		MnoMetadata  []wireTenDlcMnoMetadata `json:"mno_metadata"`
	}
	path := tenDlcBase + "/brands/" + url.PathEscape(req.Input.BrandID) + "/usecases/" + url.PathEscape(req.Input.Usecase)
	if err := c.apiJSON(ctx, http.MethodGet, path, nil, nil, &wire); err != nil {
		if isNotFound(err) {
			return resp{}, fmt.Errorf("%s: brand %q or use case %q was not found", fn, req.Input.BrandID, req.Input.Usecase)
		}
		return resp{}, fmt.Errorf("%s: %w", fn, err)
	}
	out := GetTenDlcBrandUseCaseQualificationResult{Usecase: wire.Usecase, QuarterlyFee: wire.QuarterlyFee, AnnualFee: wire.AnnualFee, Mnos: []TenDlcMnoQualification{}}
	if out.Usecase == "" {
		out.Usecase = req.Input.Usecase
	}
	for _, w := range wire.MnoMetadata {
		m := TenDlcMnoQualification{NetworkID: w.NetworkID, Mno: w.MNO, Status: w.Status, MnoSupport: w.MnoSupport, MnoReview: w.MnoReview, Qualify: w.Qualify,
			MinMsgSamples: w.MinMsgSamples, ReqSubscriberOptIn: w.ReqSubscriberOptIn, ReqSubscriberOptOut: w.ReqSubscriberOptOut, ReqSubscriberHelp: w.ReqSubscriberHelp,
			NoEmbeddedLink: w.NoEmbeddedLink, NoEmbeddedPhone: w.NoEmbeddedPhone, AttMsgClass: w.AttMsgClass, AttTpm: w.AttTpm, AttMmsTpm: w.AttMmsTpm,
			AttNumberBased: w.AttNumberBased, TmoBrandTier: w.TmoBrandTier, TmoTpd: w.TmoTpd, Complaints: []TenDlcComplaintInfo{}}
		for _, cm := range w.Complaints {
			m.Complaints = append(m.Complaints, TenDlcComplaintInfo{Description: cm.Description, Dca: cm.DCA, CreatedAt: cm.CreatedAt})
		}
		out.Mnos = append(out.Mnos, m)
	}
	return resp{Output: out}, nil
}

// ---------------------------------------------------------------------------
// getTenDlcPartnerCampaign
// ---------------------------------------------------------------------------

// GetTenDlcPartnerCampaign reads the sharing status of a partner campaign.
type GetTenDlcPartnerCampaign struct{}

// GetTenDlcPartnerCampaignArgs are the lookup arguments.
type GetTenDlcPartnerCampaignArgs struct {
	CampaignID string `pulumi:"campaignId"`
}

func (a *GetTenDlcPartnerCampaignArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.CampaignID, "The Vonage campaign ID of the partner campaign (as returned when the campaign was imported).")
}

// GetTenDlcPartnerCampaignResult is the sharing information.
type GetTenDlcPartnerCampaignResult struct {
	CampaignID      string `pulumi:"campaignId"`
	SharingStatus   string `pulumi:"sharingStatus"`
	DownstreamCnpID string `pulumi:"downstreamCnpId"`
	UpstreamCnpID   string `pulumi:"upstreamCnpId"`
}

func (a *GetTenDlcPartnerCampaignResult) Annotate(an infer.Annotator) {
	an.Describe(&a.CampaignID, "The Vonage campaign ID.")
	an.Describe(&a.SharingStatus, "The sharing status: PENDING, ACCEPTED or DECLINED (see getTenDlcEnums for the current list).")
	an.Describe(&a.DownstreamCnpID, "The downstream connectivity partner (CNP) ID, empty when not reported.")
	an.Describe(&a.UpstreamCnpID, "The upstream connectivity partner (CNP) ID, empty when not reported.")
}

func (f *GetTenDlcPartnerCampaign) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read the sharing information of a partner campaign that was shared with Vonage (GET /v1/10dlc/partnercampaigns/{campaign_id}). Importing a partner campaign is a one-off workflow step and is not managed. Not verified against a live account.")
}

func (f *GetTenDlcPartnerCampaign) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcPartnerCampaignArgs]) (infer.FunctionResponse[GetTenDlcPartnerCampaignResult], error) {
	const fn = "getTenDlcPartnerCampaign"
	type resp = infer.FunctionResponse[GetTenDlcPartnerCampaignResult]
	if req.Input.CampaignID == "" {
		return resp{}, fmt.Errorf("%s: campaignId must be set", fn)
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	var wire struct {
		CampaignID      string `json:"campaign_id"`
		SharingStatus   string `json:"sharing_status"`
		DownstreamCnpID string `json:"downstream_cnp_id"`
		UpstreamCnpID   string `json:"upstream_cnp_id"`
	}
	if err := c.apiJSON(ctx, http.MethodGet, tenDlcBase+"/partnercampaigns/"+url.PathEscape(req.Input.CampaignID), nil, nil, &wire); err != nil {
		if isNotFound(err) {
			return resp{}, fmt.Errorf("%s: partner campaign %q was not found", fn, req.Input.CampaignID)
		}
		return resp{}, fmt.Errorf("%s: %w", fn, err)
	}
	if wire.CampaignID == "" {
		wire.CampaignID = req.Input.CampaignID
	}
	return resp{Output: GetTenDlcPartnerCampaignResult{CampaignID: wire.CampaignID, SharingStatus: wire.SharingStatus, DownstreamCnpID: wire.DownstreamCnpID, UpstreamCnpID: wire.UpstreamCnpID}}, nil
}

// ---------------------------------------------------------------------------
// getTenDlcMessageFlowPreview
// ---------------------------------------------------------------------------

// GetTenDlcMessageFlowPreview renders the message flow text of a campaign from its structured details.
type GetTenDlcMessageFlowPreview struct{}

// GetTenDlcMessageFlowPreviewArgs are the lookup arguments, the messageFlowDetails of a campaign.
type GetTenDlcMessageFlowPreviewArgs struct {
	BrandName          string                   `pulumi:"brandName"`
	ConsentMechanisms  []TenDlcConsentMechanism `pulumi:"consentMechanisms"`
	PricingDisclosure  bool                     `pulumi:"pricingDisclosure"`
	CarrierDisclaimer  *bool                    `pulumi:"carrierDisclaimer,optional"`
	Frequency          string                   `pulumi:"frequency"`
	PrivacyPolicy      string                   `pulumi:"privacyPolicy"`
	TermsAndConditions string                   `pulumi:"termsAndConditions"`
	OptInKeywords      *string                  `pulumi:"optInKeywords,optional"`
}

func (a *GetTenDlcMessageFlowPreviewArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandName, "A brand name associated with the campaign; it can differ from the name of the brand the campaign belongs to.")
	an.Describe(&a.ConsentMechanisms, "The mechanisms a customer may use to consent to the campaign.")
	an.Describe(&a.PricingDisclosure, "Must be true: adds the pricing disclosure (message and data rates may apply) to the text.")
	an.Describe(&a.CarrierDisclaimer, "When true, adds the disclaimer that carriers are not liable for delayed or undelivered messages.")
	an.Describe(&a.Frequency, "How often the campaign sends messages: ONE_TIME or RECURRING.")
	an.Describe(&a.PrivacyPolicy, "Link to the campaign's privacy policy.")
	an.Describe(&a.TermsAndConditions, "Link to the campaign's terms and conditions.")
	an.Describe(&a.OptInKeywords, "Opt-in keywords, separated by commas, up to 255 characters.")
}

// GetTenDlcMessageFlowPreviewResult is the rendered text.
type GetTenDlcMessageFlowPreviewResult struct {
	MessageFlow string `pulumi:"messageFlow"`
}

func (a *GetTenDlcMessageFlowPreviewResult) Annotate(an infer.Annotator) {
	an.Describe(&a.MessageFlow, "The message flow (call to action) text Vonage computes from the details, the same text a campaign gets from messageFlowDetails.")
}

func (f *GetTenDlcMessageFlowPreview) Annotate(a infer.Annotator) {
	a.Describe(&f, "Preview the message flow text Vonage renders from structured consent details (POST /v1/10dlc/campaigns/message_flow). It is a read-only computation that creates nothing; "+
		"Vonage answers with a 422 error when the resulting text is not 40 to 4096 characters long. Not verified against a live account.")
}

func (f *GetTenDlcMessageFlowPreview) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcMessageFlowPreviewArgs]) (infer.FunctionResponse[GetTenDlcMessageFlowPreviewResult], error) {
	const fn = "getTenDlcMessageFlowPreview"
	type resp = infer.FunctionResponse[GetTenDlcMessageFlowPreviewResult]
	in := req.Input
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	mechs := make([]map[string]any, 0, len(in.ConsentMechanisms))
	for _, m := range in.ConsentMechanisms {
		w := map[string]any{"method": m.Method, "details": m.Details}
		if m.Attachment != nil {
			w["attachment"] = *m.Attachment
		}
		mechs = append(mechs, w)
	}
	body := map[string]any{"brand_name": in.BrandName, "consent_mechanisms": mechs, "pricing_disclosure": in.PricingDisclosure,
		"frequency": in.Frequency, "privacy_policy": in.PrivacyPolicy, "terms_and_conditions": in.TermsAndConditions}
	if in.CarrierDisclaimer != nil {
		body["carrier_disclaimer"] = *in.CarrierDisclaimer
	}
	if in.OptInKeywords != nil {
		body["opt_in_keywords"] = *in.OptInKeywords
	}
	var wire struct {
		MessageFlow string `json:"message_flow"`
	}
	if err := c.apiJSON(ctx, http.MethodPost, tenDlcBase+"/campaigns/message_flow", nil, body, &wire); err != nil {
		return resp{}, fmt.Errorf("%s: %w", fn, err)
	}
	return resp{Output: GetTenDlcMessageFlowPreviewResult{MessageFlow: wire.MessageFlow}}, nil
}
