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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// 10DLC (US A2P messaging registration): account settings and brands. Shapes follow the Vonage
// 10DLC OpenAPI definition v1.0.34 (neither Vonage SDK has a 10dlc package) and have not been
// exercised against a live Vonage account.

const (
	tenDlcBase     = "/v1/10dlc"
	tenDlcPageSize = 50
)

// ---------------------------------------------------------------------------
// Wire types and client
// ---------------------------------------------------------------------------

type wireTenDlcAccount struct {
	OptOutAssist bool `json:"opt_out_assist"`
}

// GetTenDlcAccount reads the 10DLC account-level settings.
func (c *Client) GetTenDlcAccount(ctx context.Context) (*wireTenDlcAccount, error) {
	var out wireTenDlcAccount
	if err := c.apiJSON(ctx, http.MethodGet, tenDlcBase+"/account", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PatchTenDlcAccount updates the 10DLC account-level settings (HTTP 204, no body).
func (c *Client) PatchTenDlcAccount(ctx context.Context, optOutAssist bool) error {
	return c.apiJSON(ctx, http.MethodPatch, tenDlcBase+"/account", nil, wireTenDlcAccount{OptOutAssist: optOutAssist}, nil)
}

type wireTenDlcCarrierSuspension struct {
	Category    string `json:"category"`
	Explanation string `json:"explanation"`
}

// wireTenDlcBrand is a brand as the API returns it. The writable fields double as the create and
// patch bodies (every one is omitted when empty).
type wireTenDlcBrand struct {
	AccountID         string `json:"account_id,omitempty"`
	PrimaryAccountID  string `json:"primary_account_id,omitempty"`
	EntityType        string `json:"entity_type,omitempty"`
	FirstName         string `json:"first_name,omitempty"`
	LastName          string `json:"last_name,omitempty"`
	DisplayName       string `json:"display_name,omitempty"`
	CompanyName       string `json:"company_name,omitempty"`
	EIN               string `json:"ein,omitempty"`
	EINIssuingCountry string `json:"ein_issuing_country,omitempty"`
	UniversalEIN      string `json:"universal_ein,omitempty"`
	AltBusinessIDType string `json:"alt_business_id_type,omitempty"`
	AltBusinessID     string `json:"alt_business_id,omitempty"`
	Phone             string `json:"phone,omitempty"`
	MobilePhone       string `json:"mobile_phone,omitempty"`
	Street            string `json:"street,omitempty"`
	City              string `json:"city,omitempty"`
	State             string `json:"state,omitempty"`
	PostalCode        string `json:"postal_code,omitempty"`
	Country           string `json:"country,omitempty"`
	Email             string `json:"email,omitempty"`
	StockSymbol       string `json:"stock_symbol,omitempty"`
	StockExchange     string `json:"stock_exchange,omitempty"`
	Website           string `json:"website,omitempty"`
	Vertical          string `json:"vertical,omitempty"`
	VerificationEmail string `json:"verification_email,omitempty"`
	Reseller          *bool  `json:"reseller,omitempty"`

	// Read-only.
	BrandID            string                        `json:"brand_id,omitempty"`
	CampaignCount      *int                          `json:"campaign_count,omitempty"`
	CarrierSuspensions []wireTenDlcCarrierSuspension `json:"carrier_suspensions,omitempty"`
	BrandRelationship  string                        `json:"brand_relationship,omitempty"`
	Partner            *bool                         `json:"partner,omitempty"`
	Shared             *bool                         `json:"shared,omitempty"`
	Owner              *bool                         `json:"owner,omitempty"`
	Status             string                        `json:"status,omitempty"`
	ReferenceID        string                        `json:"reference_id,omitempty"`
	VerifiedDate       string                        `json:"verified_date,omitempty"`
	CreatedDate        string                        `json:"created_date,omitempty"`
	LastUpdated        string                        `json:"last_updated,omitempty"`
}

// wireTenDlcBrandPatch is the body of PATCH /brands/{id}: only the fields the API lets you change.
type wireTenDlcBrandPatch struct {
	CompanyName       string `json:"company_name,omitempty"`
	FirstName         string `json:"first_name,omitempty"`
	LastName          string `json:"last_name,omitempty"`
	DisplayName       string `json:"display_name,omitempty"`
	EIN               string `json:"ein,omitempty"`
	EINIssuingCountry string `json:"ein_issuing_country,omitempty"`
	EntityType        string `json:"entity_type,omitempty"`
	Website           string `json:"website,omitempty"`
	Phone             string `json:"phone,omitempty"`
	MobilePhone       string `json:"mobile_phone,omitempty"`
	Street            string `json:"street,omitempty"`
	City              string `json:"city,omitempty"`
	State             string `json:"state,omitempty"`
	PostalCode        string `json:"postal_code,omitempty"`
	Country           string `json:"country,omitempty"`
	Email             string `json:"email,omitempty"`
	StockSymbol       string `json:"stock_symbol,omitempty"`
	StockExchange     string `json:"stock_exchange,omitempty"`
	Vertical          string `json:"vertical,omitempty"`
	Reseller          *bool  `json:"reseller,omitempty"`
}

func tenDlcBrandPath(id string) string { return tenDlcBase + "/brands/" + url.PathEscape(id) }

// tenDlcBrandGone reports whether a failed brand call means the brand does not exist. Vonage
// documents that every endpoint except the listing answers "502 - Brand not found" for a deleted
// brand, so a 502 that says so counts as gone alongside the usual 404.
func tenDlcBrandGone(err error) bool {
	if isNotFound(err) {
		return true
	}
	var ae *apiError
	return errors.As(err, &ae) && ae.Status == http.StatusBadGateway && strings.Contains(strings.ToLower(ae.Body), "not found")
}

// CreateTenDlcBrand registers a brand. The call is billed and never replayed after an unknown
// outcome (POST), so a retry cannot register the brand twice.
func (c *Client) CreateTenDlcBrand(ctx context.Context, in wireTenDlcBrand) (*wireTenDlcBrand, error) {
	var out wireTenDlcBrand
	if err := c.apiJSON(ctx, http.MethodPost, tenDlcBase+"/brands", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTenDlcBrand returns nil, nil when the brand does not exist or is in the DELETED state.
func (c *Client) GetTenDlcBrand(ctx context.Context, id string) (*wireTenDlcBrand, error) {
	var out wireTenDlcBrand
	err := c.apiJSON(ctx, http.MethodGet, tenDlcBrandPath(id), nil, nil, &out)
	if tenDlcBrandGone(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.Status == "DELETED" {
		return nil, nil
	}
	return &out, nil
}

// PatchTenDlcBrand updates a brand and returns the result.
func (c *Client) PatchTenDlcBrand(ctx context.Context, id string, in wireTenDlcBrandPatch) (*wireTenDlcBrand, error) {
	var out wireTenDlcBrand
	if err := c.apiJSON(ctx, http.MethodPatch, tenDlcBrandPath(id), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTenDlcBrand deletes a brand (it moves to the DELETED state); a missing brand is not an error.
// Vonage refuses (409) while the brand has active campaigns.
func (c *Client) DeleteTenDlcBrand(ctx context.Context, id string) error {
	err := c.apiJSON(ctx, http.MethodDelete, tenDlcBrandPath(id), nil, nil, nil)
	if tenDlcBrandGone(err) {
		return nil
	}
	return err
}

// ListTenDlcBrands returns every brand (all pages), skipping DELETED ones. status filters server-side
// (ACTIVE, SUSPENDED or TERMINATED; Vonage returns ACTIVE and SUSPENDED when it is empty).
func (c *Client) ListTenDlcBrands(ctx context.Context, status string) ([]wireTenDlcBrand, error) {
	out := []wireTenDlcBrand{}
	for page := 1; ; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(tenDlcPageSize)}}
		if status != "" {
			q.Set("status", status)
		}
		var resp struct {
			TotalPages *int `json:"total_pages"`
			Embedded   struct {
				Brands []wireTenDlcBrand `json:"brands"`
			} `json:"_embedded"`
		}
		if err := c.apiJSON(ctx, http.MethodGet, tenDlcBase+"/brands", q, nil, &resp); err != nil {
			return nil, err
		}
		for _, b := range resp.Embedded.Brands {
			if b.Status != "DELETED" {
				out = append(out, b)
			}
		}
		if resp.TotalPages == nil {
			if len(resp.Embedded.Brands) >= tenDlcPageSize {
				return nil, fmt.Errorf("vonage: GET %s/brands: the response does not report total_pages, so the listing cannot be followed", tenDlcBase)
			}
			return out, nil
		}
		if page >= *resp.TotalPages {
			return out, nil
		}
	}
}

// ---------------------------------------------------------------------------
// TenDlcAccountSettings
// ---------------------------------------------------------------------------

// TenDlcAccountSettings manages the account-level 10DLC settings.
type TenDlcAccountSettings struct{}

// TenDlcAccountSettingsArgs are the inputs of the 10DLC account settings.
type TenDlcAccountSettingsArgs struct {
	OptOutAssist bool `pulumi:"optOutAssist"`
}

func (a *TenDlcAccountSettingsArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.OptOutAssist, "Whether Vonage's opt-out assist is on: it handles opt-out keywords such as STOP for the account's 10DLC traffic.")
}

// TenDlcAccountSettingsState is the persisted state: the inputs.
type TenDlcAccountSettingsState struct {
	TenDlcAccountSettingsArgs
}

func (r *TenDlcAccountSettings) Annotate(a infer.Annotator) {
	a.Describe(&r, "The account-level 10DLC settings (US A2P messaging registration). It is a singleton: the settings always exist, so create adopts them, the ID and import ID is the account API key, and destroy only stops managing them (the setting keeps its last value). Only the primary account may change them. Shapes follow the published Vonage 10DLC OpenAPI definition and were not verified against a live account.")
}

var _ infer.CustomResource[TenDlcAccountSettingsArgs, TenDlcAccountSettingsState] = (*TenDlcAccountSettings)(nil)
var _ infer.CustomRead[TenDlcAccountSettingsArgs, TenDlcAccountSettingsState] = (*TenDlcAccountSettings)(nil)
var _ infer.CustomUpdate[TenDlcAccountSettingsArgs, TenDlcAccountSettingsState] = (*TenDlcAccountSettings)(nil)
var _ infer.CustomDiff[TenDlcAccountSettingsArgs, TenDlcAccountSettingsState] = (*TenDlcAccountSettings)(nil)
var _ infer.CustomDelete[TenDlcAccountSettingsState] = (*TenDlcAccountSettings)(nil)

func (r *TenDlcAccountSettings) Create(ctx context.Context, req infer.CreateRequest[TenDlcAccountSettingsArgs]) (infer.CreateResponse[TenDlcAccountSettingsState], error) {
	st := TenDlcAccountSettingsState{TenDlcAccountSettingsArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[TenDlcAccountSettingsState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[TenDlcAccountSettingsState]{}, err
	}
	if err := c.PatchTenDlcAccount(ctx, req.Inputs.OptOutAssist); err != nil {
		return infer.CreateResponse[TenDlcAccountSettingsState]{}, fmt.Errorf("update 10DLC account settings: %w", err)
	}
	return infer.CreateResponse[TenDlcAccountSettingsState]{ID: c.apiKey, Output: st}, nil
}

func (r *TenDlcAccountSettings) Read(ctx context.Context, req infer.ReadRequest[TenDlcAccountSettingsArgs, TenDlcAccountSettingsState]) (infer.ReadResponse[TenDlcAccountSettingsArgs, TenDlcAccountSettingsState], error) {
	type resp = infer.ReadResponse[TenDlcAccountSettingsArgs, TenDlcAccountSettingsState]
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	w, err := c.GetTenDlcAccount(ctx)
	if err != nil {
		return resp{}, fmt.Errorf("read 10DLC account settings: %w", err)
	}
	args := TenDlcAccountSettingsArgs{OptOutAssist: w.OptOutAssist}
	return resp{ID: req.ID, Inputs: args, State: TenDlcAccountSettingsState{TenDlcAccountSettingsArgs: args}}, nil
}

func (r *TenDlcAccountSettings) Diff(ctx context.Context, req infer.DiffRequest[TenDlcAccountSettingsArgs, TenDlcAccountSettingsState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	if req.Inputs.OptOutAssist != req.State.OptOutAssist {
		d["optOutAssist"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *TenDlcAccountSettings) Update(ctx context.Context, req infer.UpdateRequest[TenDlcAccountSettingsArgs, TenDlcAccountSettingsState]) (infer.UpdateResponse[TenDlcAccountSettingsState], error) {
	st := TenDlcAccountSettingsState{TenDlcAccountSettingsArgs: req.Inputs}
	if req.DryRun {
		return infer.UpdateResponse[TenDlcAccountSettingsState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[TenDlcAccountSettingsState]{}, err
	}
	if err := c.PatchTenDlcAccount(ctx, req.Inputs.OptOutAssist); err != nil {
		return infer.UpdateResponse[TenDlcAccountSettingsState]{}, fmt.Errorf("update 10DLC account settings: %w", err)
	}
	return infer.UpdateResponse[TenDlcAccountSettingsState]{Output: st}, nil
}

// Delete only forgets the settings: they cannot be removed.
func (r *TenDlcAccountSettings) Delete(ctx context.Context, req infer.DeleteRequest[TenDlcAccountSettingsState]) (infer.DeleteResponse, error) {
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// TenDlcBrand
// ---------------------------------------------------------------------------

// TenDlcBrand registers a brand (the business or person behind the campaigns) for 10DLC.
type TenDlcBrand struct{}

// TenDlcBrandArgs are the inputs of a brand. Optional inputs left unset are not managed and not sent.
type TenDlcBrandArgs struct {
	EntityType        string  `pulumi:"entityType"`
	AccountID         *string `pulumi:"accountId,optional"`
	PrimaryAccountID  *string `pulumi:"primaryAccountId,optional"`
	FirstName         *string `pulumi:"firstName,optional"`
	LastName          *string `pulumi:"lastName,optional"`
	DisplayName       *string `pulumi:"displayName,optional"`
	CompanyName       *string `pulumi:"companyName,optional"`
	Ein               *string `pulumi:"ein,optional"`
	EinIssuingCountry *string `pulumi:"einIssuingCountry,optional"`
	UniversalEin      *string `pulumi:"universalEin,optional"`
	AltBusinessIDType *string `pulumi:"altBusinessIdType,optional"`
	AltBusinessID     *string `pulumi:"altBusinessId,optional"`
	Phone             *string `pulumi:"phone,optional"`
	MobilePhone       *string `pulumi:"mobilePhone,optional"`
	Street            *string `pulumi:"street,optional"`
	City              *string `pulumi:"city,optional"`
	State             *string `pulumi:"state,optional"`
	PostalCode        *string `pulumi:"postalCode,optional"`
	Country           *string `pulumi:"country,optional"`
	Email             *string `pulumi:"email,optional"`
	StockSymbol       *string `pulumi:"stockSymbol,optional"`
	StockExchange     *string `pulumi:"stockExchange,optional"`
	Website           *string `pulumi:"website,optional"`
	Vertical          *string `pulumi:"vertical,optional"`
	VerificationEmail *string `pulumi:"verificationEmail,optional"`
	Reseller          *bool   `pulumi:"reseller,optional"`
}

func (a *TenDlcBrandArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.EntityType, "Legal form of the brand: PRIVATE_PROFIT, PUBLIC_PROFIT, NON_PROFIT, SOLE_PROPRIETOR or GOVERNMENT. It decides which other fields Vonage requires.")
	an.Describe(&a.AccountID, "Vonage account (API key) the brand belongs to. Defaults to the provider's apiKey. Changing it replaces the brand.")
	an.Describe(&a.PrimaryAccountID, "API key of the primary account, when the brand is registered for a subaccount. Changing it replaces the brand.")
	an.Describe(&a.FirstName, "First name (sole proprietors; at most 100 characters).")
	an.Describe(&a.LastName, "Last name (sole proprietors; at most 100 characters).")
	an.Describe(&a.DisplayName, "Name the brand is shown under (at most 255 characters).")
	an.Describe(&a.CompanyName, "Legal company name (at most 255 characters).")
	an.Describe(&a.Ein, "Tax ID (EIN) of the company, as NN-NNNNNNN or NNNNNNNNN.")
	an.Describe(&a.EinIssuingCountry, "Two-letter ISO 3166 code of the country that issued the tax ID, in capitals.")
	an.Describe(&a.UniversalEin, "Universal tax ID for companies outside the US. Changing it replaces the brand.")
	an.Describe(&a.AltBusinessIDType, "Type of the alternative business ID: DUNS, GIIN or LEI. Changing it replaces the brand.")
	an.Describe(&a.AltBusinessID, "Alternative business ID of the type altBusinessIdType. Changing it replaces the brand.")
	an.Describe(&a.Phone, "Contact phone number.")
	an.Describe(&a.MobilePhone, "Mobile phone number in E.164 form without the plus sign.")
	an.Describe(&a.Street, "Street address (at most 100 characters).")
	an.Describe(&a.City, "City (at most 100 characters).")
	an.Describe(&a.State, "State or region (at most 20 characters).")
	an.Describe(&a.PostalCode, "Postal code (at most 10 characters).")
	an.Describe(&a.Country, "Two-letter ISO 3166 country code of the address.")
	an.Describe(&a.Email, "Contact email address (at most 100 characters).")
	an.Describe(&a.StockSymbol, "Stock symbol (public companies).")
	an.Describe(&a.StockExchange, "Stock exchange of the symbol (for example NASDAQ, NYSE, or NONE).")
	an.Describe(&a.Website, "Website of the brand.")
	an.Describe(&a.Vertical, "Industry vertical, for example HEALTHCARE, RETAIL or EDUCATION (see getTenDlcEnums once available, or the Vonage 10DLC reference).")
	an.Describe(&a.VerificationEmail, "Email address Vonage sends the brand's verification to. Changing it replaces the brand.")
	an.Describe(&a.Reseller, "Whether the brand is a reseller.")
}

// TenDlcBrandState is the persisted state of a brand.
type TenDlcBrandState struct {
	TenDlcBrandArgs

	BrandID string `pulumi:"brandId"`
	// Status is ACTIVE or SUSPENDED; it moves on its own as the registry reviews the brand.
	Status            string  `pulumi:"status"`
	BrandRelationship *string `pulumi:"brandRelationship,optional"`
	CampaignCount     *int    `pulumi:"campaignCount,optional"`
	ReferenceID       *string `pulumi:"referenceId,optional"`
	VerifiedDate      *string `pulumi:"verifiedDate,optional"`
	CreatedDate       *string `pulumi:"createdDate,optional"`
	LastUpdated       *string `pulumi:"lastUpdated,optional"`
	Owner             *bool   `pulumi:"owner,optional"`
	Partner           *bool   `pulumi:"partner,optional"`
	Shared            *bool   `pulumi:"shared,optional"`
}

func (s *TenDlcBrandState) Annotate(a infer.Annotator) {
	a.Describe(&s.BrandID, "The brand's ID at The Campaign Registry (seven characters, 0-9 and A-F). Campaigns refer to it.")
	a.Describe(&s.Status, "Brand status: ACTIVE or SUSPENDED. It changes as the registry reviews the brand.")
	a.Describe(&s.BrandRelationship, "Relationship tier the registry assigned: BASIC_ACCOUNT, SMALL_ACCOUNT, MEDIUM_ACCOUNT, LARGE_ACCOUNT or KEY_ACCOUNT.")
	a.Describe(&s.CampaignCount, "Number of campaigns registered for the brand.")
	a.Describe(&s.ReferenceID, "Reference ID of the brand.")
	a.Describe(&s.VerifiedDate, "When the brand was verified, if it has been.")
	a.Describe(&s.CreatedDate, "When the brand was created.")
	a.Describe(&s.LastUpdated, "When the brand was last updated.")
	a.Describe(&s.Owner, "Whether the account owns the brand.")
	a.Describe(&s.Partner, "Whether the brand is a partner brand.")
	a.Describe(&s.Shared, "Whether the brand is shared with the account.")
}

func (r *TenDlcBrand) Annotate(a infer.Annotator) {
	a.Describe(&r, "A 10DLC brand: the business or person behind US A2P SMS campaigns, registered with The Campaign Registry through Vonage. Registration is billed and may need vetting before campaigns can be made, so a brand is not created lightly and the API call is never retried. Destroy deletes the brand (it moves to DELETED at Vonage and then reads as gone); Vonage refuses while the brand has active campaigns. Optional inputs left unset are not managed. Shapes follow the published Vonage 10DLC OpenAPI definition and were not verified against a live account.")
}

var _ infer.CustomResource[TenDlcBrandArgs, TenDlcBrandState] = (*TenDlcBrand)(nil)
var _ infer.CustomCheck[TenDlcBrandArgs] = (*TenDlcBrand)(nil)
var _ infer.CustomRead[TenDlcBrandArgs, TenDlcBrandState] = (*TenDlcBrand)(nil)
var _ infer.CustomUpdate[TenDlcBrandArgs, TenDlcBrandState] = (*TenDlcBrand)(nil)
var _ infer.CustomDiff[TenDlcBrandArgs, TenDlcBrandState] = (*TenDlcBrand)(nil)
var _ infer.CustomDelete[TenDlcBrandState] = (*TenDlcBrand)(nil)

var (
	tenDlcEntityTypes = []string{"PRIVATE_PROFIT", "PUBLIC_PROFIT", "NON_PROFIT", "SOLE_PROPRIETOR", "GOVERNMENT"}
	tenDlcAltIDTypes  = []string{"DUNS", "GIIN", "LEI"}
	tenDlcEINRe       = regexp.MustCompile(`^([0-9]{2}-?[0-9]{7})$|^([0-9]{3}-?[0-9]{2}-?[0-9]{4})$`)
	tenDlcMobileRe    = regexp.MustCompile(`^[1-9][0-9]{7,14}$`)
)

func (r *TenDlcBrand) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[TenDlcBrandArgs], error) {
	args, failures, err := infer.DefaultCheck[TenDlcBrandArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[TenDlcBrandArgs]{Inputs: args, Failures: failures}, err
	}
	fail := func(prop, reason string) { failures = append(failures, p.CheckFailure{Property: prop, Reason: reason}) }
	if !containsString(tenDlcEntityTypes, args.EntityType) {
		fail("entityType", "entityType must be one of "+strings.Join(tenDlcEntityTypes, ", "))
	}
	maxLen := map[string]struct {
		v *string
		n int
	}{
		"firstName": {args.FirstName, 100}, "lastName": {args.LastName, 100}, "displayName": {args.DisplayName, 255},
		"companyName": {args.CompanyName, 255}, "street": {args.Street, 100}, "city": {args.City, 100},
		"state": {args.State, 20}, "postalCode": {args.PostalCode, 10}, "email": {args.Email, 100},
		"stockSymbol": {args.StockSymbol, 10}, "verificationEmail": {args.VerificationEmail, 100},
		"universalEin": {args.UniversalEin, 50}, "altBusinessId": {args.AltBusinessID, 50},
	}
	for prop, f := range maxLen {
		if f.v != nil && len(*f.v) > f.n {
			fail(prop, fmt.Sprintf("%s must be at most %d characters", prop, f.n))
		}
	}
	for prop, v := range map[string]*string{"accountId": args.AccountID, "primaryAccountId": args.PrimaryAccountID} {
		if v != nil && (strings.TrimSpace(*v) == "" || len(*v) > 50) {
			fail(prop, prop+" must be 1 to 50 characters")
		}
	}
	if args.Ein != nil && !tenDlcEINRe.MatchString(*args.Ein) {
		fail("ein", "ein must look like NN-NNNNNNN or NNNNNNNNN")
	}
	if args.MobilePhone != nil && !tenDlcMobileRe.MatchString(*args.MobilePhone) {
		fail("mobilePhone", "mobilePhone must be 8 to 15 digits in E.164 form without the plus sign")
	}
	if args.EinIssuingCountry != nil && (!isCountryCode(*args.EinIssuingCountry) || *args.EinIssuingCountry != strings.ToUpper(*args.EinIssuingCountry)) {
		fail("einIssuingCountry", "einIssuingCountry must be a two-letter ISO 3166 country code in capitals")
	}
	if args.AltBusinessIDType != nil && !containsString(tenDlcAltIDTypes, *args.AltBusinessIDType) {
		fail("altBusinessIdType", "altBusinessIdType must be one of "+strings.Join(tenDlcAltIDTypes, ", "))
	}
	return infer.CheckResponse[TenDlcBrandArgs]{Inputs: args, Failures: dropUnknownFailures(failures, req.NewInputs)}, nil
}

// isZeroImport reports whether Read arrived with neither prior inputs nor prior state, which is how an
// import looks.
func isZeroImport[A, S any](inputs A, state S) bool {
	return reflect.ValueOf(inputs).IsZero() && reflect.ValueOf(state).IsZero()
}

func tenDlcBrandBody(a TenDlcBrandArgs, defaultAccount string) wireTenDlcBrand {
	acct := deref(a.AccountID)
	if acct == "" {
		acct = defaultAccount
	}
	return wireTenDlcBrand{
		AccountID: acct, PrimaryAccountID: deref(a.PrimaryAccountID), EntityType: a.EntityType,
		FirstName: deref(a.FirstName), LastName: deref(a.LastName), DisplayName: deref(a.DisplayName),
		CompanyName: deref(a.CompanyName), EIN: deref(a.Ein), EINIssuingCountry: deref(a.EinIssuingCountry),
		UniversalEIN: deref(a.UniversalEin), AltBusinessIDType: deref(a.AltBusinessIDType), AltBusinessID: deref(a.AltBusinessID),
		Phone: deref(a.Phone), MobilePhone: deref(a.MobilePhone), Street: deref(a.Street), City: deref(a.City),
		State: deref(a.State), PostalCode: deref(a.PostalCode), Country: deref(a.Country), Email: deref(a.Email),
		StockSymbol: deref(a.StockSymbol), StockExchange: deref(a.StockExchange), Website: deref(a.Website),
		Vertical: deref(a.Vertical), VerificationEmail: deref(a.VerificationEmail), Reseller: a.Reseller,
	}
}

func tenDlcBrandPatchBody(a TenDlcBrandArgs) wireTenDlcBrandPatch {
	return wireTenDlcBrandPatch{
		CompanyName: deref(a.CompanyName), FirstName: deref(a.FirstName), LastName: deref(a.LastName),
		DisplayName: deref(a.DisplayName), EIN: deref(a.Ein), EINIssuingCountry: deref(a.EinIssuingCountry),
		EntityType: a.EntityType, Website: deref(a.Website), Phone: deref(a.Phone), MobilePhone: deref(a.MobilePhone),
		Street: deref(a.Street), City: deref(a.City), State: deref(a.State), PostalCode: deref(a.PostalCode),
		Country: deref(a.Country), Email: deref(a.Email), StockSymbol: deref(a.StockSymbol),
		StockExchange: deref(a.StockExchange), Vertical: deref(a.Vertical), Reseller: a.Reseller,
	}
}

func tenDlcBrandArgsFrom(w *wireTenDlcBrand, prior TenDlcBrandArgs, importing bool) TenDlcBrandArgs {
	et := w.EntityType
	if strings.EqualFold(et, prior.EntityType) {
		et = prior.EntityType
	}
	return TenDlcBrandArgs{
		EntityType: et,
		AccountID:  adoptStr(prior.AccountID, w.AccountID, importing), PrimaryAccountID: adoptStr(prior.PrimaryAccountID, w.PrimaryAccountID, importing),
		FirstName: adoptStr(prior.FirstName, w.FirstName, importing), LastName: adoptStr(prior.LastName, w.LastName, importing),
		DisplayName: adoptStr(prior.DisplayName, w.DisplayName, importing), CompanyName: adoptStr(prior.CompanyName, w.CompanyName, importing),
		Ein: adoptStr(prior.Ein, w.EIN, importing), EinIssuingCountry: adoptStr(prior.EinIssuingCountry, w.EINIssuingCountry, importing),
		UniversalEin: adoptStr(prior.UniversalEin, w.UniversalEIN, importing), AltBusinessIDType: adoptStr(prior.AltBusinessIDType, w.AltBusinessIDType, importing),
		AltBusinessID: adoptStr(prior.AltBusinessID, w.AltBusinessID, importing), Phone: adoptStr(prior.Phone, w.Phone, importing),
		MobilePhone: adoptStr(prior.MobilePhone, w.MobilePhone, importing), Street: adoptStr(prior.Street, w.Street, importing),
		City: adoptStr(prior.City, w.City, importing), State: adoptStr(prior.State, w.State, importing),
		PostalCode: adoptStr(prior.PostalCode, w.PostalCode, importing), Country: adoptStr(prior.Country, w.Country, importing),
		Email: adoptStr(prior.Email, w.Email, importing), StockSymbol: adoptStr(prior.StockSymbol, w.StockSymbol, importing),
		StockExchange: adoptStr(prior.StockExchange, w.StockExchange, importing), Website: adoptStr(prior.Website, w.Website, importing),
		Vertical: adoptStr(prior.Vertical, w.Vertical, importing), VerificationEmail: adoptStr(prior.VerificationEmail, w.VerificationEmail, importing),
		Reseller: adoptBool(prior.Reseller, w.Reseller, importing),
	}
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func tenDlcBrandStateFrom(w *wireTenDlcBrand, args TenDlcBrandArgs) TenDlcBrandState {
	return TenDlcBrandState{
		TenDlcBrandArgs: args, BrandID: w.BrandID, Status: w.Status, BrandRelationship: optStr(w.BrandRelationship),
		CampaignCount: w.CampaignCount, ReferenceID: optStr(w.ReferenceID), VerifiedDate: optStr(w.VerifiedDate),
		CreatedDate: optStr(w.CreatedDate), LastUpdated: optStr(w.LastUpdated), Owner: w.Owner, Partner: w.Partner, Shared: w.Shared,
	}
}

func (r *TenDlcBrand) Create(ctx context.Context, req infer.CreateRequest[TenDlcBrandArgs]) (infer.CreateResponse[TenDlcBrandState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[TenDlcBrandState]{Output: TenDlcBrandState{TenDlcBrandArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[TenDlcBrandState]{}, err
	}
	body := tenDlcBrandBody(a, c.apiKey)
	if body.AccountID == "" {
		return infer.CreateResponse[TenDlcBrandState]{}, fmt.Errorf("create 10DLC brand: set accountId or configure the provider's apiKey")
	}
	w, err := c.CreateTenDlcBrand(ctx, body)
	if err != nil {
		return infer.CreateResponse[TenDlcBrandState]{}, fmt.Errorf("create 10DLC brand: %w", err)
	}
	if w.BrandID == "" {
		return infer.CreateResponse[TenDlcBrandState]{}, fmt.Errorf("create 10DLC brand: the response carries no brand_id")
	}
	return infer.CreateResponse[TenDlcBrandState]{ID: w.BrandID, Output: tenDlcBrandStateFrom(w, a)}, nil
}

func (r *TenDlcBrand) Read(ctx context.Context, req infer.ReadRequest[TenDlcBrandArgs, TenDlcBrandState]) (infer.ReadResponse[TenDlcBrandArgs, TenDlcBrandState], error) {
	type resp = infer.ReadResponse[TenDlcBrandArgs, TenDlcBrandState]
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	w, err := c.GetTenDlcBrand(ctx, req.ID)
	if err != nil {
		return resp{}, fmt.Errorf("read 10DLC brand %s: %w", req.ID, err)
	}
	if w == nil {
		if err := notFoundOnImport("10DLC brand", req.ID, req.Inputs, req.State); err != nil {
			return resp{}, err
		}
		return resp{}, nil
	}
	importing := isZeroImport(req.Inputs, req.State)
	args := tenDlcBrandArgsFrom(w, req.Inputs, importing)
	if w.BrandID == "" {
		w.BrandID = req.ID
	}
	return resp{ID: req.ID, Inputs: args, State: tenDlcBrandStateFrom(w, args)}, nil
}

func (r *TenDlcBrand) Diff(ctx context.Context, req infer.DiffRequest[TenDlcBrandArgs, TenDlcBrandState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.TenDlcBrandArgs
	set := func(name string, a, b *string, kind p.DiffKind) {
		if a != nil && (b == nil || *a != *b) {
			d[name] = p.PropertyDiff{Kind: kind}
		}
	}
	if in.EntityType != old.EntityType {
		d["entityType"] = p.PropertyDiff{Kind: p.Update}
	}
	// Fixed at registration: the PATCH call cannot change them.
	set("accountId", in.AccountID, old.AccountID, p.UpdateReplace)
	set("primaryAccountId", in.PrimaryAccountID, old.PrimaryAccountID, p.UpdateReplace)
	set("universalEin", in.UniversalEin, old.UniversalEin, p.UpdateReplace)
	set("altBusinessIdType", in.AltBusinessIDType, old.AltBusinessIDType, p.UpdateReplace)
	set("altBusinessId", in.AltBusinessID, old.AltBusinessID, p.UpdateReplace)
	set("verificationEmail", in.VerificationEmail, old.VerificationEmail, p.UpdateReplace)
	// Changeable in place.
	set("firstName", in.FirstName, old.FirstName, p.Update)
	set("lastName", in.LastName, old.LastName, p.Update)
	set("displayName", in.DisplayName, old.DisplayName, p.Update)
	set("companyName", in.CompanyName, old.CompanyName, p.Update)
	set("ein", in.Ein, old.Ein, p.Update)
	set("einIssuingCountry", in.EinIssuingCountry, old.EinIssuingCountry, p.Update)
	set("phone", in.Phone, old.Phone, p.Update)
	set("mobilePhone", in.MobilePhone, old.MobilePhone, p.Update)
	set("street", in.Street, old.Street, p.Update)
	set("city", in.City, old.City, p.Update)
	set("state", in.State, old.State, p.Update)
	set("postalCode", in.PostalCode, old.PostalCode, p.Update)
	set("country", in.Country, old.Country, p.Update)
	set("email", in.Email, old.Email, p.Update)
	set("stockSymbol", in.StockSymbol, old.StockSymbol, p.Update)
	set("stockExchange", in.StockExchange, old.StockExchange, p.Update)
	set("website", in.Website, old.Website, p.Update)
	set("vertical", in.Vertical, old.Vertical, p.Update)
	if in.Reseller != nil && (old.Reseller == nil || *in.Reseller != *old.Reseller) {
		d["reseller"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *TenDlcBrand) Update(ctx context.Context, req infer.UpdateRequest[TenDlcBrandArgs, TenDlcBrandState]) (infer.UpdateResponse[TenDlcBrandState], error) {
	a := req.Inputs
	if req.DryRun {
		st := req.State
		st.TenDlcBrandArgs = a
		return infer.UpdateResponse[TenDlcBrandState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[TenDlcBrandState]{}, err
	}
	w, err := c.PatchTenDlcBrand(ctx, req.ID, tenDlcBrandPatchBody(a))
	if err != nil {
		return infer.UpdateResponse[TenDlcBrandState]{}, fmt.Errorf("update 10DLC brand %s: %w", req.ID, err)
	}
	if w.BrandID == "" {
		w.BrandID = req.ID
	}
	return infer.UpdateResponse[TenDlcBrandState]{Output: tenDlcBrandStateFrom(w, a)}, nil
}

func (r *TenDlcBrand) Delete(ctx context.Context, req infer.DeleteRequest[TenDlcBrandState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.DeleteTenDlcBrand(ctx, req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete 10DLC brand %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}
