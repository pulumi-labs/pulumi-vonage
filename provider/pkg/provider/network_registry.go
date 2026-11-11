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
	"slices"
	"sort"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// Network Registry API (https://developer.vonage.com/en/api/network-registry, OpenAPI v1.0.0).
// The request and response shapes below follow the published OpenAPI document and have not been
// exercised against a live Vonage account. The document lists https://api-eu.vonage.com as its
// server; set the provider's apiBaseUrl (VONAGE_API_BASE_URL) to that host when the default
// API host does not serve /v1/network-registry for your account.

const (
	nrBase = "/v1/network-registry"
	// nrPageLimit is the page size asked of listings that accept a limit.
	nrPageLimit = 100
	// maxNRPages bounds the pages a listing follows.
	maxNRPages = 100
)

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

type wireNRBusinessInfo struct {
	AddressStreet1 string `json:"address_street_1"`
	AddressStreet2 string `json:"address_street_2,omitempty"`
	PostalCode     string `json:"postal_code"`
	Country        string `json:"country"`
	City           string `json:"city"`
	Province       string `json:"province,omitempty"`
}

// wireNRBusinessProfileIn is the writable part of a business profile (create body and
// change-request body).
type wireNRBusinessProfileIn struct {
	Countries                         []string           `json:"countries"`
	ContactRole                       string             `json:"contact_role"`
	Name                              string             `json:"name"`
	TradingName                       string             `json:"trading_name,omitempty"`
	OrganisationType                  string             `json:"organisation_type"`
	TaxNumber                         string             `json:"tax_number"`
	BusinessRegistrationID            string             `json:"business_registration_id,omitempty"`
	IssuingAuthority                  string             `json:"issuing_authority,omitempty"`
	PrivacyPolicyURL                  string             `json:"privacy_policy_url"`
	PrivacyPolicyManagerEmail         string             `json:"privacy_policy_manager_email"`
	DpoEmail                          string             `json:"dpo_email,omitempty"`
	DpoRequired                       *bool              `json:"dpo_required,omitempty"`
	TermsAndConditionsAccepted        bool               `json:"terms_and_conditions_accepted"`
	BusinessInformation               wireNRBusinessInfo `json:"business_information"`
	EmailNotificationsEnabled         *bool              `json:"email_notifications_enabled,omitempty"`
	VettingTermsAndConditionsAccepted *bool              `json:"vetting_terms_and_conditions_accepted,omitempty"`
	VettingExtraFields                map[string]string  `json:"vetting_extra_fields,omitempty"`
}

type wireNRStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// wireNRChangeRequest is a pending change: only the fields being changed are present.
type wireNRChangeRequest struct {
	Countries                         []string            `json:"countries"`
	ContactRole                       *string             `json:"contact_role"`
	Name                              *string             `json:"name"`
	TradingName                       *string             `json:"trading_name"`
	OrganisationType                  *string             `json:"organisation_type"`
	TaxNumber                         *string             `json:"tax_number"`
	BusinessRegistrationID            *string             `json:"business_registration_id"`
	IssuingAuthority                  *string             `json:"issuing_authority"`
	PrivacyPolicyURL                  *string             `json:"privacy_policy_url"`
	PrivacyPolicyManagerEmail         *string             `json:"privacy_policy_manager_email"`
	DpoEmail                          *string             `json:"dpo_email"`
	DpoRequired                       *bool               `json:"dpo_required"`
	TermsAndConditionsAccepted        *bool               `json:"terms_and_conditions_accepted"`
	BusinessInformation               *wireNRBusinessInfo `json:"business_information"`
	EmailNotificationsEnabled         *bool               `json:"email_notifications_enabled"`
	VettingTermsAndConditionsAccepted *bool               `json:"vetting_terms_and_conditions_accepted"`
	VettingExtraFields                map[string]string   `json:"vetting_extra_fields"`
	Status                            *wireNRStatus       `json:"status"`
}

type wireNRBusinessProfile struct {
	wireNRBusinessProfileIn
	ID            string               `json:"id"`
	AccountID     string               `json:"account_id"`
	Status        wireNRStatus         `json:"status"`
	CreatedAt     string               `json:"created_at"`
	UpdatedAt     string               `json:"updated_at"`
	DeleteAt      string               `json:"delete_at"`
	ChangeRequest *wireNRChangeRequest `json:"change_request"`
}

// effective returns the profile as it will read once a pending change request is approved: the
// change request overlays the profile's own fields. A rejected change request is ignored.
func (w wireNRBusinessProfile) effective() wireNRBusinessProfileIn {
	out := w.wireNRBusinessProfileIn
	cr := w.ChangeRequest
	if cr == nil || (cr.Status != nil && cr.Status.Status == "rejected") {
		return out
	}
	if cr.Countries != nil {
		out.Countries = cr.Countries
	}
	setS := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	setB := func(dst **bool, src *bool) {
		if src != nil {
			v := *src
			*dst = &v
		}
	}
	setS(&out.ContactRole, cr.ContactRole)
	setS(&out.Name, cr.Name)
	setS(&out.TradingName, cr.TradingName)
	setS(&out.OrganisationType, cr.OrganisationType)
	setS(&out.TaxNumber, cr.TaxNumber)
	setS(&out.BusinessRegistrationID, cr.BusinessRegistrationID)
	setS(&out.IssuingAuthority, cr.IssuingAuthority)
	setS(&out.PrivacyPolicyURL, cr.PrivacyPolicyURL)
	setS(&out.PrivacyPolicyManagerEmail, cr.PrivacyPolicyManagerEmail)
	setS(&out.DpoEmail, cr.DpoEmail)
	setB(&out.DpoRequired, cr.DpoRequired)
	setB(&out.EmailNotificationsEnabled, cr.EmailNotificationsEnabled)
	setB(&out.VettingTermsAndConditionsAccepted, cr.VettingTermsAndConditionsAccepted)
	if cr.TermsAndConditionsAccepted != nil {
		out.TermsAndConditionsAccepted = *cr.TermsAndConditionsAccepted
	}
	if cr.BusinessInformation != nil {
		out.BusinessInformation = *cr.BusinessInformation
	}
	if cr.VettingExtraFields != nil {
		out.VettingExtraFields = cr.VettingExtraFields
	}
	return out
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// nrEnvelope is the paging envelope shared by every Network Registry listing.
type nrEnvelope struct {
	Links struct {
		Next *struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
	Embedded map[string]json.RawMessage `json:"_embedded"`
}

// nrList follows every page of a Network Registry listing and returns the items under key. The
// next page is requested with the opaque "next" token taken from the next link; a next link
// without a token, or a token seen twice, is an error rather than a guess.
func nrList[T any](ctx context.Context, c *Client, path string, query url.Values, key string) ([]T, error) {
	out := []T{}
	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	seen := map[string]bool{}
	for page := 0; page < maxNRPages; page++ {
		var env nrEnvelope
		if err := c.apiJSON(ctx, http.MethodGet, path, q, nil, &env); err != nil {
			return nil, err
		}
		if raw, ok := env.Embedded[key]; ok && len(raw) > 0 && string(raw) != "null" {
			var items []T
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, fmt.Errorf("vonage: GET %s: decoding %s: %w", path, key, err)
			}
			out = append(out, items...)
		}
		if env.Links.Next == nil || env.Links.Next.Href == "" {
			return out, nil
		}
		u, err := url.Parse(env.Links.Next.Href)
		tok := ""
		if err == nil {
			tok = u.Query().Get("next")
		}
		if tok == "" {
			return nil, fmt.Errorf("vonage: GET %s: next link %q carries no next token, refusing to guess", path, env.Links.Next.Href)
		}
		if seen[tok] {
			return nil, fmt.Errorf("vonage: GET %s: next token repeated, refusing to loop", path)
		}
		seen[tok] = true
		q.Set("next", tok)
	}
	return nil, fmt.Errorf("vonage: GET %s: more than %d pages", path, maxNRPages)
}

func nrBusinessProfilePath(id string) string {
	return nrBase + "/business-profiles/" + url.PathEscape(id)
}

// CreateNRBusinessProfile registers a business profile. The call is never replayed after an
// unknown outcome (POST), so a retry cannot register the profile twice.
func (c *Client) CreateNRBusinessProfile(ctx context.Context, in wireNRBusinessProfileIn) (*wireNRBusinessProfile, error) {
	var out wireNRBusinessProfile
	if err := c.apiJSON(ctx, http.MethodPost, nrBase+"/business-profiles", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetNRBusinessProfile returns nil, nil when the profile does not exist or has been deleted
// (Vonage deletes softly and reports delete_at).
func (c *Client) GetNRBusinessProfile(ctx context.Context, id string) (*wireNRBusinessProfile, error) {
	var out wireNRBusinessProfile
	err := c.apiJSON(ctx, http.MethodGet, nrBusinessProfilePath(id), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.DeleteAt != "" {
		return nil, nil
	}
	return &out, nil
}

// ChangeNRBusinessProfile submits a change request for a profile.
func (c *Client) ChangeNRBusinessProfile(ctx context.Context, id string, in wireNRBusinessProfileIn) (*wireNRBusinessProfile, error) {
	var out wireNRBusinessProfile
	if err := c.apiJSON(ctx, http.MethodPut, nrBusinessProfilePath(id)+"/change-request", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteNRBusinessProfile deletes a profile; a missing profile is not an error.
func (c *Client) DeleteNRBusinessProfile(ctx context.Context, id string) error {
	err := c.apiJSON(ctx, http.MethodDelete, nrBusinessProfilePath(id), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// ListNRBusinessProfiles returns every business profile visible to the account, skipping
// soft-deleted ones. shared selects profiles shared with the account.
func (c *Client) ListNRBusinessProfiles(ctx context.Context, shared bool) ([]wireNRBusinessProfile, error) {
	q := url.Values{"limit": {fmt.Sprint(nrPageLimit)}}
	if shared {
		q.Set("shared", "true")
	}
	all, err := nrList[wireNRBusinessProfile](ctx, c, nrBase+"/business-profiles", q, "business_profiles")
	if err != nil {
		return nil, err
	}
	out := []wireNRBusinessProfile{}
	for _, w := range all {
		if w.DeleteAt == "" {
			out = append(out, w)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// NetworkBusinessProfile
// ---------------------------------------------------------------------------

var nrOrganisationTypes = []string{
	"academicScientificOrganisation", "forProfitOrganisation", "governmentalOrganisation", "industryConsortium",
	"internationalOrganisation", "nonProfitOrganisation", "nonGovernmentalOrganisation", "organisationalUnit",
}

// NetworkBusinessProfile registers a business with the Vonage Network Registry, the first step
// towards using Vonage Network APIs (number verification, SIM swap, ...) with mobile operators.
type NetworkBusinessProfile struct{}

// NetworkBusinessInformation is the postal address of the business.
type NetworkBusinessInformation struct {
	AddressStreet1 string  `pulumi:"addressStreet1"`
	AddressStreet2 *string `pulumi:"addressStreet2,optional"`
	PostalCode     string  `pulumi:"postalCode"`
	Country        string  `pulumi:"country"`
	City           string  `pulumi:"city"`
	Province       *string `pulumi:"province,optional"`
}

func (a *NetworkBusinessInformation) Annotate(an infer.Annotator) {
	an.Describe(&a.AddressStreet1, "First line of the street address (at most 500 characters).")
	an.Describe(&a.AddressStreet2, "Second line of the street address.")
	an.Describe(&a.PostalCode, "Postal or ZIP code.")
	an.Describe(&a.Country, "Two-letter ISO 3166 country code of the address.")
	an.Describe(&a.City, "City.")
	an.Describe(&a.Province, "Province, state or region.")
}

// NetworkBusinessProfileArgs are the inputs of a NetworkBusinessProfile.
type NetworkBusinessProfileArgs struct {
	Countries                         []string                   `pulumi:"countries"`
	ContactRole                       string                     `pulumi:"contactRole"`
	Name                              string                     `pulumi:"name"`
	TradingName                       *string                    `pulumi:"tradingName,optional"`
	OrganisationType                  string                     `pulumi:"organisationType"`
	TaxNumber                         string                     `pulumi:"taxNumber"`
	BusinessRegistrationID            *string                    `pulumi:"businessRegistrationId,optional"`
	IssuingAuthority                  *string                    `pulumi:"issuingAuthority,optional"`
	PrivacyPolicyURL                  string                     `pulumi:"privacyPolicyUrl"`
	PrivacyPolicyManagerEmail         string                     `pulumi:"privacyPolicyManagerEmail"`
	DpoEmail                          *string                    `pulumi:"dpoEmail,optional"`
	DpoRequired                       *bool                      `pulumi:"dpoRequired,optional"`
	TermsAndConditionsAccepted        bool                       `pulumi:"termsAndConditionsAccepted"`
	BusinessInformation               NetworkBusinessInformation `pulumi:"businessInformation"`
	EmailNotificationsEnabled         *bool                      `pulumi:"emailNotificationsEnabled,optional"`
	VettingTermsAndConditionsAccepted *bool                      `pulumi:"vettingTermsAndConditionsAccepted,optional"`
	VettingExtraFields                map[string]string          `pulumi:"vettingExtraFields,optional"`
}

func (a *NetworkBusinessProfileArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Countries, "Two-letter ISO 3166 codes of the countries the business will use Network APIs in. They decide which operators (CSPs) and vetting levels apply; see getNetworkRegistryCatalogue.")
	an.Describe(&a.ContactRole, "Role of the person responsible for the profile, for example CTO (at most 100 characters).")
	an.Describe(&a.Name, "Legal name of the business (at most 100 characters).")
	an.Describe(&a.TradingName, "Trading name, when it differs from the legal name. Optional fields that are left unset are not managed: removing one from the program does not clear it at Vonage.")
	an.Describe(&a.OrganisationType, "Kind of organisation: academicScientificOrganisation, forProfitOrganisation, governmentalOrganisation, industryConsortium, internationalOrganisation, nonProfitOrganisation, nonGovernmentalOrganisation or organisationalUnit.")
	an.Describe(&a.TaxNumber, "Tax identification number of the business.")
	an.Describe(&a.BusinessRegistrationID, "Company registration number.")
	an.Describe(&a.IssuingAuthority, "Authority that issued the registration.")
	an.Describe(&a.PrivacyPolicyURL, "URL of the business's privacy policy.")
	an.Describe(&a.PrivacyPolicyManagerEmail, "Email address of the person managing the privacy policy.")
	an.Describe(&a.DpoEmail, "Email address of the data protection officer.")
	an.Describe(&a.DpoRequired, "Whether the business is required to have a data protection officer.")
	an.Describe(&a.TermsAndConditionsAccepted, "Whether you accept the Network Registry terms and conditions (see getNetworkTermsAndConditions).")
	an.Describe(&a.BusinessInformation, "Postal address of the business.")
	an.Describe(&a.EmailNotificationsEnabled, "Whether Vonage emails status notifications about the profile.")
	an.Describe(&a.VettingTermsAndConditionsAccepted, "Whether you accept the vetting terms and conditions, when vetting applies.")
	an.Describe(&a.VettingExtraFields, "Extra fields required by the vetting levels that apply to the selected countries, as name to value (see getNetworkRegistryCatalogue vettingLevels extraFields).")
}

// NetworkBusinessProfileState is the persisted state of a NetworkBusinessProfile.
type NetworkBusinessProfileState struct {
	NetworkBusinessProfileArgs

	AccountID string `pulumi:"accountId"`
	// Status is the overall registration status, which moves on its own as operators review the profile.
	Status        string  `pulumi:"status"`
	StatusMessage *string `pulumi:"statusMessage,optional"`
	// ChangeRequestStatus is the status of the latest change request, when there is one.
	ChangeRequestStatus *string `pulumi:"changeRequestStatus,optional"`
	CreatedAt           string  `pulumi:"createdAt"`
	UpdatedAt           string  `pulumi:"updatedAt"`
}

func (s *NetworkBusinessProfileState) Annotate(a infer.Annotator) {
	a.Describe(&s.AccountID, "ID of the Vonage account that owns the profile.")
	a.Describe(&s.Status, "Overall registration status: pending, reviewed, ready-to-provision, provisioning, accepted, partially-accepted, rejected or failed. It changes as operators review the profile.")
	a.Describe(&s.StatusMessage, "Vonage's note about the status, when there is one.")
	a.Describe(&s.ChangeRequestStatus, "Status of the latest change request (pending, accepted or rejected), when there is one.")
	a.Describe(&s.CreatedAt, "When the profile was created (RFC 3339).")
	a.Describe(&s.UpdatedAt, "When the profile was last updated (RFC 3339).")
}

func (r *NetworkBusinessProfile) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Vonage Network Registry business profile. Registering a profile is a legal submission that Vonage and mobile operators review: it is not an instant API call, and its status moves from pending towards accepted on its own. Vonage has no plain update: a changed input is submitted as a change request, which the profile reflects once approved (until then the resource reports the requested values). Destroying the resource deletes the profile (Vonage deletes softly and can undelete it). Shapes follow the published Network Registry OpenAPI definition and were not verified against a live account.")
}

var _ infer.CustomResource[NetworkBusinessProfileArgs, NetworkBusinessProfileState] = (*NetworkBusinessProfile)(nil)
var _ infer.CustomCheck[NetworkBusinessProfileArgs] = (*NetworkBusinessProfile)(nil)
var _ infer.CustomRead[NetworkBusinessProfileArgs, NetworkBusinessProfileState] = (*NetworkBusinessProfile)(nil)
var _ infer.CustomUpdate[NetworkBusinessProfileArgs, NetworkBusinessProfileState] = (*NetworkBusinessProfile)(nil)
var _ infer.CustomDiff[NetworkBusinessProfileArgs, NetworkBusinessProfileState] = (*NetworkBusinessProfile)(nil)
var _ infer.CustomDelete[NetworkBusinessProfileState] = (*NetworkBusinessProfile)(nil)

func isCountryCode(s string) bool {
	if len(s) != 2 {
		return false
	}
	for _, r := range s {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func (r *NetworkBusinessProfile) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[NetworkBusinessProfileArgs], error) {
	args, failures, err := infer.DefaultCheck[NetworkBusinessProfileArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[NetworkBusinessProfileArgs]{Inputs: args, Failures: failures}, err
	}
	fail := func(prop, reason string) { failures = append(failures, p.CheckFailure{Property: prop, Reason: reason}) }
	if len(args.Countries) == 0 {
		fail("countries", "countries must list at least one country")
	}
	for _, c := range args.Countries {
		if !isCountryCode(c) {
			fail("countries", fmt.Sprintf("%q is not a two-letter ISO 3166 country code", c))
		}
	}
	for prop, v := range map[string]string{"contactRole": args.ContactRole, "name": args.Name, "taxNumber": args.TaxNumber,
		"privacyPolicyUrl": args.PrivacyPolicyURL, "privacyPolicyManagerEmail": args.PrivacyPolicyManagerEmail} {
		if strings.TrimSpace(v) == "" {
			fail(prop, prop+" must not be empty")
		}
	}
	if !slices.Contains(nrOrganisationTypes, args.OrganisationType) {
		fail("organisationType", "organisationType must be one of "+strings.Join(nrOrganisationTypes, ", "))
	}
	bi := args.BusinessInformation
	if strings.TrimSpace(bi.AddressStreet1) == "" || strings.TrimSpace(bi.PostalCode) == "" || strings.TrimSpace(bi.City) == "" {
		fail("businessInformation", "addressStreet1, postalCode and city must not be empty")
	}
	if !isCountryCode(bi.Country) {
		fail("businessInformation", "businessInformation.country must be a two-letter ISO 3166 country code")
	}
	return infer.CheckResponse[NetworkBusinessProfileArgs]{Inputs: args, Failures: dropUnknownFailures(failures, req.NewInputs)}, nil
}

func nrBody(a NetworkBusinessProfileArgs) wireNRBusinessProfileIn {
	return wireNRBusinessProfileIn{
		Countries: a.Countries, ContactRole: a.ContactRole, Name: a.Name, TradingName: deref(a.TradingName),
		OrganisationType: a.OrganisationType, TaxNumber: a.TaxNumber, BusinessRegistrationID: deref(a.BusinessRegistrationID),
		IssuingAuthority: deref(a.IssuingAuthority), PrivacyPolicyURL: a.PrivacyPolicyURL,
		PrivacyPolicyManagerEmail: a.PrivacyPolicyManagerEmail, DpoEmail: deref(a.DpoEmail), DpoRequired: a.DpoRequired,
		TermsAndConditionsAccepted: a.TermsAndConditionsAccepted,
		BusinessInformation: wireNRBusinessInfo{AddressStreet1: a.BusinessInformation.AddressStreet1,
			AddressStreet2: deref(a.BusinessInformation.AddressStreet2), PostalCode: a.BusinessInformation.PostalCode,
			Country: a.BusinessInformation.Country, City: a.BusinessInformation.City, Province: deref(a.BusinessInformation.Province)},
		EmailNotificationsEnabled: a.EmailNotificationsEnabled, VettingTermsAndConditionsAccepted: a.VettingTermsAndConditionsAccepted,
		VettingExtraFields: a.VettingExtraFields,
	}
}

func sameCountries(a, b []string) bool {
	norm := func(in []string) []string {
		out := make([]string, len(in))
		for i, s := range in {
			out[i] = strings.ToUpper(s)
		}
		sort.Strings(out)
		return out
	}
	return slices.Equal(norm(a), norm(b))
}

// adoptStr reports a server string as an optional input. A value the program set is refreshed from
// the server (so drift shows); one it left unset stays unset (the field is unmanaged), except on
// import, where there is no program yet and the server value is adopted.
func adoptStr(prior *string, server string, importing bool) *string {
	if prior != nil {
		v := server
		if strings.EqualFold(v, *prior) && v != *prior {
			v = *prior
		}
		return &v
	}
	if importing && server != "" {
		return &server
	}
	return nil
}

func adoptBool(prior, server *bool, importing bool) *bool {
	if server == nil {
		return prior
	}
	if prior != nil || importing {
		v := *server
		return &v
	}
	return nil
}

// nrArgsFrom maps the effective server profile onto inputs, keeping the program's spelling where
// the server value is equivalent and leaving unset optional fields unmanaged.
func nrArgsFrom(e wireNRBusinessProfileIn, prior NetworkBusinessProfileArgs, importing bool) NetworkBusinessProfileArgs {
	countries := e.Countries
	if sameCountries(countries, prior.Countries) {
		countries = prior.Countries
	}
	bi := e.BusinessInformation
	pbi := prior.BusinessInformation
	country := bi.Country
	if strings.EqualFold(country, pbi.Country) {
		country = pbi.Country
	}
	var extra map[string]string
	if prior.VettingExtraFields != nil || (importing && len(e.VettingExtraFields) > 0) {
		extra = e.VettingExtraFields
		if extra == nil {
			extra = map[string]string{}
		}
	}
	return NetworkBusinessProfileArgs{
		Countries: countries, ContactRole: e.ContactRole, Name: e.Name,
		TradingName:      adoptStr(prior.TradingName, e.TradingName, importing),
		OrganisationType: e.OrganisationType, TaxNumber: e.TaxNumber,
		BusinessRegistrationID:     adoptStr(prior.BusinessRegistrationID, e.BusinessRegistrationID, importing),
		IssuingAuthority:           adoptStr(prior.IssuingAuthority, e.IssuingAuthority, importing),
		PrivacyPolicyURL:           e.PrivacyPolicyURL,
		PrivacyPolicyManagerEmail:  e.PrivacyPolicyManagerEmail,
		DpoEmail:                   adoptStr(prior.DpoEmail, e.DpoEmail, importing),
		DpoRequired:                adoptBool(prior.DpoRequired, e.DpoRequired, importing),
		TermsAndConditionsAccepted: e.TermsAndConditionsAccepted,
		BusinessInformation: NetworkBusinessInformation{
			AddressStreet1: bi.AddressStreet1, PostalCode: bi.PostalCode, Country: country, City: bi.City,
			AddressStreet2: adoptStr(pbi.AddressStreet2, bi.AddressStreet2, importing),
			Province:       adoptStr(pbi.Province, bi.Province, importing),
		},
		EmailNotificationsEnabled:         adoptBool(prior.EmailNotificationsEnabled, e.EmailNotificationsEnabled, importing),
		VettingTermsAndConditionsAccepted: adoptBool(prior.VettingTermsAndConditionsAccepted, e.VettingTermsAndConditionsAccepted, importing),
		VettingExtraFields:                extra,
	}
}

func nrStateFrom(w *wireNRBusinessProfile, args NetworkBusinessProfileArgs) NetworkBusinessProfileState {
	st := NetworkBusinessProfileState{NetworkBusinessProfileArgs: args, AccountID: w.AccountID, Status: w.Status.Status,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt}
	if w.Status.Message != "" {
		m := w.Status.Message
		st.StatusMessage = &m
	}
	if w.ChangeRequest != nil && w.ChangeRequest.Status != nil && w.ChangeRequest.Status.Status != "" {
		s := w.ChangeRequest.Status.Status
		st.ChangeRequestStatus = &s
	}
	return st
}

func (r *NetworkBusinessProfile) Create(ctx context.Context, req infer.CreateRequest[NetworkBusinessProfileArgs]) (infer.CreateResponse[NetworkBusinessProfileState], error) {
	a := req.Inputs
	if req.DryRun {
		return infer.CreateResponse[NetworkBusinessProfileState]{Output: NetworkBusinessProfileState{NetworkBusinessProfileArgs: a}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[NetworkBusinessProfileState]{}, err
	}
	w, err := c.CreateNRBusinessProfile(ctx, nrBody(a))
	if err != nil {
		return infer.CreateResponse[NetworkBusinessProfileState]{}, fmt.Errorf("create network business profile %q: %w", a.Name, err)
	}
	if w.ID == "" {
		return infer.CreateResponse[NetworkBusinessProfileState]{}, fmt.Errorf("create network business profile %q: the response carries no id", a.Name)
	}
	return infer.CreateResponse[NetworkBusinessProfileState]{ID: w.ID, Output: nrStateFrom(w, a)}, nil
}

func (r *NetworkBusinessProfile) Read(ctx context.Context, req infer.ReadRequest[NetworkBusinessProfileArgs, NetworkBusinessProfileState]) (infer.ReadResponse[NetworkBusinessProfileArgs, NetworkBusinessProfileState], error) {
	type resp = infer.ReadResponse[NetworkBusinessProfileArgs, NetworkBusinessProfileState]
	c, err := clientFor(ctx)
	if err != nil {
		return resp{}, err
	}
	w, err := c.GetNRBusinessProfile(ctx, req.ID)
	if err != nil {
		return resp{}, fmt.Errorf("read network business profile %s: %w", req.ID, err)
	}
	if w == nil {
		if err := notFoundOnImport("network business profile", req.ID, req.Inputs, req.State); err != nil {
			return resp{}, err
		}
		return resp{}, nil
	}
	importing := reflect.ValueOf(req.Inputs).IsZero() && reflect.ValueOf(req.State).IsZero()
	args := nrArgsFrom(w.effective(), req.Inputs, importing)
	return resp{ID: req.ID, Inputs: args, State: nrStateFrom(w, args)}, nil
}

func (r *NetworkBusinessProfile) Diff(ctx context.Context, req infer.DiffRequest[NetworkBusinessProfileArgs, NetworkBusinessProfileState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	in, old := req.Inputs, req.State.NetworkBusinessProfileArgs
	upd := func(name string, changed bool) {
		if changed {
			d[name] = p.PropertyDiff{Kind: p.Update}
		}
	}
	optS := func(a, b *string) bool { return a != nil && (b == nil || *a != *b) }
	optB := func(a, b *bool) bool { return a != nil && (b == nil || *a != *b) }
	upd("countries", !sameCountries(in.Countries, old.Countries))
	upd("contactRole", in.ContactRole != old.ContactRole)
	upd("name", in.Name != old.Name)
	upd("tradingName", optS(in.TradingName, old.TradingName))
	upd("organisationType", in.OrganisationType != old.OrganisationType)
	upd("taxNumber", in.TaxNumber != old.TaxNumber)
	upd("businessRegistrationId", optS(in.BusinessRegistrationID, old.BusinessRegistrationID))
	upd("issuingAuthority", optS(in.IssuingAuthority, old.IssuingAuthority))
	upd("privacyPolicyUrl", in.PrivacyPolicyURL != old.PrivacyPolicyURL)
	upd("privacyPolicyManagerEmail", in.PrivacyPolicyManagerEmail != old.PrivacyPolicyManagerEmail)
	upd("dpoEmail", optS(in.DpoEmail, old.DpoEmail))
	upd("dpoRequired", optB(in.DpoRequired, old.DpoRequired))
	upd("termsAndConditionsAccepted", in.TermsAndConditionsAccepted != old.TermsAndConditionsAccepted)
	upd("emailNotificationsEnabled", optB(in.EmailNotificationsEnabled, old.EmailNotificationsEnabled))
	upd("vettingTermsAndConditionsAccepted", optB(in.VettingTermsAndConditionsAccepted, old.VettingTermsAndConditionsAccepted))
	if in.VettingExtraFields != nil && !reflect.DeepEqual(in.VettingExtraFields, old.VettingExtraFields) &&
		!(len(in.VettingExtraFields) == 0 && len(old.VettingExtraFields) == 0) {
		d["vettingExtraFields"] = p.PropertyDiff{Kind: p.Update}
	}
	bi, obi := in.BusinessInformation, old.BusinessInformation
	upd("businessInformation", bi.AddressStreet1 != obi.AddressStreet1 || bi.PostalCode != obi.PostalCode ||
		bi.City != obi.City || !strings.EqualFold(bi.Country, obi.Country) ||
		optS(bi.AddressStreet2, obi.AddressStreet2) || optS(bi.Province, obi.Province))
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r *NetworkBusinessProfile) Update(ctx context.Context, req infer.UpdateRequest[NetworkBusinessProfileArgs, NetworkBusinessProfileState]) (infer.UpdateResponse[NetworkBusinessProfileState], error) {
	a := req.Inputs
	if req.DryRun {
		st := req.State
		st.NetworkBusinessProfileArgs = a
		return infer.UpdateResponse[NetworkBusinessProfileState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[NetworkBusinessProfileState]{}, err
	}
	// Vonage has no plain update: the change goes in as a change request, which the profile
	// takes over once approved. Read overlays the pending request so the state stays stable.
	body := nrBody(a)
	w, err := c.ChangeNRBusinessProfile(ctx, req.ID, body)
	if err != nil {
		return infer.UpdateResponse[NetworkBusinessProfileState]{}, fmt.Errorf("update network business profile %s: %w", req.ID, err)
	}
	if w.AccountID == "" {
		w.AccountID = req.State.AccountID
	}
	if w.Status.Status == "" {
		w.Status.Status = req.State.Status
	}
	if w.CreatedAt == "" {
		w.CreatedAt = req.State.CreatedAt
	}
	return infer.UpdateResponse[NetworkBusinessProfileState]{Output: nrStateFrom(w, a)}, nil
}

func (r *NetworkBusinessProfile) Delete(ctx context.Context, req infer.DeleteRequest[NetworkBusinessProfileState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.DeleteNRBusinessProfile(ctx, req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete network business profile %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}
