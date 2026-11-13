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
	"regexp"
	"slices"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// 10DLC resellers and reference data (Vonage 10DLC OpenAPI v1.0.34). Shapes follow the published
// document and have not been exercised against a live Vonage account.

// ---------------------------------------------------------------------------
// Wire types and client
// ---------------------------------------------------------------------------

type wireTenDlcReseller struct {
	ID              string `json:"id,omitempty"`
	TCRResellerID   string `json:"tcr_reseller_id,omitempty"`
	CompanyName     string `json:"company_name,omitempty"`
	Phone           string `json:"phone,omitempty"`
	Email           string `json:"email,omitempty"`
	Status          string `json:"status,omitempty"`
	RejectionReason string `json:"rejection_reason,omitempty"`
}

func tenDlcResellerPath(id string) string { return tenDlcBase + "/resellers/" + url.PathEscape(id) }

// CreateTenDlcReseller registers a reseller. Vonage offers no way to update or delete one, so the call
// is never replayed after an unknown outcome (POST).
func (c *Client) CreateTenDlcReseller(ctx context.Context, in wireTenDlcReseller) (*wireTenDlcReseller, error) {
	var out wireTenDlcReseller
	if err := c.apiJSON(ctx, http.MethodPost, tenDlcBase+"/resellers", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTenDlcReseller returns nil, nil when the reseller does not exist.
func (c *Client) GetTenDlcReseller(ctx context.Context, id string) (*wireTenDlcReseller, error) {
	var out wireTenDlcReseller
	err := c.apiJSON(ctx, http.MethodGet, tenDlcResellerPath(id), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.ID == "" {
		out.ID = id
	}
	return &out, nil
}

// ListTenDlcResellers returns every reseller of the account (all pages).
func (c *Client) ListTenDlcResellers(ctx context.Context) ([]wireTenDlcReseller, error) {
	var out []wireTenDlcReseller
	err := c.tenDlcPages(ctx, tenDlcBase+"/resellers", "", "resellers", func(raw []byte) error {
		var list []wireTenDlcReseller
		if err := json.Unmarshal(raw, &list); err != nil {
			return err
		}
		out = append(out, list...)
		return nil
	})
	return out, err
}

type wireTenDlcThroughputValue struct {
	Shared    *float64 `json:"shared"`
	SMS       *float64 `json:"sms"`
	MMS       *float64 `json:"mms"`
	Unlimited *bool    `json:"unlimited"`
}

type wireTenDlcThroughput struct {
	ID         string                    `json:"id"`
	BrandID    string                    `json:"brand_id"`
	CampaignID string                    `json:"campaign_id"`
	Carriers   []string                  `json:"carriers"`
	Value      wireTenDlcThroughputValue `json:"value"`
	Period     string                    `json:"period"`
	Scope      string                    `json:"scope"`
}

// ListTenDlcThroughputs follows the page tokens of the throughput listing until Vonage stops handing one out.
func (c *Client) ListTenDlcThroughputs(ctx context.Context, campaignID string) ([]wireTenDlcThroughput, error) {
	var out []wireTenDlcThroughput
	seen := map[string]bool{}
	token := ""
	for {
		q := url.Values{"page_size": {fmt.Sprint(tenDlcPageSize)}}
		if token != "" {
			q.Set("page_token", token)
		}
		if campaignID != "" {
			q.Set("campaign_id", campaignID)
		}
		var resp struct {
			NextPage string `json:"next_page"`
			Embedded struct {
				Throughputs []wireTenDlcThroughput `json:"throughputs"`
			} `json:"_embedded"`
		}
		if err := c.apiJSON(ctx, http.MethodGet, tenDlcBase+"/throughputs", q, nil, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Embedded.Throughputs...)
		if resp.NextPage == "" {
			return out, nil
		}
		if seen[resp.NextPage] {
			return nil, fmt.Errorf("vonage: GET %s/throughputs: the page token %q repeats, so the listing cannot be followed", tenDlcBase, resp.NextPage)
		}
		seen[resp.NextPage] = true
		token = resp.NextPage
	}
}

// GetTenDlcEnums returns the raw enumeration lists, keyed as the API names them.
func (c *Client) GetTenDlcEnums(ctx context.Context) (map[string][]map[string]any, error) {
	var out map[string][]map[string]any
	if err := c.apiJSON(ctx, http.MethodGet, tenDlcBase+"/enum", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTenDlcCampaignTerms returns the terms and conditions a campaign owner agrees to with tcAgreed.
func (c *Client) GetTenDlcCampaignTerms(ctx context.Context) ([]string, error) {
	var out struct {
		SubTerms []string `json:"sub_terms"`
	}
	if err := c.apiJSON(ctx, http.MethodGet, tenDlcBase+"/campaigns/terms_and_conditions", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.SubTerms, nil
}

// ---------------------------------------------------------------------------
// TenDlcReseller
// ---------------------------------------------------------------------------

// TenDlcReseller registers a 10DLC reseller. Vonage has no call to update or delete one.
type TenDlcReseller struct{}

// TenDlcResellerArgs are the inputs of a reseller. Vonage cannot change a reseller, so every one replaces it.
type TenDlcResellerArgs struct {
	CompanyName string `pulumi:"companyName"`
	Phone       string `pulumi:"phone"`
	Email       string `pulumi:"email"`
}

func (a *TenDlcResellerArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.CompanyName, "Company name of the reseller (at most 100 characters). Changing it replaces the reseller.")
	an.Describe(&a.Phone, "Mobile phone number of the reseller contact in E.164 format without the + prefix, for example 14155550100. Changing it replaces the reseller.")
	an.Describe(&a.Email, "Email address of the reseller contact (at most 100 characters). Changing it replaces the reseller.")
}

// TenDlcResellerState is the persisted state of a reseller.
type TenDlcResellerState struct {
	TenDlcResellerArgs

	ResellerID string `pulumi:"resellerId"`
	// Status moves from PENDING to APPROVED or REJECTED as the reseller is reviewed.
	Status          string  `pulumi:"status"`
	TcrResellerID   *string `pulumi:"tcrResellerId,optional"`
	RejectionReason *string `pulumi:"rejectionReason,optional"`
}

func (s *TenDlcResellerState) Annotate(a infer.Annotator) {
	a.Describe(&s.ResellerID, "ID of the reseller (also the resource ID); this is the resellerId a TenDlcCampaign takes.")
	a.Describe(&s.Status, "Review status: PENDING, APPROVED or REJECTED. It changes on its own as the reseller is reviewed.")
	a.Describe(&s.TcrResellerID, "ID of the reseller at The Campaign Registry; only present once the reseller is APPROVED.")
	a.Describe(&s.RejectionReason, "Why the reseller was rejected; only present when the status is REJECTED.")
}

func (r *TenDlcReseller) Annotate(a infer.Annotator) {
	a.Describe(&r, "A 10DLC reseller, registered with Vonage and The Campaign Registry. Vonage has no call to update or delete a reseller: every input replaces it (a new reseller is registered) and destroy only removes it from Pulumi state, the reseller stays registered on the account. Registration is reviewed (status moves from PENDING to APPROVED or REJECTED), possibly billed, and never retried after an unknown outcome. A REJECTED reseller is kept in state with its status and rejection reason. Import with the reseller ID. Shapes follow the published Vonage 10DLC OpenAPI definition and were not verified against a live account.")
}

var _ infer.CustomResource[TenDlcResellerArgs, TenDlcResellerState] = (*TenDlcReseller)(nil)
var _ infer.CustomCheck[TenDlcResellerArgs] = (*TenDlcReseller)(nil)
var _ infer.CustomRead[TenDlcResellerArgs, TenDlcResellerState] = (*TenDlcReseller)(nil)
var _ infer.CustomUpdate[TenDlcResellerArgs, TenDlcResellerState] = (*TenDlcReseller)(nil)
var _ infer.CustomDiff[TenDlcResellerArgs, TenDlcResellerState] = (*TenDlcReseller)(nil)
var _ infer.CustomDelete[TenDlcResellerState] = (*TenDlcReseller)(nil)

var tenDlcResellerPhoneRe = regexp.MustCompile(`^[1-9][0-9]{7,14}$`)

func (r *TenDlcReseller) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[TenDlcResellerArgs], error) {
	args, failures, err := infer.DefaultCheck[TenDlcResellerArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[TenDlcResellerArgs]{Inputs: args, Failures: failures}, err
	}
	fail := func(prop, reason string) { failures = append(failures, p.CheckFailure{Property: prop, Reason: reason}) }
	if n := len(strings.TrimSpace(args.CompanyName)); n < 1 || len(args.CompanyName) > 100 {
		fail("companyName", "companyName must be 1 to 100 characters")
	}
	if !tenDlcResellerPhoneRe.MatchString(args.Phone) {
		fail("phone", "phone must be an E.164 number without the + prefix (8 to 15 digits, not starting with 0)")
	}
	if at := strings.Index(args.Email, "@"); at < 1 || at == len(args.Email)-1 || len(args.Email) > 100 || strings.ContainsAny(args.Email, " \t") {
		fail("email", "email must be an email address of at most 100 characters")
	}
	return infer.CheckResponse[TenDlcResellerArgs]{Inputs: args, Failures: dropUnknownFailures(failures, req.NewInputs)}, nil
}

func tenDlcResellerState(w *wireTenDlcReseller, args TenDlcResellerArgs) TenDlcResellerState {
	return TenDlcResellerState{TenDlcResellerArgs: args, ResellerID: w.ID, Status: w.Status, TcrResellerID: optStr(w.TCRResellerID), RejectionReason: optStr(w.RejectionReason)}
}

// tenDlcResellerRejected is the partial-creation error of a reseller Vonage rejected: it exists on the
// account (and cannot be removed), so it stays in state with its reason.
func tenDlcResellerRejected(w *wireTenDlcReseller) error {
	reason := w.RejectionReason
	if reason == "" {
		reason = "no reason given"
	}
	return infer.ResourceInitFailedError{Reasons: []string{fmt.Sprintf("Vonage rejected reseller %s: %s", w.ID, reason)}}
}

func (r *TenDlcReseller) Create(ctx context.Context, req infer.CreateRequest[TenDlcResellerArgs]) (infer.CreateResponse[TenDlcResellerState], error) {
	a := req.Inputs
	type resp = infer.CreateResponse[TenDlcResellerState]
	if req.DryRun {
		return resp{Output: TenDlcResellerState{TenDlcResellerArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	w, err := c.CreateTenDlcReseller(ctx, wireTenDlcReseller{CompanyName: a.CompanyName, Phone: a.Phone, Email: a.Email})
	if err != nil {
		return resp{}, fmt.Errorf("create 10DLC reseller %q: %w", a.CompanyName, err)
	}
	if w.ID == "" {
		return resp{}, fmt.Errorf("create 10DLC reseller %q: the response carries no id", a.CompanyName)
	}
	out := resp{ID: w.ID, Output: tenDlcResellerState(w, a)}
	if w.Status == "REJECTED" {
		return out, tenDlcResellerRejected(w)
	}
	return out, nil
}

func (r *TenDlcReseller) Read(ctx context.Context, req infer.ReadRequest[TenDlcResellerArgs, TenDlcResellerState]) (infer.ReadResponse[TenDlcResellerArgs, TenDlcResellerState], error) {
	type resp = infer.ReadResponse[TenDlcResellerArgs, TenDlcResellerState]
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	w, err := c.GetTenDlcReseller(ctx, req.ID)
	if err != nil {
		return resp{}, fmt.Errorf("read 10DLC reseller %s: %w", req.ID, err)
	}
	if w == nil {
		if err := notFoundOnImport("10DLC reseller", req.ID, req.Inputs, req.State); err != nil {
			return resp{}, err
		}
		return resp{}, nil
	}
	args := TenDlcResellerArgs{CompanyName: w.CompanyName, Phone: w.Phone, Email: w.Email}
	return resp{ID: req.ID, Inputs: args, State: tenDlcResellerState(w, args)}, nil
}

func (r *TenDlcReseller) Diff(ctx context.Context, req infer.DiffRequest[TenDlcResellerArgs, TenDlcResellerState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.TenDlcResellerArgs
	// Vonage has no update call: every change registers a new reseller.
	if in.CompanyName != old.CompanyName {
		d["companyName"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.Phone != old.Phone {
		d["phone"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if in.Email != old.Email {
		d["email"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

// Update never changes anything at Vonage (every input replaces the reseller). It runs after a partial
// creation, to re-read the reseller: a reseller still REJECTED keeps reporting that.
func (r *TenDlcReseller) Update(ctx context.Context, req infer.UpdateRequest[TenDlcResellerArgs, TenDlcResellerState]) (infer.UpdateResponse[TenDlcResellerState], error) {
	type resp = infer.UpdateResponse[TenDlcResellerState]
	st := req.State
	st.TenDlcResellerArgs = req.Inputs
	if req.DryRun {
		return resp{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	w, err := c.GetTenDlcReseller(ctx, req.ID)
	if err != nil {
		return resp{}, fmt.Errorf("read 10DLC reseller %s: %w", req.ID, err)
	}
	if w == nil {
		return resp{}, fmt.Errorf("10DLC reseller %s no longer exists; run pulumi refresh", req.ID)
	}
	out := resp{Output: tenDlcResellerState(w, req.Inputs)}
	if w.Status == "REJECTED" {
		return out, tenDlcResellerRejected(w)
	}
	return out, nil
}

// Delete only forgets the reseller: Vonage offers no call to remove one.
func (r *TenDlcReseller) Delete(ctx context.Context, req infer.DeleteRequest[TenDlcResellerState]) (infer.DeleteResponse, error) {
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// getTenDlcReseller, getTenDlcResellers
// ---------------------------------------------------------------------------

// TenDlcResellerInfo describes a reseller; fields Vonage does not report are empty.
type TenDlcResellerInfo struct {
	ResellerID      string `pulumi:"resellerId"`
	TcrResellerID   string `pulumi:"tcrResellerId"`
	CompanyName     string `pulumi:"companyName"`
	Phone           string `pulumi:"phone"`
	Email           string `pulumi:"email"`
	Status          string `pulumi:"status"`
	RejectionReason string `pulumi:"rejectionReason"`
}

func (a *TenDlcResellerInfo) Annotate(an infer.Annotator) {
	an.Describe(&a.ResellerID, "ID of the reseller.")
	an.Describe(&a.TcrResellerID, "ID at The Campaign Registry; empty until the reseller is APPROVED.")
	an.Describe(&a.CompanyName, "Company name of the reseller.")
	an.Describe(&a.Phone, "Contact phone number, E.164 without the + prefix.")
	an.Describe(&a.Email, "Contact email address.")
	an.Describe(&a.Status, "Review status: PENDING, APPROVED or REJECTED.")
	an.Describe(&a.RejectionReason, "Why the reseller was rejected; empty otherwise.")
}

func tenDlcResellerInfo(w wireTenDlcReseller) TenDlcResellerInfo {
	return TenDlcResellerInfo{ResellerID: w.ID, TcrResellerID: w.TCRResellerID, CompanyName: w.CompanyName, Phone: w.Phone, Email: w.Email,
		Status: w.Status, RejectionReason: w.RejectionReason}
}

// GetTenDlcReseller looks up one reseller.
type GetTenDlcReseller struct{}

// GetTenDlcResellerArgs are the lookup arguments.
type GetTenDlcResellerArgs struct {
	ResellerID string `pulumi:"resellerId"`
}

func (a *GetTenDlcResellerArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.ResellerID, "ID of the reseller.")
}

func (f *GetTenDlcReseller) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a 10DLC reseller by ID.")
}

func (f *GetTenDlcReseller) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcResellerArgs]) (infer.FunctionResponse[TenDlcResellerInfo], error) {
	if req.Input.ResellerID == "" {
		return infer.FunctionResponse[TenDlcResellerInfo]{}, fmt.Errorf("getTenDlcReseller: resellerId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[TenDlcResellerInfo]{}, err
	}
	w, err := c.GetTenDlcReseller(ctx, req.Input.ResellerID)
	if err != nil {
		return infer.FunctionResponse[TenDlcResellerInfo]{}, fmt.Errorf("getTenDlcReseller: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[TenDlcResellerInfo]{}, fmt.Errorf("getTenDlcReseller: reseller %q was not found", req.Input.ResellerID)
	}
	return infer.FunctionResponse[TenDlcResellerInfo]{Output: tenDlcResellerInfo(*w)}, nil
}

// GetTenDlcResellers lists the resellers of the account.
type GetTenDlcResellers struct{}

// GetTenDlcResellersArgs are the lookup arguments (none).
type GetTenDlcResellersArgs struct{}

// GetTenDlcResellersResult lists the resellers.
type GetTenDlcResellersResult struct {
	Resellers []TenDlcResellerInfo `pulumi:"resellers"`
}

func (a *GetTenDlcResellersResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Resellers, "Every reseller of the account, every page.")
}

func (f *GetTenDlcResellers) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the 10DLC resellers of the account, following every page.")
}

func (f *GetTenDlcResellers) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcResellersArgs]) (infer.FunctionResponse[GetTenDlcResellersResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcResellersResult]{}, err
	}
	list, err := c.ListTenDlcResellers(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcResellersResult]{}, fmt.Errorf("getTenDlcResellers: %w", err)
	}
	out := GetTenDlcResellersResult{Resellers: []TenDlcResellerInfo{}}
	for _, w := range list {
		out.Resellers = append(out.Resellers, tenDlcResellerInfo(w))
	}
	return infer.FunctionResponse[GetTenDlcResellersResult]{Output: out}, nil
}

// ---------------------------------------------------------------------------
// getTenDlcCampaignTermsAndConditions
// ---------------------------------------------------------------------------

// GetTenDlcCampaignTermsAndConditions reads the terms a campaign owner agrees to.
type GetTenDlcCampaignTermsAndConditions struct{}

// GetTenDlcCampaignTermsAndConditionsArgs are the lookup arguments (none).
type GetTenDlcCampaignTermsAndConditionsArgs struct{}

// GetTenDlcCampaignTermsAndConditionsResult carries the terms.
type GetTenDlcCampaignTermsAndConditionsResult struct {
	SubTerms []string `pulumi:"subTerms"`
}

func (a *GetTenDlcCampaignTermsAndConditionsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.SubTerms, "The terms and conditions a campaign owner agrees to by setting tcAgreed on a TenDlcCampaign.")
}

func (f *GetTenDlcCampaignTermsAndConditions) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read the 10DLC campaign terms and conditions that tcAgreed on a TenDlcCampaign accepts.")
}

func (f *GetTenDlcCampaignTermsAndConditions) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcCampaignTermsAndConditionsArgs]) (infer.FunctionResponse[GetTenDlcCampaignTermsAndConditionsResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcCampaignTermsAndConditionsResult]{}, err
	}
	terms, err := c.GetTenDlcCampaignTerms(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcCampaignTermsAndConditionsResult]{}, fmt.Errorf("getTenDlcCampaignTermsAndConditions: %w", err)
	}
	if terms == nil {
		terms = []string{}
	}
	return infer.FunctionResponse[GetTenDlcCampaignTermsAndConditionsResult]{Output: GetTenDlcCampaignTermsAndConditionsResult{SubTerms: terms}}, nil
}

// ---------------------------------------------------------------------------
// getTenDlcThroughputs
// ---------------------------------------------------------------------------

// TenDlcThroughputValue is a throughput: exactly one of shared, sms and mms together, or unlimited, is set.
type TenDlcThroughputValue struct {
	Shared    *float64 `pulumi:"shared,optional"`
	Sms       *float64 `pulumi:"sms,optional"`
	Mms       *float64 `pulumi:"mms,optional"`
	Unlimited *bool    `pulumi:"unlimited,optional"`
}

func (a *TenDlcThroughputValue) Annotate(an infer.Annotator) {
	an.Describe(&a.Shared, "Messages per period shared by SMS and MMS.")
	an.Describe(&a.Sms, "SMS messages per period, when SMS and MMS are limited separately.")
	an.Describe(&a.Mms, "MMS messages per period, when SMS and MMS are limited separately.")
	an.Describe(&a.Unlimited, "True when the throughput is unlimited.")
}

// TenDlcThroughputInfo is one throughput counter.
type TenDlcThroughputInfo struct {
	ID         string                `pulumi:"id"`
	BrandID    string                `pulumi:"brandId"`
	CampaignID string                `pulumi:"campaignId"`
	Carriers   []string              `pulumi:"carriers"`
	Value      TenDlcThroughputValue `pulumi:"value"`
	Period     string                `pulumi:"period"`
	Scope      string                `pulumi:"scope"`
}

func (a *TenDlcThroughputInfo) Annotate(an infer.Annotator) {
	an.Describe(&a.ID, "ID of the throughput; only custom overrides carry one.")
	an.Describe(&a.BrandID, "Brand the throughput applies to; empty when not reported.")
	an.Describe(&a.CampaignID, "Campaign the throughput applies to; empty when not reported.")
	an.Describe(&a.Carriers, "Carriers the throughput applies to.")
	an.Describe(&a.Value, "How many messages are allowed per period.")
	an.Describe(&a.Period, "Period the value covers: minute or day.")
	an.Describe(&a.Scope, "What the throughput limits: campaign, brand or number.")
}

// GetTenDlcThroughputs lists the throughput counters of the account.
type GetTenDlcThroughputs struct{}

// GetTenDlcThroughputsArgs are the lookup arguments.
type GetTenDlcThroughputsArgs struct {
	CampaignID *string `pulumi:"campaignId,optional"`
}

func (a *GetTenDlcThroughputsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.CampaignID, "Only list the throughputs of this campaign.")
}

// GetTenDlcThroughputsResult lists the throughputs.
type GetTenDlcThroughputsResult struct {
	Throughputs []TenDlcThroughputInfo `pulumi:"throughputs"`
}

func (a *GetTenDlcThroughputsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Throughputs, "The throughput counters, every page.")
}

func (f *GetTenDlcThroughputs) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the 10DLC throughput counters (messages per minute or day, by scope), optionally for one campaign, following every page.")
}

func (f *GetTenDlcThroughputs) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcThroughputsArgs]) (infer.FunctionResponse[GetTenDlcThroughputsResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcThroughputsResult]{}, err
	}
	list, err := c.ListTenDlcThroughputs(ctx, deref(req.Input.CampaignID))
	if err != nil {
		return infer.FunctionResponse[GetTenDlcThroughputsResult]{}, fmt.Errorf("getTenDlcThroughputs: %w", err)
	}
	out := GetTenDlcThroughputsResult{Throughputs: []TenDlcThroughputInfo{}}
	for _, w := range list {
		carriers := w.Carriers
		if carriers == nil {
			carriers = []string{}
		}
		out.Throughputs = append(out.Throughputs, TenDlcThroughputInfo{ID: w.ID, BrandID: w.BrandID, CampaignID: w.CampaignID, Carriers: carriers,
			Value: TenDlcThroughputValue{Shared: w.Value.Shared, Sms: w.Value.SMS, Mms: w.Value.MMS, Unlimited: w.Value.Unlimited}, Period: w.Period, Scope: w.Scope})
	}
	return infer.FunctionResponse[GetTenDlcThroughputsResult]{Output: out}, nil
}

// ---------------------------------------------------------------------------
// getTenDlcEnums
// ---------------------------------------------------------------------------

// TenDlcUsecaseInfo is a campaign use case Vonage supports.
type TenDlcUsecaseInfo struct {
	ID             string   `pulumi:"id"`
	DisplayName    string   `pulumi:"displayName"`
	Description    string   `pulumi:"description"`
	Classification string   `pulumi:"classification"`
	MinSubUsecases *float64 `pulumi:"minSubUsecases,optional"`
	MaxSubUsecases *float64 `pulumi:"maxSubUsecases,optional"`
	MinMsgSamples  *float64 `pulumi:"minMsgSamples,optional"`
}

func (a *TenDlcUsecaseInfo) Annotate(an infer.Annotator) {
	an.Describe(&a.ID, "Use case ID, the usecase of a TenDlcCampaign.")
	an.Describe(&a.DisplayName, "Display name.")
	an.Describe(&a.Description, "What the use case covers.")
	an.Describe(&a.Classification, "Classification, for example STANDARD or SPECIAL.")
	an.Describe(&a.MinSubUsecases, "Fewest sub use cases a MIXED-style campaign needs; unset when not reported.")
	an.Describe(&a.MaxSubUsecases, "Most sub use cases a campaign may list; unset when not reported.")
	an.Describe(&a.MinMsgSamples, "Fewest sample messages the use case needs; unset when not reported.")
}

// TenDlcVerticalInfo is an industry vertical Vonage supports.
type TenDlcVerticalInfo struct {
	ID          string `pulumi:"id"`
	DisplayName string `pulumi:"displayName"`
	Description string `pulumi:"description"`
	IndustryID  string `pulumi:"industryId"`
}

func (a *TenDlcVerticalInfo) Annotate(an infer.Annotator) {
	an.Describe(&a.ID, "Vertical ID, the vertical of a TenDlcCampaign.")
	an.Describe(&a.DisplayName, "Display name.")
	an.Describe(&a.Description, "What the vertical covers.")
	an.Describe(&a.IndustryID, "Industry the vertical belongs to; empty when not reported.")
}

// GetTenDlcEnums reads the enumerations of the 10DLC API.
type GetTenDlcEnums struct{}

// GetTenDlcEnumsArgs are the lookup arguments (none).
type GetTenDlcEnumsArgs struct{}

// GetTenDlcEnumsResult carries the enumerations.
type GetTenDlcEnumsResult struct {
	Usecases  []TenDlcUsecaseInfo  `pulumi:"usecases"`
	Verticals []TenDlcVerticalInfo `pulumi:"verticals"`
	Values    map[string][]string  `pulumi:"values"`
}

func (a *GetTenDlcEnumsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Usecases, "The campaign use cases with their rules.")
	an.Describe(&a.Verticals, "The industry verticals.")
	an.Describe(&a.Values, "The IDs of every enumeration Vonage lists, keyed by the name the API uses, for example campaign_status, brand_status, entity_type, country, usstate, mno, vetting_provider or tmo_brand_tier (usecase and vertical included).")
}

func (f *GetTenDlcEnums) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read the 10DLC enumerations: campaign use cases and industry verticals with their details, and the IDs of every other list Vonage publishes (statuses, entity types, countries, carriers and so on).")
}

func enumString(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func enumNumber(m map[string]any, key string) *float64 {
	if v, ok := m[key].(float64); ok {
		return &v
	}
	return nil
}

func (f *GetTenDlcEnums) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcEnumsArgs]) (infer.FunctionResponse[GetTenDlcEnumsResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcEnumsResult]{}, err
	}
	raw, err := c.GetTenDlcEnums(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcEnumsResult]{}, fmt.Errorf("getTenDlcEnums: %w", err)
	}
	out := GetTenDlcEnumsResult{Usecases: []TenDlcUsecaseInfo{}, Verticals: []TenDlcVerticalInfo{}, Values: map[string][]string{}}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		ids := []string{}
		for _, item := range raw[k] {
			if id := enumString(item, "id"); id != "" {
				ids = append(ids, id)
			}
			switch k {
			case "usecase":
				out.Usecases = append(out.Usecases, TenDlcUsecaseInfo{ID: enumString(item, "id"), DisplayName: enumString(item, "display_name"),
					Description: enumString(item, "description"), Classification: enumString(item, "classification"),
					MinSubUsecases: enumNumber(item, "min_sub_usecases"), MaxSubUsecases: enumNumber(item, "max_sub_usecases"), MinMsgSamples: enumNumber(item, "min_msg_samples")})
			case "vertical":
				out.Verticals = append(out.Verticals, TenDlcVerticalInfo{ID: enumString(item, "id"), DisplayName: enumString(item, "display_name"),
					Description: enumString(item, "description"), IndustryID: enumString(item, "industry_id")})
			}
		}
		out.Values[k] = ids
	}
	return infer.FunctionResponse[GetTenDlcEnumsResult]{Output: out}, nil
}
