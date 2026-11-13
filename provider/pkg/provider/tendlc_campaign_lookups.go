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

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// getTenDlcCampaign, getTenDlcCampaigns, getTenDlcCampaignNumber, getTenDlcCampaignNumbers
// ---------------------------------------------------------------------------

// TenDlcCampaignInfo describes a campaign; fields Vonage does not report are empty.
type TenDlcCampaignInfo struct {
	CampaignID       string   `pulumi:"campaignId"`
	BrandID          string   `pulumi:"brandId"`
	AccountID        string   `pulumi:"accountId"`
	TcrCampaignID    string   `pulumi:"tcrCampaignId"`
	Usecase          string   `pulumi:"usecase"`
	SubUsecases      []string `pulumi:"subUsecases"`
	Vertical         string   `pulumi:"vertical"`
	Description      string   `pulumi:"description"`
	MessageFlow      string   `pulumi:"messageFlow"`
	Samples          []string `pulumi:"samples"`
	UrlSamples       []string `pulumi:"urlSamples"`
	HelpMessage      string   `pulumi:"helpMessage"`
	HelpKeywords     string   `pulumi:"helpKeywords"`
	OptInMessage     string   `pulumi:"optInMessage"`
	OptInKeywords    string   `pulumi:"optInKeywords"`
	OptOutMessage    string   `pulumi:"optOutMessage"`
	OptOutKeywords   string   `pulumi:"optOutKeywords"`
	ResellerID       string   `pulumi:"resellerId"`
	Reseller         bool     `pulumi:"reseller"`
	Label            string   `pulumi:"label"`
	AgeGated         bool     `pulumi:"ageGated"`
	DirectLending    bool     `pulumi:"directLending"`
	EmbeddedLink     bool     `pulumi:"embeddedLink"`
	EmbeddedPhone    bool     `pulumi:"embeddedPhone"`
	Hipaa            bool     `pulumi:"hipaa"`
	AutoRenewal      bool     `pulumi:"autoRenewal"`
	Status           string   `pulumi:"status"`
	TcrStatus        string   `pulumi:"tcrStatus"`
	TrafficEnabled   bool     `pulumi:"trafficEnabled"`
	RegistrationDate string   `pulumi:"registrationDate"`
	RenewalDate      string   `pulumi:"renewalDate"`
	CreatedDate      string   `pulumi:"createdDate"`
	LastUpdated      string   `pulumi:"lastUpdated"`
}

func (a *TenDlcCampaignInfo) Annotate(an infer.Annotator) {
	an.Describe(&a.CampaignID, "ID of the campaign.")
	an.Describe(&a.BrandID, "ID of the brand the campaign belongs to.")
	an.Describe(&a.AccountID, "Vonage account (API key) the campaign belongs to.")
	an.Describe(&a.TcrCampaignID, "ID of the campaign at The Campaign Registry; empty until it exists there.")
	an.Describe(&a.Usecase, "Use case of the campaign.")
	an.Describe(&a.SubUsecases, "Sub use cases.")
	an.Describe(&a.Vertical, "Industry vertical; empty when unset.")
	an.Describe(&a.Description, "What the campaign is for.")
	an.Describe(&a.MessageFlow, "How subscribers opt in and what they receive.")
	an.Describe(&a.Samples, "The sample messages.")
	an.Describe(&a.UrlSamples, "The sample links.")
	an.Describe(&a.HelpMessage, "Reply to the HELP keyword.")
	an.Describe(&a.HelpKeywords, "Help keywords, comma separated; empty when unset.")
	an.Describe(&a.OptInMessage, "Confirmation sent when a subscriber opts in.")
	an.Describe(&a.OptInKeywords, "Opt-in keywords, comma separated; empty when unset.")
	an.Describe(&a.OptOutMessage, "Confirmation sent when a subscriber opts out.")
	an.Describe(&a.OptOutKeywords, "Opt-out keywords, comma separated; empty when unset.")
	an.Describe(&a.ResellerID, "Reseller ID; empty when unset.")
	an.Describe(&a.Reseller, "Whether a reseller registered the campaign.")
	an.Describe(&a.Label, "Your label for the campaign; empty when unset.")
	an.Describe(&a.AgeGated, "Whether the content is age-gated.")
	an.Describe(&a.DirectLending, "Whether the campaign involves direct lending.")
	an.Describe(&a.EmbeddedLink, "Whether messages contain embedded links.")
	an.Describe(&a.EmbeddedPhone, "Whether messages contain embedded phone numbers.")
	an.Describe(&a.Hipaa, "Whether the campaign handles HIPAA-covered data.")
	an.Describe(&a.AutoRenewal, "Whether the campaign renews automatically.")
	an.Describe(&a.Status, "Campaign status (ended campaigns are not returned).")
	an.Describe(&a.TcrStatus, "Status at The Campaign Registry; empty when unknown.")
	an.Describe(&a.TrafficEnabled, "Whether the campaign can carry traffic.")
	an.Describe(&a.RegistrationDate, "When the campaign was registered; empty when not yet.")
	an.Describe(&a.RenewalDate, "When the campaign renews or ends; empty when unknown.")
	an.Describe(&a.CreatedDate, "When the campaign was created.")
	an.Describe(&a.LastUpdated, "When the campaign was last updated.")
}

func tenDlcCampaignInfo(brandID string, w wireTenDlcCampaign) TenDlcCampaignInfo {
	urls := []string{}
	for _, u := range w.URLSamples {
		urls = append(urls, u.URL)
	}
	subs := w.SubUsecases
	if subs == nil {
		subs = []string{}
	}
	b := w.BrandID
	if b == "" {
		b = brandID
	}
	return TenDlcCampaignInfo{
		CampaignID: w.CampaignID, BrandID: b, AccountID: w.AccountID, TcrCampaignID: w.TCRCampaignID, Usecase: w.Usecase,
		SubUsecases: subs, Vertical: w.Vertical, Description: w.Description, MessageFlow: w.MessageFlow,
		Samples: tenDlcSamplesFrom(&w), UrlSamples: urls, HelpMessage: w.HelpMessage, HelpKeywords: w.HelpKeywords,
		OptInMessage: w.OptInMessage, OptInKeywords: w.OptInKeywords, OptOutMessage: w.OptOutMessage, OptOutKeywords: w.OptOutKeywords,
		ResellerID: w.ResellerID, Reseller: boolOf(w.Reseller), Label: w.Label, AgeGated: boolOf(w.AgeGated),
		DirectLending: boolOf(w.DirectLending), EmbeddedLink: boolOf(w.EmbeddedLink), EmbeddedPhone: boolOf(w.EmbeddedPhone),
		Hipaa: boolOf(w.HIPAA), AutoRenewal: boolOf(w.AutoRenewal), Status: w.Status, TcrStatus: w.TCRStatus,
		TrafficEnabled: boolOf(w.TrafficEnabled), RegistrationDate: w.RegistrationDate, RenewalDate: w.RenewalDate,
		CreatedDate: w.CreatedDate, LastUpdated: w.LastUpdated,
	}
}

// GetTenDlcCampaign looks up one campaign.
type GetTenDlcCampaign struct{}

// GetTenDlcCampaignArgs are the lookup arguments.
type GetTenDlcCampaignArgs struct {
	BrandID    string `pulumi:"brandId"`
	CampaignID string `pulumi:"campaignId"`
}

func (a *GetTenDlcCampaignArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "ID of the brand.")
	an.Describe(&a.CampaignID, "ID of the campaign.")
}

func (f *GetTenDlcCampaign) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a 10DLC campaign by brand and campaign ID. An ended (terminated, canceled, expired or rejected) campaign is reported as not found.")
}

func (f *GetTenDlcCampaign) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcCampaignArgs]) (infer.FunctionResponse[TenDlcCampaignInfo], error) {
	in := req.Input
	if in.BrandID == "" || in.CampaignID == "" {
		return infer.FunctionResponse[TenDlcCampaignInfo]{}, fmt.Errorf("getTenDlcCampaign: brandId and campaignId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[TenDlcCampaignInfo]{}, err
	}
	w, err := c.GetTenDlcCampaign(ctx, in.BrandID, in.CampaignID)
	if err != nil {
		return infer.FunctionResponse[TenDlcCampaignInfo]{}, fmt.Errorf("getTenDlcCampaign: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[TenDlcCampaignInfo]{}, fmt.Errorf("getTenDlcCampaign: campaign %q of brand %q was not found", in.CampaignID, in.BrandID)
	}
	if w.CampaignID == "" {
		w.CampaignID = in.CampaignID
	}
	return infer.FunctionResponse[TenDlcCampaignInfo]{Output: tenDlcCampaignInfo(in.BrandID, *w)}, nil
}

// GetTenDlcCampaigns lists the campaigns of a brand.
type GetTenDlcCampaigns struct{}

// GetTenDlcCampaignsArgs are the lookup arguments.
type GetTenDlcCampaignsArgs struct {
	BrandID string  `pulumi:"brandId"`
	Status  *string `pulumi:"status,optional"`
}

func (a *GetTenDlcCampaignsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "ID of the brand.")
	an.Describe(&a.Status, "Only list campaigns with this status, for example ACTIVE.")
}

// GetTenDlcCampaignsResult lists the campaigns.
type GetTenDlcCampaignsResult struct {
	Campaigns []TenDlcCampaignInfo `pulumi:"campaigns"`
}

func (a *GetTenDlcCampaignsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Campaigns, "The campaigns of the brand, every page, ended ones excluded.")
}

func (f *GetTenDlcCampaigns) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the 10DLC campaigns of a brand, following every page.")
}

func (f *GetTenDlcCampaigns) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcCampaignsArgs]) (infer.FunctionResponse[GetTenDlcCampaignsResult], error) {
	in := req.Input
	if in.BrandID == "" {
		return infer.FunctionResponse[GetTenDlcCampaignsResult]{}, fmt.Errorf("getTenDlcCampaigns: brandId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcCampaignsResult]{}, err
	}
	all, err := c.ListTenDlcCampaigns(ctx, in.BrandID, deref(in.Status))
	if err != nil {
		if isNotFound(err) {
			return infer.FunctionResponse[GetTenDlcCampaignsResult]{}, fmt.Errorf("getTenDlcCampaigns: brand %q was not found", in.BrandID)
		}
		return infer.FunctionResponse[GetTenDlcCampaignsResult]{}, fmt.Errorf("getTenDlcCampaigns: %w", err)
	}
	out := make([]TenDlcCampaignInfo, 0, len(all))
	for _, w := range all {
		out = append(out, tenDlcCampaignInfo(in.BrandID, w))
	}
	return infer.FunctionResponse[GetTenDlcCampaignsResult]{Output: GetTenDlcCampaignsResult{Campaigns: out}}, nil
}

// TenDlcCampaignNumberInfo describes a number linked to a campaign.
type TenDlcCampaignNumberInfo struct {
	Number     string   `pulumi:"number"`
	Country    string   `pulumi:"country"`
	Status     string   `pulumi:"status"`
	Compliance []string `pulumi:"compliance"`
}

func (a *TenDlcCampaignNumberInfo) Annotate(an infer.Annotator) {
	an.Describe(&a.Number, "The number, digits only.")
	an.Describe(&a.Country, "Two-letter country code of the number.")
	an.Describe(&a.Status, "Link status: LINKING, LINKED, REJECTED or UNLINKING. Only LINKED allows traffic.")
	an.Describe(&a.Compliance, "Compliance findings Vonage reports for the number.")
}

func tenDlcNumberInfo(w wireTenDlcNumber) TenDlcCampaignNumberInfo {
	comp := w.Compliance
	if comp == nil {
		comp = []string{}
	}
	return TenDlcCampaignNumberInfo{Number: w.Number, Country: w.Country, Status: w.Status, Compliance: comp}
}

// GetTenDlcCampaignNumber looks up whether a number is linked to a campaign.
type GetTenDlcCampaignNumber struct{}

// GetTenDlcCampaignNumberArgs are the lookup arguments.
type GetTenDlcCampaignNumberArgs struct {
	BrandID    string `pulumi:"brandId"`
	CampaignID string `pulumi:"campaignId"`
	Number     string `pulumi:"number"`
}

func (a *GetTenDlcCampaignNumberArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "ID of the brand.")
	an.Describe(&a.CampaignID, "ID of the campaign.")
	an.Describe(&a.Number, "The number, digits only.")
}

func (f *GetTenDlcCampaignNumber) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a number linked to a 10DLC campaign. A number that is not linked (or already UNLINKED) is reported as not found.")
}

func (f *GetTenDlcCampaignNumber) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcCampaignNumberArgs]) (infer.FunctionResponse[TenDlcCampaignNumberInfo], error) {
	in := req.Input
	if in.BrandID == "" || in.CampaignID == "" || in.Number == "" {
		return infer.FunctionResponse[TenDlcCampaignNumberInfo]{}, fmt.Errorf("getTenDlcCampaignNumber: brandId, campaignId and number must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[TenDlcCampaignNumberInfo]{}, err
	}
	w, err := c.GetTenDlcNumber(ctx, in.BrandID, in.CampaignID, in.Number)
	if err != nil {
		return infer.FunctionResponse[TenDlcCampaignNumberInfo]{}, fmt.Errorf("getTenDlcCampaignNumber: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[TenDlcCampaignNumberInfo]{}, fmt.Errorf("getTenDlcCampaignNumber: number %q is not linked to campaign %q of brand %q", in.Number, in.CampaignID, in.BrandID)
	}
	if w.Number == "" {
		w.Number = in.Number
	}
	return infer.FunctionResponse[TenDlcCampaignNumberInfo]{Output: tenDlcNumberInfo(*w)}, nil
}

// GetTenDlcCampaignNumbers lists the numbers linked to a campaign.
type GetTenDlcCampaignNumbers struct{}

// GetTenDlcCampaignNumbersArgs are the lookup arguments.
type GetTenDlcCampaignNumbersArgs struct {
	BrandID    string `pulumi:"brandId"`
	CampaignID string `pulumi:"campaignId"`
}

func (a *GetTenDlcCampaignNumbersArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "ID of the brand.")
	an.Describe(&a.CampaignID, "ID of the campaign.")
}

// GetTenDlcCampaignNumbersResult lists the links.
type GetTenDlcCampaignNumbersResult struct {
	Numbers []TenDlcCampaignNumberInfo `pulumi:"numbers"`
}

func (a *GetTenDlcCampaignNumbersResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Numbers, "The numbers linked to the campaign, every page, unlinked ones excluded.")
}

func (f *GetTenDlcCampaignNumbers) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the numbers linked to a 10DLC campaign, following every page.")
}

func (f *GetTenDlcCampaignNumbers) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcCampaignNumbersArgs]) (infer.FunctionResponse[GetTenDlcCampaignNumbersResult], error) {
	in := req.Input
	if in.BrandID == "" || in.CampaignID == "" {
		return infer.FunctionResponse[GetTenDlcCampaignNumbersResult]{}, fmt.Errorf("getTenDlcCampaignNumbers: brandId and campaignId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcCampaignNumbersResult]{}, err
	}
	all, err := c.ListTenDlcNumbers(ctx, in.BrandID, in.CampaignID)
	if err != nil {
		if isNotFound(err) {
			return infer.FunctionResponse[GetTenDlcCampaignNumbersResult]{}, fmt.Errorf("getTenDlcCampaignNumbers: campaign %q of brand %q was not found", in.CampaignID, in.BrandID)
		}
		return infer.FunctionResponse[GetTenDlcCampaignNumbersResult]{}, fmt.Errorf("getTenDlcCampaignNumbers: %w", err)
	}
	out := make([]TenDlcCampaignNumberInfo, 0, len(all))
	for _, w := range all {
		out = append(out, tenDlcNumberInfo(w))
	}
	return infer.FunctionResponse[GetTenDlcCampaignNumbersResult]{Output: GetTenDlcCampaignNumbersResult{Numbers: out}}, nil
}
