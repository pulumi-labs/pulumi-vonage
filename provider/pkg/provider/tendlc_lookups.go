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
// getTenDlcAccountSettings, getTenDlcBrand, getTenDlcBrands
// ---------------------------------------------------------------------------

// GetTenDlcAccountSettings reads the account-level 10DLC settings.
type GetTenDlcAccountSettings struct{}

// GetTenDlcAccountSettingsArgs has no arguments.
type GetTenDlcAccountSettingsArgs struct{}

// GetTenDlcAccountSettingsResult carries the settings.
type GetTenDlcAccountSettingsResult struct {
	OptOutAssist bool `pulumi:"optOutAssist"`
}

func (a *GetTenDlcAccountSettingsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.OptOutAssist, "Whether Vonage's opt-out assist is on for the account's 10DLC traffic.")
}

func (f *GetTenDlcAccountSettings) Annotate(a infer.Annotator) {
	a.Describe(&f, "Read the account-level 10DLC settings.")
}

func (f *GetTenDlcAccountSettings) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcAccountSettingsArgs]) (infer.FunctionResponse[GetTenDlcAccountSettingsResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcAccountSettingsResult]{}, err
	}
	w, err := c.GetTenDlcAccount(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcAccountSettingsResult]{}, fmt.Errorf("getTenDlcAccountSettings: %w", err)
	}
	return infer.FunctionResponse[GetTenDlcAccountSettingsResult]{Output: GetTenDlcAccountSettingsResult{OptOutAssist: w.OptOutAssist}}, nil
}

// GetTenDlcBrand looks up one brand by ID.
type GetTenDlcBrand struct{}

// GetTenDlcBrandArgs are the lookup arguments.
type GetTenDlcBrandArgs struct {
	BrandID string `pulumi:"brandId"`
}

func (a *GetTenDlcBrandArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "ID of the brand (seven characters, 0-9 and A-F).")
}

// TenDlcBrandInfo describes a brand; fields Vonage does not report are empty.
type TenDlcBrandInfo struct {
	BrandID            string                    `pulumi:"brandId"`
	AccountID          string                    `pulumi:"accountId"`
	PrimaryAccountID   string                    `pulumi:"primaryAccountId"`
	EntityType         string                    `pulumi:"entityType"`
	FirstName          string                    `pulumi:"firstName"`
	LastName           string                    `pulumi:"lastName"`
	DisplayName        string                    `pulumi:"displayName"`
	CompanyName        string                    `pulumi:"companyName"`
	Ein                string                    `pulumi:"ein"`
	EinIssuingCountry  string                    `pulumi:"einIssuingCountry"`
	UniversalEin       string                    `pulumi:"universalEin"`
	AltBusinessIDType  string                    `pulumi:"altBusinessIdType"`
	AltBusinessID      string                    `pulumi:"altBusinessId"`
	Phone              string                    `pulumi:"phone"`
	MobilePhone        string                    `pulumi:"mobilePhone"`
	Street             string                    `pulumi:"street"`
	City               string                    `pulumi:"city"`
	State              string                    `pulumi:"state"`
	PostalCode         string                    `pulumi:"postalCode"`
	Country            string                    `pulumi:"country"`
	Email              string                    `pulumi:"email"`
	StockSymbol        string                    `pulumi:"stockSymbol"`
	StockExchange      string                    `pulumi:"stockExchange"`
	Website            string                    `pulumi:"website"`
	Vertical           string                    `pulumi:"vertical"`
	VerificationEmail  string                    `pulumi:"verificationEmail"`
	Reseller           bool                      `pulumi:"reseller"`
	Status             string                    `pulumi:"status"`
	BrandRelationship  string                    `pulumi:"brandRelationship"`
	CampaignCount      int                       `pulumi:"campaignCount"`
	ReferenceID        string                    `pulumi:"referenceId"`
	VerifiedDate       string                    `pulumi:"verifiedDate"`
	CreatedDate        string                    `pulumi:"createdDate"`
	LastUpdated        string                    `pulumi:"lastUpdated"`
	Owner              bool                      `pulumi:"owner"`
	Partner            bool                      `pulumi:"partner"`
	Shared             bool                      `pulumi:"shared"`
	CarrierSuspensions []TenDlcCarrierSuspension `pulumi:"carrierSuspensions"`
}

// TenDlcCarrierSuspension is a carrier's suspension of a brand.
type TenDlcCarrierSuspension struct {
	Category    string `pulumi:"category"`
	Explanation string `pulumi:"explanation"`
}

func (a *TenDlcCarrierSuspension) Annotate(an infer.Annotator) {
	an.Describe(&a.Category, "Category of the suspension.")
	an.Describe(&a.Explanation, "The carrier's explanation.")
}

func (a *TenDlcBrandInfo) Annotate(an infer.Annotator) {
	an.Describe(&a.BrandID, "ID of the brand at The Campaign Registry.")
	an.Describe(&a.AccountID, "Vonage account (API key) the brand belongs to.")
	an.Describe(&a.PrimaryAccountID, "API key of the primary account; empty when unset.")
	an.Describe(&a.EntityType, "Legal form: PRIVATE_PROFIT, PUBLIC_PROFIT, NON_PROFIT, SOLE_PROPRIETOR or GOVERNMENT.")
	an.Describe(&a.FirstName, "First name; empty when unset.")
	an.Describe(&a.LastName, "Last name; empty when unset.")
	an.Describe(&a.DisplayName, "Display name; empty when unset.")
	an.Describe(&a.CompanyName, "Legal company name; empty when unset.")
	an.Describe(&a.Ein, "Tax ID (EIN); empty when unset.")
	an.Describe(&a.EinIssuingCountry, "Country that issued the tax ID; empty when unset.")
	an.Describe(&a.UniversalEin, "Universal tax ID; empty when unset.")
	an.Describe(&a.AltBusinessIDType, "Alternative business ID type (DUNS, GIIN or LEI); empty when unset.")
	an.Describe(&a.AltBusinessID, "Alternative business ID; empty when unset.")
	an.Describe(&a.Phone, "Contact phone number; empty when unset.")
	an.Describe(&a.MobilePhone, "Mobile phone number; empty when unset.")
	an.Describe(&a.Street, "Street address; empty when unset.")
	an.Describe(&a.City, "City; empty when unset.")
	an.Describe(&a.State, "State or region; empty when unset.")
	an.Describe(&a.PostalCode, "Postal code; empty when unset.")
	an.Describe(&a.Country, "Country of the address; empty when unset.")
	an.Describe(&a.Email, "Contact email; empty when unset.")
	an.Describe(&a.StockSymbol, "Stock symbol; empty when unset.")
	an.Describe(&a.StockExchange, "Stock exchange; empty when unset.")
	an.Describe(&a.Website, "Website; empty when unset.")
	an.Describe(&a.Vertical, "Industry vertical; empty when unset.")
	an.Describe(&a.VerificationEmail, "Verification email address; empty when unset.")
	an.Describe(&a.Reseller, "Whether the brand is a reseller.")
	an.Describe(&a.Status, "ACTIVE or SUSPENDED (deleted brands are not returned).")
	an.Describe(&a.BrandRelationship, "Relationship tier the registry assigned; empty when not yet known.")
	an.Describe(&a.CampaignCount, "Number of campaigns registered for the brand.")
	an.Describe(&a.ReferenceID, "Reference ID of the brand; empty when unset.")
	an.Describe(&a.VerifiedDate, "When the brand was verified; empty when it has not been.")
	an.Describe(&a.CreatedDate, "When the brand was created.")
	an.Describe(&a.LastUpdated, "When the brand was last updated.")
	an.Describe(&a.Owner, "Whether the account owns the brand.")
	an.Describe(&a.Partner, "Whether the brand is a partner brand.")
	an.Describe(&a.Shared, "Whether the brand is shared with the account.")
	an.Describe(&a.CarrierSuspensions, "Carrier suspensions of the brand, if any.")
}

func (f *GetTenDlcBrand) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a 10DLC brand by ID. A deleted brand is reported as not found.")
}

func boolOf(b *bool) bool { return b != nil && *b }

func tenDlcBrandInfo(w wireTenDlcBrand) TenDlcBrandInfo {
	n := 0
	if w.CampaignCount != nil {
		n = *w.CampaignCount
	}
	sus := []TenDlcCarrierSuspension{}
	for _, s := range w.CarrierSuspensions {
		sus = append(sus, TenDlcCarrierSuspension{Category: s.Category, Explanation: s.Explanation})
	}
	return TenDlcBrandInfo{
		BrandID: w.BrandID, AccountID: w.AccountID, PrimaryAccountID: w.PrimaryAccountID, EntityType: w.EntityType,
		FirstName: w.FirstName, LastName: w.LastName, DisplayName: w.DisplayName, CompanyName: w.CompanyName,
		Ein: w.EIN, EinIssuingCountry: w.EINIssuingCountry, UniversalEin: w.UniversalEIN,
		AltBusinessIDType: w.AltBusinessIDType, AltBusinessID: w.AltBusinessID, Phone: w.Phone, MobilePhone: w.MobilePhone,
		Street: w.Street, City: w.City, State: w.State, PostalCode: w.PostalCode, Country: w.Country, Email: w.Email,
		StockSymbol: w.StockSymbol, StockExchange: w.StockExchange, Website: w.Website, Vertical: w.Vertical,
		VerificationEmail: w.VerificationEmail, Reseller: boolOf(w.Reseller), Status: w.Status,
		BrandRelationship: w.BrandRelationship, CampaignCount: n, ReferenceID: w.ReferenceID, VerifiedDate: w.VerifiedDate,
		CreatedDate: w.CreatedDate, LastUpdated: w.LastUpdated, Owner: boolOf(w.Owner), Partner: boolOf(w.Partner),
		Shared: boolOf(w.Shared), CarrierSuspensions: sus,
	}
}

func (f *GetTenDlcBrand) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcBrandArgs]) (infer.FunctionResponse[TenDlcBrandInfo], error) {
	id := req.Input.BrandID
	if id == "" {
		return infer.FunctionResponse[TenDlcBrandInfo]{}, fmt.Errorf("getTenDlcBrand: brandId must not be empty")
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[TenDlcBrandInfo]{}, err
	}
	w, err := c.GetTenDlcBrand(ctx, id)
	if err != nil {
		return infer.FunctionResponse[TenDlcBrandInfo]{}, fmt.Errorf("getTenDlcBrand: %w", err)
	}
	if w == nil {
		return infer.FunctionResponse[TenDlcBrandInfo]{}, fmt.Errorf("getTenDlcBrand: brand %q was not found", id)
	}
	if w.BrandID == "" {
		w.BrandID = id
	}
	return infer.FunctionResponse[TenDlcBrandInfo]{Output: tenDlcBrandInfo(*w)}, nil
}

// GetTenDlcBrands lists brands.
type GetTenDlcBrands struct{}

// GetTenDlcBrandsArgs are the lookup arguments.
type GetTenDlcBrandsArgs struct {
	Status *string `pulumi:"status,optional"`
}

func (a *GetTenDlcBrandsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Status, "Only list brands with this status (ACTIVE, SUSPENDED or TERMINATED). Vonage lists ACTIVE and SUSPENDED brands when it is unset.")
}

// GetTenDlcBrandsResult lists the brands.
type GetTenDlcBrandsResult struct {
	Brands []TenDlcBrandInfo `pulumi:"brands"`
}

func (a *GetTenDlcBrandsResult) Annotate(an infer.Annotator) {
	an.Describe(&a.Brands, "The brands, every page, deleted ones excluded.")
}

func (f *GetTenDlcBrands) Annotate(a infer.Annotator) {
	a.Describe(&f, "List the account's 10DLC brands, following every page.")
}

func (f *GetTenDlcBrands) Invoke(ctx context.Context, req infer.FunctionRequest[GetTenDlcBrandsArgs]) (infer.FunctionResponse[GetTenDlcBrandsResult], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.FunctionResponse[GetTenDlcBrandsResult]{}, err
	}
	all, err := c.ListTenDlcBrands(ctx, deref(req.Input.Status))
	if err != nil {
		return infer.FunctionResponse[GetTenDlcBrandsResult]{}, fmt.Errorf("getTenDlcBrands: %w", err)
	}
	out := make([]TenDlcBrandInfo, 0, len(all))
	for _, w := range all {
		out = append(out, tenDlcBrandInfo(w))
	}
	return infer.FunctionResponse[GetTenDlcBrandsResult]{Output: GetTenDlcBrandsResult{Brands: out}}, nil
}
