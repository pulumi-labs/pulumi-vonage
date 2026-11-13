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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// 10DLC campaigns and the numbers linked to them (Vonage 10DLC OpenAPI v1.0.34). Shapes follow the
// published document and have not been exercised against a live Vonage account.

// ---------------------------------------------------------------------------
// Wire types and client
// ---------------------------------------------------------------------------

type wireTenDlcConsentMechanism struct {
	Method     string `json:"method"`
	Details    string `json:"details"`
	Attachment string `json:"attachment,omitempty"`
}

// wireTenDlcMessageFlowDetails is the structured alternative to the free-text message_flow.
type wireTenDlcMessageFlowDetails struct {
	BrandName          string                       `json:"brand_name,omitempty"`
	ConsentMechanisms  []wireTenDlcConsentMechanism `json:"consent_mechanisms"`
	PricingDisclosure  bool                         `json:"pricing_disclosure"`
	CarrierDisclaimer  *bool                        `json:"carrier_disclaimer,omitempty"`
	Frequency          string                       `json:"frequency"`
	PrivacyPolicy      string                       `json:"privacy_policy"`
	TermsAndConditions string                       `json:"terms_and_conditions"`
}

type wireTenDlcURLSample struct {
	URL string `json:"url"`
}

// wireTenDlcCampaign is a campaign as the API returns it. The writable fields double as the create
// body; every optional one is omitted when empty.
type wireTenDlcCampaign struct {
	AccountID     string   `json:"account_id,omitempty"`
	Vertical      string   `json:"vertical,omitempty"`
	Usecase       string   `json:"usecase,omitempty"`
	SubUsecases   []string `json:"sub_usecases,omitempty"`
	ResellerID    string   `json:"reseller_id,omitempty"`
	Reseller      *bool    `json:"reseller,omitempty"`
	Description   string   `json:"description,omitempty"`
	AgeGated      *bool    `json:"age_gated,omitempty"`
	DirectLending *bool    `json:"direct_lending,omitempty"`
	SampleOne     string   `json:"sample_one,omitempty"`
	SampleTwo     string   `json:"sample_two,omitempty"`
	SampleThree   string   `json:"sample_three,omitempty"`
	SampleFour    string   `json:"sample_four,omitempty"`
	SampleFive    string   `json:"sample_five,omitempty"`
	MessageFlow   string   `json:"message_flow,omitempty"`
	// MessageFlowDetails is sent instead of MessageFlow; Vonage returns it only when it was used to create or update.
	MessageFlowDetails *wireTenDlcMessageFlowDetails `json:"message_flow_details,omitempty"`
	URLSamples         []wireTenDlcURLSample         `json:"url_samples,omitempty"`
	HelpMessage        string                        `json:"help_message,omitempty"`
	OptInMessage       string                        `json:"opt_in_message,omitempty"`
	OptOutMessage      string                        `json:"opt_out_message,omitempty"`
	HelpKeywords       string                        `json:"help_keywords,omitempty"`
	OptInKeywords      string                        `json:"opt_in_keywords,omitempty"`
	OptOutKeywords     string                        `json:"opt_out_keywords,omitempty"`
	TCAgreed           *bool                         `json:"tc_agreed,omitempty"`
	EmbeddedLink       *bool                         `json:"embedded_link,omitempty"`
	EmbeddedPhone      *bool                         `json:"embedded_phone,omitempty"`
	Label              string                        `json:"label,omitempty"`
	HIPAA              *bool                         `json:"hipaa,omitempty"`
	AutoRenewal        *bool                         `json:"auto_renewal,omitempty"`
	Status             string                        `json:"status,omitempty"`

	// Read-only.
	CampaignID       string `json:"campaign_id,omitempty"`
	BrandID          string `json:"brand_id,omitempty"`
	TCRCampaignID    string `json:"tcr_campaign_id,omitempty"`
	TrafficEnabled   *bool  `json:"traffic_enabled,omitempty"`
	TCRStatus        string `json:"tcr_status,omitempty"`
	RegistrationDate string `json:"registration_date,omitempty"`
	RenewalDate      string `json:"renewal_date,omitempty"`
	CreatedDate      string `json:"created_date,omitempty"`
	LastUpdated      string `json:"last_updated,omitempty"`
}

func tenDlcCampaignPath(brandID, campaignID string) string {
	return tenDlcBase + "/brands/" + url.PathEscape(brandID) + "/campaigns/" + url.PathEscape(campaignID)
}

// tenDlcCampaignGone reports the campaign states in which nothing can be done any more.
func tenDlcCampaignGone(status string) bool {
	switch status {
	case "TERMINATED", "CANCELED", "EXPIRED", "REJECTED", "CNP_CANCELED", "PORTED_OUT":
		return true
	}
	return false
}

// CreateTenDlcCampaign registers a campaign. The call is billed and never replayed after an unknown
// outcome (POST).
func (c *Client) CreateTenDlcCampaign(ctx context.Context, brandID string, in wireTenDlcCampaign) (*wireTenDlcCampaign, error) {
	var out wireTenDlcCampaign
	if err := c.apiJSON(ctx, http.MethodPost, tenDlcBase+"/brands/"+url.PathEscape(brandID)+"/campaigns", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTenDlcCampaign returns nil, nil when the campaign (or its brand) does not exist or the campaign
// has ended (terminated, canceled, expired, rejected).
func (c *Client) GetTenDlcCampaign(ctx context.Context, brandID, campaignID string) (*wireTenDlcCampaign, error) {
	var out wireTenDlcCampaign
	err := c.apiJSON(ctx, http.MethodGet, tenDlcCampaignPath(brandID, campaignID), nil, nil, &out)
	if tenDlcBrandGone(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if tenDlcCampaignGone(out.Status) {
		return nil, nil
	}
	return &out, nil
}

// PatchTenDlcCampaign updates a campaign and returns the result.
func (c *Client) PatchTenDlcCampaign(ctx context.Context, brandID, campaignID string, in wireTenDlcCampaign) (*wireTenDlcCampaign, error) {
	var out wireTenDlcCampaign
	if err := c.apiJSON(ctx, http.MethodPatch, tenDlcCampaignPath(brandID, campaignID), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTenDlcCampaigns returns every campaign of a brand (all pages), skipping ended ones. A missing
// brand is reported as a 404 apiError.
func (c *Client) ListTenDlcCampaigns(ctx context.Context, brandID, status string) ([]wireTenDlcCampaign, error) {
	path := tenDlcBase + "/brands/" + url.PathEscape(brandID) + "/campaigns"
	var out []wireTenDlcCampaign
	err := c.tenDlcPages(ctx, path, status, "campaigns", func(raw []byte) error {
		var list []wireTenDlcCampaign
		if err := json.Unmarshal(raw, &list); err != nil {
			return err
		}
		out = append(out, list...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	kept := out[:0]
	for _, w := range out {
		if !tenDlcCampaignGone(w.Status) {
			kept = append(kept, w)
		}
	}
	return slices.Clone(kept), nil
}

type wireTenDlcNumber struct {
	Number     string   `json:"number"`
	Country    string   `json:"country"`
	Status     string   `json:"status"`
	Compliance []string `json:"compliance"`
}

func tenDlcNumberPath(brandID, campaignID, number string) string {
	return tenDlcCampaignPath(brandID, campaignID) + "/numbers/" + url.PathEscape(number)
}

// LinkTenDlcNumber links an owned number to a campaign. Vonage accepts it asynchronously (HTTP 202).
func (c *Client) LinkTenDlcNumber(ctx context.Context, brandID, campaignID, country, number string) (*wireTenDlcNumber, error) {
	var out wireTenDlcNumber
	body := map[string]string{"country": country, "number": number}
	if err := c.apiJSON(ctx, http.MethodPost, tenDlcCampaignPath(brandID, campaignID)+"/numbers", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTenDlcNumber returns nil, nil when the number is not linked (404, or UNLINKED).
func (c *Client) GetTenDlcNumber(ctx context.Context, brandID, campaignID, number string) (*wireTenDlcNumber, error) {
	var out wireTenDlcNumber
	err := c.apiJSON(ctx, http.MethodGet, tenDlcNumberPath(brandID, campaignID, number), nil, nil, &out)
	if tenDlcBrandGone(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.Status == "UNLINKED" {
		return nil, nil
	}
	return &out, nil
}

// UnlinkTenDlcNumber unlinks a number; an absent link is not an error.
func (c *Client) UnlinkTenDlcNumber(ctx context.Context, brandID, campaignID, number string) error {
	err := c.apiJSON(ctx, http.MethodDelete, tenDlcNumberPath(brandID, campaignID, number), nil, nil, nil)
	if tenDlcBrandGone(err) {
		return nil
	}
	return err
}

// ListTenDlcNumbers returns the numbers linked to a campaign (all pages), skipping UNLINKED ones.
func (c *Client) ListTenDlcNumbers(ctx context.Context, brandID, campaignID string) ([]wireTenDlcNumber, error) {
	var out []wireTenDlcNumber
	err := c.tenDlcPages(ctx, tenDlcCampaignPath(brandID, campaignID)+"/numbers", "", "numbers", func(raw []byte) error {
		var list []wireTenDlcNumber
		if err := json.Unmarshal(raw, &list); err != nil {
			return err
		}
		for _, n := range list {
			if n.Status != "UNLINKED" {
				out = append(out, n)
			}
		}
		return nil
	})
	return out, err
}

// tenDlcPages walks a paged 10DLC listing and hands the raw array under _embedded.<key> of every page
// to each. It fails rather than guess when the response does not report total_pages and the page is full.
func (c *Client) tenDlcPages(ctx context.Context, path, status, key string, each func(raw []byte) error) error {
	for page := 1; ; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(tenDlcPageSize)}}
		if status != "" {
			q.Set("status", status)
		}
		var resp struct {
			TotalPages *int                       `json:"total_pages"`
			Embedded   map[string]json.RawMessage `json:"_embedded"`
		}
		if err := c.apiJSON(ctx, http.MethodGet, path, q, nil, &resp); err != nil {
			return err
		}
		raw := resp.Embedded[key]
		if len(raw) == 0 {
			raw = []byte("[]")
		}
		if err := each(raw); err != nil {
			return err
		}
		var n []struct{}
		_ = json.Unmarshal(raw, &n)
		if resp.TotalPages == nil {
			if len(n) >= tenDlcPageSize {
				return fmt.Errorf("vonage: GET %s: the response does not report total_pages, so the listing cannot be followed", path)
			}
			return nil
		}
		if page >= *resp.TotalPages {
			return nil
		}
	}
}

// ---------------------------------------------------------------------------
// TenDlcCampaign
// ---------------------------------------------------------------------------

// TenDlcCampaign registers a 10DLC campaign: one messaging use case of a brand.
type TenDlcCampaign struct{}

// TenDlcCampaignArgs are the inputs of a campaign. Optional inputs left unset are not managed and not sent.
type TenDlcCampaignArgs struct {
	BrandID        string   `pulumi:"brandId"`
	Usecase        string   `pulumi:"usecase"`
	Description    string   `pulumi:"description"`
	Samples        []string `pulumi:"samples"`
	HelpMessage    string   `pulumi:"helpMessage"`
	OptInMessage   string   `pulumi:"optInMessage"`
	OptOutMessage  string   `pulumi:"optOutMessage"`
	TcAgreed       bool     `pulumi:"tcAgreed"`
	AccountID      *string  `pulumi:"accountId,optional"`
	Vertical       *string  `pulumi:"vertical,optional"`
	SubUsecases    []string `pulumi:"subUsecases,optional"`
	ResellerID     *string  `pulumi:"resellerId,optional"`
	Reseller       *bool    `pulumi:"reseller,optional"`
	AgeGated       *bool    `pulumi:"ageGated,optional"`
	DirectLending  *bool    `pulumi:"directLending,optional"`
	UrlSamples     []string `pulumi:"urlSamples,optional"`
	HelpKeywords   *string  `pulumi:"helpKeywords,optional"`
	OptInKeywords  *string  `pulumi:"optInKeywords,optional"`
	OptOutKeywords *string  `pulumi:"optOutKeywords,optional"`
	EmbeddedLink   *bool    `pulumi:"embeddedLink,optional"`
	EmbeddedPhone  *bool    `pulumi:"embeddedPhone,optional"`
	Label          *string  `pulumi:"label,optional"`
	Hipaa          *bool    `pulumi:"hipaa,optional"`
	AutoRenewal    *bool    `pulumi:"autoRenewal,optional"`
	// Exactly one of MessageFlow and MessageFlowDetails is set.
	MessageFlow        *string                   `pulumi:"messageFlow,optional"`
	MessageFlowDetails *TenDlcMessageFlowDetails `pulumi:"messageFlowDetails,optional"`
}

// TenDlcConsentMechanism is one way subscribers consent to receive messages.
type TenDlcConsentMechanism struct {
	Method     string  `pulumi:"method"`
	Details    string  `pulumi:"details"`
	Attachment *string `pulumi:"attachment,optional"`
}

func (m *TenDlcConsentMechanism) Annotate(an infer.Annotator) {
	an.Describe(&m.Method, "How consent is collected: ONLINE_FORM, LIVE_OPERATOR, POINT_OF_SALE, TEXT_TO_JOIN or OTHER.")
	an.Describe(&m.Details, "Describes how consent is collected with this method.")
	an.Describe(&m.Attachment, "HTTP(S) link to evidence of the consent flow, such as a screenshot. Required for every method except TEXT_TO_JOIN.")
}

// TenDlcMessageFlowDetails is the structured description of the opt-in flow. Vonage renders it into the message flow text.
type TenDlcMessageFlowDetails struct {
	BrandName          *string                  `pulumi:"brandName,optional"`
	ConsentMechanisms  []TenDlcConsentMechanism `pulumi:"consentMechanisms"`
	PricingDisclosure  bool                     `pulumi:"pricingDisclosure"`
	CarrierDisclaimer  *bool                    `pulumi:"carrierDisclaimer,optional"`
	Frequency          string                   `pulumi:"frequency"`
	PrivacyPolicy      string                   `pulumi:"privacyPolicy"`
	TermsAndConditions string                   `pulumi:"termsAndConditions"`
}

func (d *TenDlcMessageFlowDetails) Annotate(an infer.Annotator) {
	an.Describe(&d.BrandName, "Brand name shown in the generated text. Vonage does not require it for sole proprietors.")
	an.Describe(&d.ConsentMechanisms, "At least one way subscribers consent to receive messages.")
	an.Describe(&d.PricingDisclosure, "Whether subscribers are told about message and data rates. Must be true.")
	an.Describe(&d.CarrierDisclaimer, "Whether the carriers are not liable for delayed or undelivered messages is disclosed. Only true is accepted; leave unset otherwise.")
	an.Describe(&d.Frequency, "How often messages are sent: ONE_TIME or RECURRING.")
	an.Describe(&d.PrivacyPolicy, "Link to the privacy policy.")
	an.Describe(&d.TermsAndConditions, "Link to the terms and conditions.")
}

func (a *TenDlcCampaignArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "ID of the TenDlcBrand the campaign belongs to. Changing it replaces the campaign.")
	an.Describe(&a.Usecase, "Use case: CUSTOMER_CARE, POLLING_VOTING, HIGHER_EDUCATION, PUBLIC_SERVICE_ANNOUNCEMENT, MARKETING or SECURITY_ALERT (the list can grow, see Vonage's enum endpoint). Read-only once Vonage approved the campaign, so changing it replaces the campaign.")
	an.Describe(&a.Description, "What the campaign is for and who receives the messages (40 to 4096 characters).")
	an.Describe(&a.MessageFlow, "How subscribers opt in and what they receive (40 to 4096 characters). Set either this or messageFlowDetails, not both. Vonage marks the free-text field deprecated. In the state this is always the text Vonage holds, which is rendered from messageFlowDetails when those were used.")
	an.Describe(&a.MessageFlowDetails, "Structured description of the opt-in flow, the replacement for messageFlow. Set either this or messageFlow, not both. Vonage renders it into the message flow text. Switching between the two updates the campaign in place.")
	an.Describe(&a.Samples, "One to five sample messages (20 to 1024 characters each). Vonage needs at least as many as the use case and the sub use cases require.")
	an.Describe(&a.HelpMessage, "Reply to the HELP keyword (20 to 320 characters).")
	an.Describe(&a.OptInMessage, "Confirmation sent when a subscriber opts in (20 to 320 characters).")
	an.Describe(&a.OptOutMessage, "Confirmation sent when a subscriber opts out (20 to 320 characters).")
	an.Describe(&a.TcAgreed, "Must be true: you agree to the 10DLC terms and conditions. Vonage never returns it, so it is not refreshed; an import sets it to true.")
	an.Describe(&a.AccountID, "Vonage account (API key) the campaign belongs to. Defaults to the provider's apiKey. Changing it replaces the campaign.")
	an.Describe(&a.Vertical, "Industry vertical. Read-only once the campaign is approved, so changing it replaces the campaign.")
	an.Describe(&a.SubUsecases, "Sub use cases of a MIXED-style campaign. Changing them replaces the campaign.")
	an.Describe(&a.ResellerID, "ID of the reseller the campaign is registered for (at most 8 characters).")
	an.Describe(&a.Reseller, "Whether the campaign is registered by a reseller.")
	an.Describe(&a.AgeGated, "Whether the content is age-gated. Changing it replaces the campaign.")
	an.Describe(&a.DirectLending, "Whether the campaign involves direct lending or loan arrangement. Changing it replaces the campaign.")
	an.Describe(&a.UrlSamples, "Up to 15 sample links that might be sent to subscribers.")
	an.Describe(&a.HelpKeywords, "Keywords that trigger the help message, comma separated (at most 255 characters).")
	an.Describe(&a.OptInKeywords, "Keywords that opt a subscriber in, comma separated. Needs optInMessage.")
	an.Describe(&a.OptOutKeywords, "Keywords that opt a subscriber out, comma separated.")
	an.Describe(&a.EmbeddedLink, "Whether messages contain embedded links. Changing it replaces the campaign.")
	an.Describe(&a.EmbeddedPhone, "Whether messages contain embedded phone numbers. Changing it replaces the campaign.")
	an.Describe(&a.Label, "Your own label for the campaign (at most 50 characters).")
	an.Describe(&a.Hipaa, "Whether the campaign handles HIPAA-covered data. Vonage only allows a change while no number was ever linked, so changing it replaces the campaign.")
	an.Describe(&a.AutoRenewal, "Whether the campaign renews automatically. Destroy sets it to false.")
}

// TenDlcCampaignState is the persisted state of a campaign.
type TenDlcCampaignState struct {
	TenDlcCampaignArgs

	CampaignID string `pulumi:"campaignId"`
	// Status moves on its own as Vonage and the carriers review the campaign.
	Status           string  `pulumi:"status"`
	TcrCampaignID    *string `pulumi:"tcrCampaignId,optional"`
	TcrStatus        *string `pulumi:"tcrStatus,optional"`
	TrafficEnabled   *bool   `pulumi:"trafficEnabled,optional"`
	RegistrationDate *string `pulumi:"registrationDate,optional"`
	RenewalDate      *string `pulumi:"renewalDate,optional"`
	CreatedDate      *string `pulumi:"createdDate,optional"`
	LastUpdated      *string `pulumi:"lastUpdated,optional"`
}

func (s *TenDlcCampaignState) Annotate(a infer.Annotator) {
	a.Describe(&s.CampaignID, "ID of the campaign (the resource ID is BRAND_ID/CAMPAIGN_ID).")
	a.Describe(&s.Status, "Campaign status: PENDING_REVIEW, UPDATES_REQUIRED, CARRIERS_REVIEW, ACTIVE, PENDING_CANCELLATION or SUSPENDED. It changes as Vonage and the carriers review the campaign; only ACTIVE lets traffic flow.")
	a.Describe(&s.TcrCampaignID, "ID of the campaign at The Campaign Registry, once it exists there.")
	a.Describe(&s.TcrStatus, "Status at The Campaign Registry: UNKNOWN, ACTIVE or CANCELED.")
	a.Describe(&s.TrafficEnabled, "Whether the campaign can carry traffic.")
	a.Describe(&s.RegistrationDate, "When the campaign was registered.")
	a.Describe(&s.RenewalDate, "When the campaign renews or ends.")
	a.Describe(&s.CreatedDate, "When the campaign was created.")
	a.Describe(&s.LastUpdated, "When the campaign was last updated.")
}

func (r *TenDlcCampaign) Annotate(a infer.Annotator) {
	a.Describe(&r, "A 10DLC campaign: one messaging use case of a brand, registered with The Campaign Registry through Vonage. Registration is billed, reviewed by Vonage and the carriers (status moves from PENDING_REVIEW towards ACTIVE on its own) and never retried after an unknown outcome. Vonage cannot delete a campaign: destroy turns auto-renewal off, so it ends at its renewal date, and an ended campaign reads as gone. Fields Vonage locks after approval replace the campaign when changed. The resource ID is BRAND_ID/CAMPAIGN_ID. Shapes follow the published Vonage 10DLC OpenAPI definition and were not verified against a live account.")
}

var _ infer.CustomResource[TenDlcCampaignArgs, TenDlcCampaignState] = (*TenDlcCampaign)(nil)
var _ infer.CustomCheck[TenDlcCampaignArgs] = (*TenDlcCampaign)(nil)
var _ infer.CustomRead[TenDlcCampaignArgs, TenDlcCampaignState] = (*TenDlcCampaign)(nil)
var _ infer.CustomUpdate[TenDlcCampaignArgs, TenDlcCampaignState] = (*TenDlcCampaign)(nil)
var _ infer.CustomDiff[TenDlcCampaignArgs, TenDlcCampaignState] = (*TenDlcCampaign)(nil)
var _ infer.CustomDelete[TenDlcCampaignState] = (*TenDlcCampaign)(nil)

var tenDlcNumberRe = regexp.MustCompile(`^[0-9]{7,15}$`)

func tenDlcCampaignID(brandID, campaignID string) string { return brandID + "/" + campaignID }

func splitTenDlcCampaignID(id string) (string, string, error) {
	brandID, campaignID, ok := strings.Cut(id, "/")
	if !ok || brandID == "" || campaignID == "" || strings.Contains(campaignID, "/") {
		return "", "", fmt.Errorf("invalid import ID %q: expected BRAND_ID/CAMPAIGN_ID", id)
	}
	return brandID, campaignID, nil
}

func (r *TenDlcCampaign) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[TenDlcCampaignArgs], error) {
	args, failures, err := infer.DefaultCheck[TenDlcCampaignArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[TenDlcCampaignArgs]{Inputs: args, Failures: failures}, err
	}
	fail := func(prop, reason string) { failures = append(failures, p.CheckFailure{Property: prop, Reason: reason}) }
	bound := func(prop, v string, lo, hi int) {
		if n := len(v); n < lo || n > hi {
			fail(prop, fmt.Sprintf("%s must be %d to %d characters", prop, lo, hi))
		}
	}
	if strings.TrimSpace(args.BrandID) == "" || strings.Contains(args.BrandID, "/") {
		fail("brandId", "brandId must not be empty or contain a slash")
	}
	if strings.TrimSpace(args.Usecase) == "" || len(args.Usecase) > 50 {
		fail("usecase", "usecase must be 1 to 50 characters")
	}
	bound("description", args.Description, 40, 4096)
	switch {
	case args.MessageFlow != nil && args.MessageFlowDetails != nil:
		fail("messageFlow", "set either messageFlow or messageFlowDetails, not both")
	case args.MessageFlow == nil && args.MessageFlowDetails == nil:
		fail("messageFlow", "set messageFlowDetails (or the deprecated messageFlow)")
	case args.MessageFlow != nil:
		bound("messageFlow", *args.MessageFlow, 40, 4096)
	default:
		checkTenDlcMessageFlowDetails(args.MessageFlowDetails, fail)
	}
	bound("helpMessage", args.HelpMessage, 20, 320)
	bound("optInMessage", args.OptInMessage, 20, 320)
	bound("optOutMessage", args.OptOutMessage, 20, 320)
	if !args.TcAgreed {
		fail("tcAgreed", "tcAgreed must be true: Vonage only registers a campaign once you agree to the 10DLC terms and conditions")
	}
	if len(args.Samples) < 1 || len(args.Samples) > 5 {
		fail("samples", "samples must list one to five messages")
	}
	for _, s := range args.Samples {
		if n := len(s); n < 20 || n > 1024 {
			fail("samples", "every sample must be 20 to 1024 characters")
			break
		}
	}
	if len(args.UrlSamples) > 15 {
		fail("urlSamples", "urlSamples must list at most 15 links")
	}
	for prop, f := range map[string]struct {
		v *string
		n int
	}{"helpKeywords": {args.HelpKeywords, 255}, "optInKeywords": {args.OptInKeywords, 255}, "optOutKeywords": {args.OptOutKeywords, 255},
		"label": {args.Label, 50}, "resellerId": {args.ResellerID, 8}, "accountId": {args.AccountID, 50}} {
		if f.v != nil && len(*f.v) > f.n {
			fail(prop, fmt.Sprintf("%s must be at most %d characters", prop, f.n))
		}
	}
	return infer.CheckResponse[TenDlcCampaignArgs]{Inputs: args, Failures: dropUnknownFailures(failures, req.NewInputs)}, nil
}

func checkTenDlcMessageFlowDetails(d *TenDlcMessageFlowDetails, fail func(prop, reason string)) {
	const prop = "messageFlowDetails"
	if len(d.ConsentMechanisms) == 0 {
		fail(prop, "messageFlowDetails.consentMechanisms needs at least one entry")
	}
	for _, m := range d.ConsentMechanisms {
		switch m.Method {
		case "ONLINE_FORM", "LIVE_OPERATOR", "POINT_OF_SALE", "TEXT_TO_JOIN", "OTHER":
		default:
			fail(prop, fmt.Sprintf("consentMechanisms.method %q must be ONLINE_FORM, LIVE_OPERATOR, POINT_OF_SALE, TEXT_TO_JOIN or OTHER", m.Method))
		}
		if strings.TrimSpace(m.Details) == "" {
			fail(prop, "every consentMechanisms entry needs details")
		}
		if m.Method != "TEXT_TO_JOIN" && (m.Attachment == nil || strings.TrimSpace(*m.Attachment) == "") {
			fail(prop, fmt.Sprintf("consentMechanisms.attachment is required for method %s (only TEXT_TO_JOIN may omit it)", m.Method))
		}
	}
	if !d.PricingDisclosure {
		fail(prop, "messageFlowDetails.pricingDisclosure must be true")
	}
	if d.CarrierDisclaimer != nil && !*d.CarrierDisclaimer {
		fail(prop, "messageFlowDetails.carrierDisclaimer only accepts true; leave it unset instead of false")
	}
	if d.Frequency != "ONE_TIME" && d.Frequency != "RECURRING" {
		fail(prop, "messageFlowDetails.frequency must be ONE_TIME or RECURRING")
	}
	if strings.TrimSpace(d.PrivacyPolicy) == "" {
		fail(prop, "messageFlowDetails.privacyPolicy is required")
	}
	if strings.TrimSpace(d.TermsAndConditions) == "" {
		fail(prop, "messageFlowDetails.termsAndConditions is required")
	}
}

func tenDlcDetailsWire(d *TenDlcMessageFlowDetails) *wireTenDlcMessageFlowDetails {
	if d == nil {
		return nil
	}
	w := &wireTenDlcMessageFlowDetails{BrandName: deref(d.BrandName), PricingDisclosure: d.PricingDisclosure, CarrierDisclaimer: d.CarrierDisclaimer,
		Frequency: d.Frequency, PrivacyPolicy: d.PrivacyPolicy, TermsAndConditions: d.TermsAndConditions, ConsentMechanisms: []wireTenDlcConsentMechanism{}}
	for _, m := range d.ConsentMechanisms {
		w.ConsentMechanisms = append(w.ConsentMechanisms, wireTenDlcConsentMechanism{Method: m.Method, Details: m.Details, Attachment: deref(m.Attachment)})
	}
	return w
}

func tenDlcDetailsFrom(w *wireTenDlcMessageFlowDetails) *TenDlcMessageFlowDetails {
	if w == nil {
		return nil
	}
	d := &TenDlcMessageFlowDetails{BrandName: optStr(w.BrandName), PricingDisclosure: w.PricingDisclosure, CarrierDisclaimer: w.CarrierDisclaimer,
		Frequency: w.Frequency, PrivacyPolicy: w.PrivacyPolicy, TermsAndConditions: w.TermsAndConditions, ConsentMechanisms: []TenDlcConsentMechanism{}}
	for _, m := range w.ConsentMechanisms {
		d.ConsentMechanisms = append(d.ConsentMechanisms, TenDlcConsentMechanism{Method: m.Method, Details: m.Details, Attachment: optStr(m.Attachment)})
	}
	return d
}

func sameTenDlcDetails(a, b *TenDlcMessageFlowDetails) bool {
	return reflect.DeepEqual(a, b)
}

func tenDlcSampleFields(w *wireTenDlcCampaign, samples []string) {
	dst := []*string{&w.SampleOne, &w.SampleTwo, &w.SampleThree, &w.SampleFour, &w.SampleFive}
	for i, s := range samples {
		if i < len(dst) {
			*dst[i] = s
		}
	}
}

func tenDlcSamplesFrom(w *wireTenDlcCampaign) []string {
	out := []string{}
	for _, s := range []string{w.SampleOne, w.SampleTwo, w.SampleThree, w.SampleFour, w.SampleFive} {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func tenDlcURLSamples(in []string) []wireTenDlcURLSample {
	out := make([]wireTenDlcURLSample, 0, len(in))
	for _, u := range in {
		out = append(out, wireTenDlcURLSample{URL: u})
	}
	return out
}

// tenDlcCampaignBody is the create body, and (with patch set) the PATCH body: the fields Vonage
// locks after approval are then left out.
func tenDlcCampaignBody(a TenDlcCampaignArgs, defaultAccount string, patch bool) wireTenDlcCampaign {
	w := wireTenDlcCampaign{
		Description: a.Description, MessageFlow: deref(a.MessageFlow), HelpMessage: a.HelpMessage, OptInMessage: a.OptInMessage,
		OptOutMessage: a.OptOutMessage, HelpKeywords: deref(a.HelpKeywords), OptInKeywords: deref(a.OptInKeywords),
		OptOutKeywords: deref(a.OptOutKeywords), Label: deref(a.Label), ResellerID: deref(a.ResellerID), Reseller: a.Reseller,
		AutoRenewal: a.AutoRenewal,
	}
	if a.MessageFlowDetails != nil {
		// Vonage rejects a body carrying both.
		w.MessageFlow, w.MessageFlowDetails = "", tenDlcDetailsWire(a.MessageFlowDetails)
	}
	tenDlcSampleFields(&w, a.Samples)
	if len(a.UrlSamples) > 0 {
		w.URLSamples = tenDlcURLSamples(a.UrlSamples)
	}
	if patch {
		return w
	}
	acct := deref(a.AccountID)
	if acct == "" {
		acct = defaultAccount
	}
	t := true
	w.AccountID, w.Usecase, w.Vertical, w.SubUsecases = acct, a.Usecase, deref(a.Vertical), a.SubUsecases
	w.AgeGated, w.DirectLending, w.EmbeddedLink, w.EmbeddedPhone, w.HIPAA = a.AgeGated, a.DirectLending, a.EmbeddedLink, a.EmbeddedPhone, a.Hipaa
	w.TCAgreed = &t
	return w
}

func tenDlcCampaignArgsFrom(w *wireTenDlcCampaign, brandID string, prior TenDlcCampaignArgs, importing bool) TenDlcCampaignArgs {
	samples := tenDlcSamplesFrom(w)
	if slices.Equal(samples, prior.Samples) {
		samples = prior.Samples
	}
	var urls []string
	for _, u := range w.URLSamples {
		urls = append(urls, u.URL)
	}
	urlOut := prior.UrlSamples
	if importing || prior.UrlSamples != nil {
		urlOut = urls
		if slices.Equal(urls, prior.UrlSamples) {
			urlOut = prior.UrlSamples
		}
		if len(urlOut) == 0 {
			urlOut = prior.UrlSamples
		}
	}
	subs := prior.SubUsecases
	if importing && len(w.SubUsecases) > 0 {
		subs = w.SubUsecases
	}
	// The structured details win when in use or when Vonage returns them; otherwise the text is the input.
	flow, details := optStr(w.MessageFlow), (*TenDlcMessageFlowDetails)(nil)
	switch {
	case w.MessageFlowDetails != nil:
		flow, details = nil, tenDlcDetailsFrom(w.MessageFlowDetails)
		if prior.MessageFlowDetails != nil && sameTenDlcDetails(prior.MessageFlowDetails, details) {
			details = prior.MessageFlowDetails
		}
	case prior.MessageFlowDetails != nil && !importing:
		// Vonage returns the details only when they were used to create or update; keep what was set.
		flow, details = nil, prior.MessageFlowDetails
	}
	return TenDlcCampaignArgs{
		BrandID: brandID, Usecase: w.Usecase, Description: w.Description, MessageFlow: flow, MessageFlowDetails: details, Samples: samples,
		HelpMessage: w.HelpMessage, OptInMessage: w.OptInMessage, OptOutMessage: w.OptOutMessage,
		TcAgreed:  prior.TcAgreed || importing,
		AccountID: adoptStr(prior.AccountID, w.AccountID, importing), Vertical: adoptStr(prior.Vertical, w.Vertical, importing),
		SubUsecases: subs, ResellerID: adoptStr(prior.ResellerID, w.ResellerID, importing), Reseller: adoptBool(prior.Reseller, w.Reseller, importing),
		AgeGated: adoptBool(prior.AgeGated, w.AgeGated, importing), DirectLending: adoptBool(prior.DirectLending, w.DirectLending, importing),
		UrlSamples: urlOut, HelpKeywords: adoptStr(prior.HelpKeywords, w.HelpKeywords, importing),
		OptInKeywords: adoptStr(prior.OptInKeywords, w.OptInKeywords, importing), OptOutKeywords: adoptStr(prior.OptOutKeywords, w.OptOutKeywords, importing),
		EmbeddedLink: adoptBool(prior.EmbeddedLink, w.EmbeddedLink, importing), EmbeddedPhone: adoptBool(prior.EmbeddedPhone, w.EmbeddedPhone, importing),
		Label: adoptStr(prior.Label, w.Label, importing), Hipaa: adoptBool(prior.Hipaa, w.HIPAA, importing),
		AutoRenewal: adoptBool(prior.AutoRenewal, w.AutoRenewal, importing),
	}
}

func tenDlcCampaignStateFrom(w *wireTenDlcCampaign, args TenDlcCampaignArgs) TenDlcCampaignState {
	if w.MessageFlow != "" {
		args.MessageFlow = &w.MessageFlow
	}
	return TenDlcCampaignState{
		TenDlcCampaignArgs: args, CampaignID: w.CampaignID, Status: w.Status, TcrCampaignID: optStr(w.TCRCampaignID),
		TcrStatus: optStr(w.TCRStatus), TrafficEnabled: w.TrafficEnabled, RegistrationDate: optStr(w.RegistrationDate),
		RenewalDate: optStr(w.RenewalDate), CreatedDate: optStr(w.CreatedDate), LastUpdated: optStr(w.LastUpdated),
	}
}

func (r *TenDlcCampaign) Create(ctx context.Context, req infer.CreateRequest[TenDlcCampaignArgs]) (infer.CreateResponse[TenDlcCampaignState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[TenDlcCampaignState]{Output: TenDlcCampaignState{TenDlcCampaignArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[TenDlcCampaignState]{}, err
	}
	body := tenDlcCampaignBody(a, c.apiKey, false)
	if body.AccountID == "" {
		return infer.CreateResponse[TenDlcCampaignState]{}, fmt.Errorf("create 10DLC campaign: set accountId or configure the provider's apiKey")
	}
	w, err := c.CreateTenDlcCampaign(ctx, a.BrandID, body)
	if err != nil {
		return infer.CreateResponse[TenDlcCampaignState]{}, fmt.Errorf("create 10DLC campaign for brand %s: %w", a.BrandID, err)
	}
	if w.CampaignID == "" {
		return infer.CreateResponse[TenDlcCampaignState]{}, fmt.Errorf("create 10DLC campaign for brand %s: the response carries no campaign_id", a.BrandID)
	}
	return infer.CreateResponse[TenDlcCampaignState]{ID: tenDlcCampaignID(a.BrandID, w.CampaignID), Output: tenDlcCampaignStateFrom(w, a)}, nil
}

func (r *TenDlcCampaign) Read(ctx context.Context, req infer.ReadRequest[TenDlcCampaignArgs, TenDlcCampaignState]) (infer.ReadResponse[TenDlcCampaignArgs, TenDlcCampaignState], error) {
	type resp = infer.ReadResponse[TenDlcCampaignArgs, TenDlcCampaignState]
	brandID, campaignID, err := splitTenDlcCampaignID(req.ID)
	if err != nil {
		return resp{}, err
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	w, err := c.GetTenDlcCampaign(ctx, brandID, campaignID)
	if err != nil {
		return resp{}, fmt.Errorf("read 10DLC campaign %s: %w", req.ID, err)
	}
	if w == nil {
		if err := notFoundOnImport("10DLC campaign", req.ID, req.Inputs, req.State); err != nil {
			return resp{}, err
		}
		return resp{}, nil
	}
	if w.CampaignID == "" {
		w.CampaignID = campaignID
	}
	args := tenDlcCampaignArgsFrom(w, brandID, req.Inputs, isZeroImport(req.Inputs, req.State))
	return resp{ID: req.ID, Inputs: args, State: tenDlcCampaignStateFrom(w, args)}, nil
}

func (r *TenDlcCampaign) Diff(ctx context.Context, req infer.DiffRequest[TenDlcCampaignArgs, TenDlcCampaignState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.TenDlcCampaignArgs
	mark := func(name string, changed bool, kind p.DiffKind) {
		if changed {
			d[name] = p.PropertyDiff{Kind: kind}
		}
	}
	optS := func(a, b *string) bool { return a != nil && (b == nil || *a != *b) }
	optB := func(a, b *bool) bool { return a != nil && (b == nil || *a != *b) }
	// Fixed by the campaign's identity or locked by Vonage after approval.
	mark("brandId", in.BrandID != old.BrandID, p.UpdateReplace)
	mark("usecase", in.Usecase != old.Usecase, p.UpdateReplace)
	mark("accountId", optS(in.AccountID, old.AccountID), p.UpdateReplace)
	mark("vertical", optS(in.Vertical, old.Vertical), p.UpdateReplace)
	mark("subUsecases", in.SubUsecases != nil && !sameStringSet(in.SubUsecases, old.SubUsecases), p.UpdateReplace)
	mark("ageGated", optB(in.AgeGated, old.AgeGated), p.UpdateReplace)
	mark("directLending", optB(in.DirectLending, old.DirectLending), p.UpdateReplace)
	mark("embeddedLink", optB(in.EmbeddedLink, old.EmbeddedLink), p.UpdateReplace)
	mark("embeddedPhone", optB(in.EmbeddedPhone, old.EmbeddedPhone), p.UpdateReplace)
	mark("hipaa", optB(in.Hipaa, old.Hipaa), p.UpdateReplace)
	// Changeable in place.
	mark("description", in.Description != old.Description, p.Update)
	mark("messageFlow", optS(in.MessageFlow, old.MessageFlow), p.Update)
	mark("messageFlowDetails", in.MessageFlowDetails != nil && !sameTenDlcDetails(in.MessageFlowDetails, old.MessageFlowDetails), p.Update)
	mark("samples", !slices.Equal(in.Samples, old.Samples), p.Update)
	mark("helpMessage", in.HelpMessage != old.HelpMessage, p.Update)
	mark("optInMessage", in.OptInMessage != old.OptInMessage, p.Update)
	mark("optOutMessage", in.OptOutMessage != old.OptOutMessage, p.Update)
	mark("resellerId", optS(in.ResellerID, old.ResellerID), p.Update)
	mark("reseller", optB(in.Reseller, old.Reseller), p.Update)
	mark("urlSamples", in.UrlSamples != nil && !slices.Equal(in.UrlSamples, old.UrlSamples), p.Update)
	mark("helpKeywords", optS(in.HelpKeywords, old.HelpKeywords), p.Update)
	mark("optInKeywords", optS(in.OptInKeywords, old.OptInKeywords), p.Update)
	mark("optOutKeywords", optS(in.OptOutKeywords, old.OptOutKeywords), p.Update)
	mark("label", optS(in.Label, old.Label), p.Update)
	mark("autoRenewal", optB(in.AutoRenewal, old.AutoRenewal), p.Update)
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func sameStringSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func (r *TenDlcCampaign) Update(ctx context.Context, req infer.UpdateRequest[TenDlcCampaignArgs, TenDlcCampaignState]) (infer.UpdateResponse[TenDlcCampaignState], error) {
	a := req.Inputs
	if req.DryRun {
		st := req.State
		st.TenDlcCampaignArgs = a
		return infer.UpdateResponse[TenDlcCampaignState]{Output: st}, nil
	}
	brandID, campaignID, err := splitTenDlcCampaignID(req.ID)
	if err != nil {
		return infer.UpdateResponse[TenDlcCampaignState]{}, err
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[TenDlcCampaignState]{}, err
	}
	w, err := c.PatchTenDlcCampaign(ctx, brandID, campaignID, tenDlcCampaignBody(a, "", true))
	if err != nil {
		return infer.UpdateResponse[TenDlcCampaignState]{}, fmt.Errorf("update 10DLC campaign %s: %w", req.ID, err)
	}
	if w.CampaignID == "" {
		w.CampaignID = campaignID
	}
	return infer.UpdateResponse[TenDlcCampaignState]{Output: tenDlcCampaignStateFrom(w, a)}, nil
}

// Delete turns auto-renewal off: Vonage cannot delete a campaign before it expires.
func (r *TenDlcCampaign) Delete(ctx context.Context, req infer.DeleteRequest[TenDlcCampaignState]) (infer.DeleteResponse, error) {
	brandID, campaignID, err := splitTenDlcCampaignID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	// An ended campaign cannot be updated any more (Vonage answers 422): there is nothing left to end.
	cur, err := c.GetTenDlcCampaign(ctx, brandID, campaignID)
	if err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("end 10DLC campaign %s: %w", req.ID, err)
	}
	if cur == nil {
		return infer.DeleteResponse{}, nil
	}
	off := false
	_, err = c.PatchTenDlcCampaign(ctx, brandID, campaignID, wireTenDlcCampaign{AutoRenewal: &off})
	if tenDlcBrandGone(err) {
		return infer.DeleteResponse{}, nil
	}
	if err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("end 10DLC campaign %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// TenDlcCampaignNumber
// ---------------------------------------------------------------------------

// TenDlcCampaignNumber links one owned number to a 10DLC campaign.
type TenDlcCampaignNumber struct{}

// TenDlcCampaignNumberArgs are the inputs of a link. The API can only link and unlink, so changing any
// of them replaces the link.
type TenDlcCampaignNumberArgs struct {
	BrandID    string `pulumi:"brandId"`
	CampaignID string `pulumi:"campaignId"`
	Country    string `pulumi:"country"`
	Number     string `pulumi:"number"`
}

func (a *TenDlcCampaignNumberArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "ID of the brand that owns the campaign. Changing it replaces the link.")
	an.Describe(&a.CampaignID, "ID of the campaign (the campaignId of a TenDlcCampaign). Changing it replaces the link.")
	an.Describe(&a.Country, "Two-letter ISO 3166 country code of the number, in capitals. Changing it replaces the link.")
	an.Describe(&a.Number, "The Vonage number to link, digits only with the country code (7 to 15 digits). It must be owned by the account. Changing it replaces the link.")
}

// TenDlcCampaignNumberState is the persisted state of a link.
type TenDlcCampaignNumberState struct {
	TenDlcCampaignNumberArgs

	// Status is LINKING, LINKED or UNLINKING; only LINKED allows traffic.
	Status     string   `pulumi:"status"`
	Compliance []string `pulumi:"compliance"`
}

func (s *TenDlcCampaignNumberState) Annotate(a infer.Annotator) {
	a.Describe(&s.Status, "Link status: LINKING, LINKED or UNLINKING. Only LINKED allows traffic. It moves on its own; a link Vonage rejects fails the create.")
	a.Describe(&s.Compliance, "Compliance findings Vonage reports for the number, if any.")
}

func (r *TenDlcCampaignNumber) Annotate(a infer.Annotator) {
	a.Describe(&r, "Links an owned Vonage number to a 10DLC campaign so it may send campaign traffic. Vonage accepts the link asynchronously (HTTP 202), so the status starts as LINKING; the create then reads the link back and, if Vonage rejected it, reports the resource as partially created (it stays in state, so the next apply or destroy handles it). The resource ID is BRAND_ID/CAMPAIGN_ID/NUMBER. Every input replaces the link, and destroy unlinks the number. Shapes follow the published Vonage 10DLC OpenAPI definition and were not verified against a live account.")
}

var _ infer.CustomResource[TenDlcCampaignNumberArgs, TenDlcCampaignNumberState] = (*TenDlcCampaignNumber)(nil)
var _ infer.CustomCheck[TenDlcCampaignNumberArgs] = (*TenDlcCampaignNumber)(nil)
var _ infer.CustomRead[TenDlcCampaignNumberArgs, TenDlcCampaignNumberState] = (*TenDlcCampaignNumber)(nil)
var _ infer.CustomDiff[TenDlcCampaignNumberArgs, TenDlcCampaignNumberState] = (*TenDlcCampaignNumber)(nil)
var _ infer.CustomDelete[TenDlcCampaignNumberState] = (*TenDlcCampaignNumber)(nil)

func tenDlcNumberID(a TenDlcCampaignNumberArgs) string {
	return a.BrandID + "/" + a.CampaignID + "/" + a.Number
}

func (r *TenDlcCampaignNumber) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[TenDlcCampaignNumberArgs], error) {
	args, failures, err := infer.DefaultCheck[TenDlcCampaignNumberArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[TenDlcCampaignNumberArgs]{Inputs: args, Failures: failures}, err
	}
	for prop, v := range map[string]string{"brandId": args.BrandID, "campaignId": args.CampaignID} {
		if strings.TrimSpace(v) == "" || strings.Contains(v, "/") {
			failures = append(failures, p.CheckFailure{Property: prop, Reason: prop + " must not be empty or contain a slash"})
		}
	}
	if !isCountryCode(args.Country) || args.Country != strings.ToUpper(args.Country) {
		failures = append(failures, p.CheckFailure{Property: "country", Reason: "country must be a two-letter ISO 3166 country code in capitals"})
	}
	if !tenDlcNumberRe.MatchString(args.Number) {
		failures = append(failures, p.CheckFailure{Property: "number", Reason: "number must be 7 to 15 digits, with no plus sign or spaces"})
	}
	return infer.CheckResponse[TenDlcCampaignNumberArgs]{Inputs: args, Failures: dropUnknownFailures(failures, req.NewInputs)}, nil
}

func tenDlcNumberState(a TenDlcCampaignNumberArgs, w *wireTenDlcNumber) TenDlcCampaignNumberState {
	comp := w.Compliance
	if comp == nil {
		comp = []string{}
	}
	return TenDlcCampaignNumberState{TenDlcCampaignNumberArgs: a, Status: w.Status, Compliance: comp}
}

func (r *TenDlcCampaignNumber) Create(ctx context.Context, req infer.CreateRequest[TenDlcCampaignNumberArgs]) (infer.CreateResponse[TenDlcCampaignNumberState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[TenDlcCampaignNumberState]{Output: TenDlcCampaignNumberState{TenDlcCampaignNumberArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[TenDlcCampaignNumberState]{}, err
	}
	if _, err := c.LinkTenDlcNumber(ctx, a.BrandID, a.CampaignID, a.Country, a.Number); err != nil {
		return infer.CreateResponse[TenDlcCampaignNumberState]{}, fmt.Errorf("link number %s to 10DLC campaign %s/%s: %w", a.Number, a.BrandID, a.CampaignID, err)
	}
	// The link is accepted asynchronously (202), so it exists from here on: read it back and, when that
	// fails or Vonage rejected it, keep the ID (ResourceInitFailedError) so the next apply or destroy
	// manages the link instead of posting it again.
	id, st := tenDlcNumberID(a), TenDlcCampaignNumberState{TenDlcCampaignNumberArgs: a, Status: "LINKING", Compliance: []string{}}
	w, err := c.GetTenDlcNumber(ctx, a.BrandID, a.CampaignID, a.Number)
	if err != nil {
		return infer.CreateResponse[TenDlcCampaignNumberState]{ID: id, Output: st},
			infer.ResourceInitFailedError{Reasons: []string{fmt.Sprintf("number linked but confirming the link failed: %v", err)}}
	}
	if w == nil {
		w = &wireTenDlcNumber{Number: a.Number, Country: a.Country, Status: "LINKING"}
	}
	if w.Status == "REJECTED" {
		return infer.CreateResponse[TenDlcCampaignNumberState]{ID: id, Output: tenDlcNumberState(a, w)},
			infer.ResourceInitFailedError{Reasons: []string{fmt.Sprintf("Vonage rejected the link of number %s to campaign %s/%s (compliance: %s)", a.Number, a.BrandID, a.CampaignID, strings.Join(w.Compliance, ", "))}}
	}
	return infer.CreateResponse[TenDlcCampaignNumberState]{ID: tenDlcNumberID(a), Output: tenDlcNumberState(a, w)}, nil
}

func (r *TenDlcCampaignNumber) Read(ctx context.Context, req infer.ReadRequest[TenDlcCampaignNumberArgs, TenDlcCampaignNumberState]) (infer.ReadResponse[TenDlcCampaignNumberArgs, TenDlcCampaignNumberState], error) {
	type resp = infer.ReadResponse[TenDlcCampaignNumberArgs, TenDlcCampaignNumberState]
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return resp{}, fmt.Errorf("invalid import ID %q: expected BRAND_ID/CAMPAIGN_ID/NUMBER", req.ID)
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	w, err := c.GetTenDlcNumber(ctx, parts[0], parts[1], parts[2])
	if err != nil {
		return resp{}, fmt.Errorf("read 10DLC campaign number %s: %w", req.ID, err)
	}
	if w == nil {
		if err := notFoundOnImport("10DLC campaign number", req.ID, req.Inputs, req.State); err != nil {
			return resp{}, err
		}
		return resp{}, nil
	}
	country := w.Country
	if country == "" {
		country = req.Inputs.Country
	}
	args := TenDlcCampaignNumberArgs{BrandID: parts[0], CampaignID: parts[1], Country: country, Number: parts[2]}
	return resp{ID: req.ID, Inputs: args, State: tenDlcNumberState(args, w)}, nil
}

func (r *TenDlcCampaignNumber) Diff(ctx context.Context, req infer.DiffRequest[TenDlcCampaignNumberArgs, TenDlcCampaignNumberState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.TenDlcCampaignNumberArgs
	for name, changed := range map[string]bool{"brandId": in.BrandID != old.BrandID, "campaignId": in.CampaignID != old.CampaignID,
		"country": in.Country != old.Country, "number": in.Number != old.Number} {
		if changed {
			d[name] = p.PropertyDiff{Kind: p.UpdateReplace}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *TenDlcCampaignNumber) Delete(ctx context.Context, req infer.DeleteRequest[TenDlcCampaignNumberState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	s := req.State
	if err := c.UnlinkTenDlcNumber(ctx, s.BrandID, s.CampaignID, s.Number); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("unlink number %s from 10DLC campaign %s/%s: %w", s.Number, s.BrandID, s.CampaignID, err)
	}
	return infer.DeleteResponse{}, nil
}
